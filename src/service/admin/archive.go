package admin

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/pkg/auth"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/cloudreve/Cloudreve/v4/pkg/util"
	"github.com/gin-gonic/gin"
)

const (
	archiveStatusPending   = "pending"
	archiveStatusRunning   = "running"
	archiveStatusCompleted = "completed"
	archiveStatusFailed    = "failed"

	archiveRetention = 24 * time.Hour
)

type (
	// ArchiveEventsService creates an audit log archive.
	ArchiveEventsService struct {
		NotBefore  string `json:"not_before" binding:"required"`
		NotAfter   string `json:"not_after" binding:"required"`
		Categories []int  `json:"categories"`
	}
	ArchiveEventsParamCtx struct{}

	// ArchiveEventStatusService polls an archive job.
	ArchiveEventStatusService struct {
		ID string `uri:"id" binding:"required"`
	}
	ArchiveEventStatusParamCtx struct{}

	// ArchiveEventDeleteService deletes archived logs and the archive file.
	ArchiveEventDeleteService struct {
		ID string `uri:"id" binding:"required"`
	}
	ArchiveEventDeleteParamCtx struct{}

	// ArchiveEventDownloadService serves a signed archive download.
	ArchiveEventDownloadService struct {
		ID string `uri:"id" binding:"required"`
	}
	ArchiveEventDownloadParamCtx struct{}

	// CleanupEventsService deletes audit logs in a date range.
	CleanupEventsService struct {
		NotAfter   string `json:"not_after" binding:"required"`
		Categories []int  `json:"categories"`
	}
	CleanupEventsParamCtx struct{}

	// BatchDeleteEventsService deletes audit logs by id.
	BatchDeleteEventsService struct {
		IDs []int `json:"ids" binding:"required"`
	}
	BatchDeleteEventsParamCtx struct{}

	// ArchiveJobResponse is the archive job view returned to clients.
	ArchiveJobResponse struct {
		ID        string    `json:"id"`
		Status    string    `json:"status"`
		URL       string    `json:"url,omitempty"`
		Error     string    `json:"error,omitempty"`
		Expires   time.Time `json:"expires"`
		Size      int64     `json:"size,omitempty"`
		NotBefore time.Time `json:"not_before"`
		NotAfter  time.Time `json:"not_after"`
	}

	archiveJob struct {
		id         string
		status     string
		path       string
		size       int64
		expires    time.Time
		notBefore  time.Time
		notAfter   time.Time
		categories []int
		err        string
	}
)

var (
	archiveJobsMu sync.Mutex
	archiveJobs   = map[string]*archiveJob{}
)

func newArchiveID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func auditLogToJSON(log *ent.AuditLog) map[string]any {
	item := map[string]any{
		"id":         log.ID,
		"type":       log.Type,
		"ip":         log.IP,
		"created_at": log.CreatedAt,
	}
	if log.CorrelationID != nil {
		item["correlation_id"] = log.CorrelationID.String()
	}
	if log.Content != nil {
		item["content"] = log.Content
	}
	if log.UserAuditLogs != nil {
		item["user_id"] = *log.UserAuditLogs
	}
	if log.FileAuditLogs != nil {
		item["file_id"] = *log.FileAuditLogs
	}
	if log.EntityAuditLogs != nil {
		item["entity_id"] = *log.EntityAuditLogs
	}
	if log.ShareAuditLogs != nil {
		item["share_id"] = *log.ShareAuditLogs
	}
	return item
}

func createArchiveFile(ctx context.Context, dep dependency.Dep, job *archiveJob, types []int) error {
	logs, err := dep.AuditLogClient().Archive(ctx, &job.notBefore, &job.notAfter, types)
	if err != nil {
		return fmt.Errorf("failed to query audit logs: %w", err)
	}

	dir := filepath.Join(util.RelativePath(dep.SettingProvider().TempPath(ctx)), "audit_archive")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create archive dir: %w", err)
	}

	path := filepath.Join(dir, job.id+".zip")
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create archive file: %w", err)
	}
	defer file.Close()

	zw := zip.NewWriter(file)
	entries, err := zw.Create("events.jsonl")
	if err != nil {
		_ = zw.Close()
		return fmt.Errorf("failed to create archive entry: %w", err)
	}

	encoder := json.NewEncoder(entries)
	for _, log := range logs {
		if err := encoder.Encode(auditLogToJSON(log)); err != nil {
			_ = zw.Close()
			return fmt.Errorf("failed to encode audit log: %w", err)
		}
	}

	manifest, err := zw.Create("manifest.json")
	if err == nil {
		_ = json.NewEncoder(manifest).Encode(map[string]any{
			"generated_at": time.Now(),
			"not_before":   job.notBefore,
			"not_after":    job.notAfter,
			"categories":   types,
			"count":        len(logs),
			"version":      1,
		})
	}

	if err := zw.Close(); err != nil {
		return fmt.Errorf("failed to finalize archive: %w", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat archive: %w", err)
	}

	job.path = path
	job.size = info.Size()
	return nil
}

