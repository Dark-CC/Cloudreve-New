package workflows

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/task"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/driver"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs/dbfs"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/manager"
	"github.com/cloudreve/Cloudreve/v4/pkg/hashid"
	"github.com/cloudreve/Cloudreve/v4/pkg/logging"
	"github.com/cloudreve/Cloudreve/v4/pkg/queue"
	"github.com/cloudreve/Cloudreve/v4/pkg/util"
	"github.com/cloudreve/Cloudreve/v4/service/audit"
	"github.com/gofrs/uuid"
	"golang.org/x/tools/container/intsets"
)

type (
	RelocateTask struct {
		*queue.DBTask

		l        logging.Logger
		state    *RelocateTaskState
		progress queue.Progresses
	}

	RelocateTaskState struct {
		Src         []string `json:"src,omitempty"`
		SrcMultiple bool     `json:"src_multiple,omitempty"`
		DstPolicyID int      `json:"dst_policy_id"`
		Failed      int      `json:"failed,omitempty"`
		Total       int      `json:"total,omitempty"`
		Done        bool     `json:"done,omitempty"`
	}

	relocateEntityJob struct {
		entity       fs.Entity
		file         fs.File
		parentFiles  []int
		primaryFiles []int
	}
)

func init() {
	queue.RegisterResumableTaskFactory(queue.RelocateTaskType, NewRelocateTaskFromModel)
}

// NewRelocateTask creates a task that relocates entities of given source URIs
// to another storage policy.
func NewRelocateTask(ctx context.Context, src []string, dstPolicyID int) (queue.Task, error) {
	state := &RelocateTaskState{
		Src:         src,
		SrcMultiple: len(src) > 1,
		DstPolicyID: dstPolicyID,
	}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state: %w", err)
	}

	return &RelocateTask{
		DBTask: &queue.DBTask{
			Task: &ent.Task{
				Type:          queue.RelocateTaskType,
				CorrelationID: logging.CorrelationID(ctx),
				PrivateState:  string(stateBytes),
				PublicState:   &types.TaskPublicState{},
			},
			DirectOwner: inventory.UserFromContext(ctx),
		},
	}, nil
}

func NewRelocateTaskFromModel(model *ent.Task) queue.Task {
	return &RelocateTask{
		DBTask: &queue.DBTask{
			Task: model,
		},
	}
}

func (m *RelocateTask) Do(ctx context.Context) (task.Status, error) {
	dep := dependency.FromContext(ctx)
	m.l = dep.Logger()

	m.Lock()
	if m.progress == nil {
		m.progress = make(queue.Progresses)
	}
	m.Unlock()

	state := &RelocateTaskState{}
	if err := json.Unmarshal([]byte(m.State()), state); err != nil {
		return task.StatusError, fmt.Errorf("failed to unmarshal state: %w", err)
	}
	m.state = state

	if m.state.Total == 0 {
		m.state.Total = m.countEntities(ctx, dep)
		m.Lock()
		m.progress[ProgressTypeRelocateTransferCount] = &queue.Progress{Total: int64(m.state.Total)}
		m.Unlock()
	}

	if m.state.Done {
		return task.StatusCompleted, nil
	}

	dstPolicy, err := dep.StoragePolicyClient().GetPolicyByID(ctx, m.state.DstPolicyID)
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to get destination policy: %w", err)
	}
	if dstPolicy.Type == types.PolicyTypeLoadBalance {
		return task.StatusError, fmt.Errorf("destination policy cannot be a load balance policy: %w", queue.CriticalErr)
	}

	user := inventory.UserFromContext(ctx)
	fm := manager.NewFileManager(dep, user)
	defer fm.Recycle()

	dstDriver, err := fm.GetStorageDriver(ctx, dstPolicy)
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to get destination storage driver: %w", err)
	}

	for _, src := range m.state.Src {
		uri, err := fs.NewUriFromString(src)
		if err != nil {
			m.state.Failed++
			m.l.Warning("Failed to parse source %q: %s", src, err)
			continue
		}

		jobs := m.collectJobs(ctx, dep, fm, uri)
		for _, job := range jobs {
			err := m.relocateOne(ctx, dep, fm, dstDriver, dstPolicy, job)
			entry := &audit.LogEntry{
				Name:              job.file.Name(),
				From:              job.file.Uri(false).String(),
				StoragePolicyID:   strconv.Itoa(dstPolicy.ID),
				StoragePolicyName: dstPolicy.Name,
			}
			opts := []audit.Option{audit.WithFile(job.file.ID())}
			entityID := 0
			if job.entity != nil {
				entityID = job.entity.ID()
				opts = append(opts, audit.WithEntity(entityID))
			}

			if err != nil {
				m.state.Failed++
				m.l.Warning("Failed to relocate entity %d of %s: %s", entityID, job.file.Name(), err)
				entry.Failed = true
				entry.Error = err.Error()
				audit.Record(ctx, audit.EventRelocate, entry, opts...)
			} else {
				audit.Record(ctx, audit.EventRelocate, entry, opts...)
			}
			atomic.AddInt64(&m.progress[ProgressTypeRelocateTransferCount].Current, 1)
		}
	}

	m.state.Done = true
	newState, err := json.Marshal(m.state)
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to marshal state: %w", err)
	}
	m.Lock()
	m.Task.PrivateState = string(newState)
	m.Unlock()

	return task.StatusCompleted, nil
}

func (m *RelocateTask) countEntities(ctx context.Context, dep dependency.Dep) int {
	user := inventory.UserFromContext(ctx)
	fm := manager.NewFileManager(dep, user)
	defer fm.Recycle()

	total := 0
	for _, src := range m.state.Src {
		uri, err := fs.NewUriFromString(src)
		if err != nil {
			continue
		}
		total += len(m.collectJobs(ctx, dep, fm, uri))
	}
	return total
}

func (m *RelocateTask) collectJobs(ctx context.Context, dep dependency.Dep, fm manager.FileManager, uri *fs.URI) []relocateEntityJob {
	target, err := fm.Get(ctx, uri, dbfs.WithFileEntities())
	if err != nil {
		m.l.Warning("Failed to get source %q: %s", uri.String(), err)
		return nil
	}

	if target.Type() == types.FileTypeFile {
		return m.jobsForFile(target)
	}

	var jobs []relocateEntityJob
	// Every folder inside the selected subtree must be reassigned to the
	// destination policy, including empty folders that contain no entities.
	jobs = append(jobs, relocateEntityJob{file: target})
	if err := fm.Walk(ctx, uri, intsets.MaxInt, func(file fs.File, level int) error {
		if file.Type() == types.FileTypeFolder {
			jobs = append(jobs, relocateEntityJob{file: file})
		} else {
			jobs = append(jobs, m.jobsForFile(file)...)
		}
		return nil
	}, dbfs.WithFileEntities()); err != nil {
		m.l.Warning("Failed to walk source %q: %s", uri.String(), err)
	}
	return jobs
}

func (m *RelocateTask) jobsForFile(file fs.File) []relocateEntityJob {
	entities := file.Entities()
	if len(entities) == 0 {
		return nil
	}

	jobs := make([]relocateEntityJob, 0, len(entities))
	for _, entity := range entities {
		primary := []int{file.ID()}
		if entity.ID() != file.PrimaryEntityID() {
			primary = nil
		}
		jobs = append(jobs, relocateEntityJob{
			entity:       entity,
			file:         file,
			parentFiles:  []int{file.ID()},
			primaryFiles: primary,
		})
	}
	return jobs
}