// Create starts an audit log archive job.
func (s *ArchiveEventsService) Create(c *gin.Context) (*ArchiveJobResponse, error) {
	dep := dependency.FromContext(c)
	notBefore, err := time.Parse(time.RFC3339, s.NotBefore)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeParamErr, "Invalid not_before time", err)
	}
	notAfter, err := time.Parse(time.RFC3339, s.NotAfter)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeParamErr, "Invalid not_after time", err)
	}
	if !notAfter.After(notBefore) {
		return nil, serializer.NewError(serializer.CodeParamErr, "Invalid archive time range", nil)
	}

	job := &archiveJob{
		id:         newArchiveID(),
		status:     archiveStatusPending,
		expires:    time.Now().Add(archiveRetention),
		notBefore:  notBefore,
		notAfter:   notAfter,
		categories: append([]int(nil), s.Categories...),
	}

	archiveJobsMu.Lock()
	archiveJobs[job.id] = job
	archiveJobsMu.Unlock()

	// Build the archive asynchronously, mirroring Pro's polling workflow.
	go func() {
		archiveJobsMu.Lock()
		job.status = archiveStatusRunning
		archiveJobsMu.Unlock()

		if err := createArchiveFile(context.Background(), dep, job, s.Categories); err != nil {
			archiveJobsMu.Lock()
			job.status = archiveStatusFailed
			job.err = err.Error()
			archiveJobsMu.Unlock()
			return
		}

		archiveJobsMu.Lock()
		job.status = archiveStatusCompleted
		archiveJobsMu.Unlock()
	}()

	res := buildArchiveResponse(c, dep, job)
	return &res, nil
}

func buildArchiveResponse(c *gin.Context, dep dependency.Dep, job *archiveJob) ArchiveJobResponse {
	res := ArchiveJobResponse{
		ID:        job.id,
		Status:    job.status,
		Error:     job.err,
		Expires:   job.expires,
		Size:      job.size,
		NotBefore: job.notBefore,
		NotAfter:  job.notAfter,
	}

	if job.status == archiveStatusCompleted {
		base := dep.SettingProvider().SiteURL(c)
		base.Path = strings.TrimSuffix(base.Path, "/") +
			fmt.Sprintf("/api/v4/admin/event/archive/%s/download", job.id)
		expire := job.expires
		if signed, err := auth.SignURI(c, dep.GeneralAuth(), base.String(), &expire); err == nil {
			res.URL = signed.String()
		}
	}

	return res
}

// Status returns the archive job status.
func (s *ArchiveEventStatusService) Status(c *gin.Context) (*ArchiveJobResponse, error) {
	dep := dependency.FromContext(c)
	archiveJobsMu.Lock()
	job, ok := archiveJobs[s.ID]
	archiveJobsMu.Unlock()
	if !ok {
		return nil, serializer.NewError(serializer.CodeNotFound, "Archive job not found", nil)
	}

	res := buildArchiveResponse(c, dep, job)
	return &res, nil
}

// Download serves the archive file. The route validates the URL signature.
func (s *ArchiveEventDownloadService) Download(c *gin.Context) error {
	archiveJobsMu.Lock()
	job, ok := archiveJobs[s.ID]
	archiveJobsMu.Unlock()
	if !ok || job.status != archiveStatusCompleted || job.path == "" {
		return serializer.NewError(serializer.CodeNotFound, "Archive not found", nil)
	}
	if time.Now().After(job.expires) {
		return serializer.NewError(serializer.CodeNotFound, "Archive expired", nil)
	}

	c.FileAttachment(job.path, fmt.Sprintf("audit-events-%s.zip", job.id))
	return nil
}

// Delete removes archived logs from the database and deletes the archive file.
func (s *ArchiveEventDeleteService) Delete(c *gin.Context) error {
	dep := dependency.FromContext(c)

	archiveJobsMu.Lock()
	job, ok := archiveJobs[s.ID]
	archiveJobsMu.Unlock()
	if !ok {
		return serializer.NewError(serializer.CodeNotFound, "Archive job not found", nil)
	}

	if _, err := dep.AuditLogClient().DeleteByRange(c, &job.notBefore, &job.notAfter, job.categories); err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to delete archived logs", err)
	}

	if job.path != "" {
		_ = os.Remove(job.path)
	}

	archiveJobsMu.Lock()
	delete(archiveJobs, s.ID)
	archiveJobsMu.Unlock()
	return nil
}

// Cleanup deletes audit logs in the given range.
func (s *CleanupEventsService) Cleanup(c *gin.Context) error {
	dep := dependency.FromContext(c)
	notAfter, err := time.Parse(time.RFC3339, s.NotAfter)
	if err != nil {
		return serializer.NewError(serializer.CodeParamErr, "Invalid not_after time", err)
	}
	if _, err := dep.AuditLogClient().DeleteByRange(c, nil, &notAfter, s.Categories); err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to cleanup audit logs", err)
	}
	return nil
}

// BatchDelete deletes audit logs by id.
func (s *BatchDeleteEventsService) BatchDelete(c *gin.Context) error {
	dep := dependency.FromContext(c)
	if _, err := dep.AuditLogClient().DeleteByIDs(c, s.IDs); err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to delete audit logs", err)
	}
	return nil
}