func (m *RelocateTask) relocateOne(ctx context.Context, dep dependency.Dep, fm manager.FileManager,
	dstDriver driver.Handler, dstPolicy *ent.StoragePolicy, job relocateEntityJob) error {
	if job.entity == nil {
		return m.relocateFolder(ctx, dep, job.file, dstPolicy)
	}

	if job.entity.PolicyID() == dstPolicy.ID {
		return nil
	}
	if job.entity.UploadSessionID() != nil {
		return fmt.Errorf("entity is still uploading")
	}

	srcPolicyID := job.entity.PolicyID()
	src, err := fm.GetEntitySource(ctx, job.entity.ID())
	if err != nil {
		return fmt.Errorf("failed to open entity source: %w", err)
	}
	defer src.Close()

	var encryptMetadata *types.EncryptMetadata
	var (
		uploadSource io.ReadCloser = src
		uploadSeeker io.Seeker     = src
	)
	if dstPolicy.Settings != nil && (dstPolicy.Settings.Encryption || dstPolicy.Settings.Relay) {
		cryptor, err := dep.EncryptorFactory(ctx)(types.CipherAES256CTR)
		if err != nil {
			return fmt.Errorf("failed to create encryptor: %w", err)
		}
		encryptMetadata, err = cryptor.GenerateMetadata(ctx)
		if err != nil {
			return fmt.Errorf("failed to generate encryption metadata: %w", err)
		}
		if err := cryptor.LoadMetadata(ctx, encryptMetadata); err != nil {
			return fmt.Errorf("failed to load encryption metadata: %w", err)
		}
		if err := cryptor.SetSource(src, src, job.entity.Size(), 0); err != nil {
			return fmt.Errorf("failed to set encrypted source: %w", err)
		}
		uploadSource = cryptor
		uploadSeeker = cryptor
	}

	savePath := relocateSavePath(dstPolicy, job.file, dep, ctx)
	req := &fs.UploadRequest{
		Props: &fs.UploadProps{
			Uri:             job.file.Uri(false),
			Size:            job.entity.Size(),
			SavePath:        savePath,
			UploadSessionID: uuid.Must(uuid.NewV4()).String(),
		},
		Mode:   fs.ModeOverwrite,
		File:   uploadSource,
		Seeker: uploadSeeker,
	}
	if err := dstDriver.Put(ctx, req); err != nil {
		return fmt.Errorf("failed to write entity to destination policy: %w", err)
	}

	args := &inventory.RelocateEntityParameter{
		Entity:                   job.entity.Model(),
		NewSource:                savePath,
		NewStoragePolicyID:       dstPolicy.ID,
		NewSize:                  job.entity.Size(),
		ParentFiles:              job.parentFiles,
		PrimaryEntityParentFiles: job.primaryFiles,
		EncryptMetadata:          encryptMetadata,
	}
	if err := dep.FileClient().RelocateEntity(ctx, args); err != nil {
		_, _ = dstDriver.Delete(ctx, savePath)
		return err
	}

	if !src.IsLocal() || src.LocalPath(ctx) != dstDriver.LocalPath(ctx, savePath) {
		if _, err := srcLocalizeDelete(ctx, dep, srcPolicyID, job.entity.Source()); err != nil {
			m.l.Warning("Failed to remove old entity object %d: %s", job.entity.ID(), err)
		}
	}

	return nil
}

func (m *RelocateTask) relocateFolder(ctx context.Context, dep dependency.Dep, file fs.File, dstPolicy *ent.StoragePolicy) error {
	args := &inventory.RelocateFilePolicyParameter{
		FileIDs:         []int{file.ID()},
		StoragePolicyID: dstPolicy.ID,
	}
	if err := dep.FileClient().RelocateFilePolicy(ctx, args); err != nil {
		return err
	}
	return nil
}

func srcLocalizeDelete(ctx context.Context, dep dependency.Dep, policyID int, source string) (bool, error) {
	policy, err := dep.StoragePolicyClient().GetPolicyByID(ctx, policyID)
	if err != nil {
		return false, err
	}
	fm := manager.NewFileManager(dep, nil)
	defer fm.Recycle()
	d, err := fm.GetStorageDriver(ctx, policy)
	if err != nil {
		return false, err
	}
	failed, err := d.Delete(ctx, source)
	if len(failed) > 0 {
		return false, fmt.Errorf("failed to delete objects: %v", failed)
	}
	return true, err
}

func relocateSavePath(policy *ent.StoragePolicy, file fs.File, dep dependency.Dep, ctx context.Context) string {
	now := time.Now()
	userID := file.OwnerID()
	dynamicReplace := func(rule string, pathAvailable bool) string {
		return util.ReplaceMagicVar(rule, fs.Separator, pathAvailable, false, now, userID,
			file.Name(), file.Uri(false).Dir(), "")
	}
	dirRule := dynamicReplace(filepath.ToSlash(policy.DirNameRule), true)
	nameRule := dynamicReplace(policy.FileNameRule, false)
	return path.Join(path.Clean(dirRule), nameRule)
}

func (m *RelocateTask) Progress(ctx context.Context) queue.Progresses {
	m.Lock()
	defer m.Unlock()
	return m.progress
}

func (m *RelocateTask) Summarize(hasher hashid.Encoder) *queue.Summary {
	if m.state == nil {
		if err := json.Unmarshal([]byte(m.State()), &m.state); err != nil {
			return nil
		}
	}

	props := map[string]any{
		SummaryKeySrcDstPolicyID: hashid.EncodePolicyID(hasher, m.state.DstPolicyID),
		SummaryKeyFailed:         m.state.Failed,
		SummaryKeyTotal:          m.state.Total,
	}
	if m.state.SrcMultiple {
		props[SummaryKeySrcMultiple] = m.state.Src
	} else if len(m.state.Src) == 1 {
		props[SummaryKeySrcStr] = m.state.Src[0]
	}

	return &queue.Summary{Props: props}
}

func (m *RelocateTask) Cleanup(ctx context.Context) error {
	return nil
}
