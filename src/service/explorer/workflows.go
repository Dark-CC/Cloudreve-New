package explorer

import (
	"encoding/gob"
	"github.com/cloudreve/Cloudreve/v4/pkg/hashid"
	"time"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/task"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/downloader"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs/dbfs"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/manager"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/workflows"
	"github.com/cloudreve/Cloudreve/v4/pkg/queue"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid"
	"github.com/samber/lo"
	"golang.org/x/tools/container/intsets"
)

// ItemMoveService 处理多文件/目录移动
type ItemMoveService struct {
	SrcDir string        `json:"src_dir" binding:"required,min=1,max=65535"`
	Src    ItemIDService `json:"src"`
	Dst    string        `json:"dst" binding:"required,min=1,max=65535"`
}

// ItemRenameService 处理多文件/目录重命名
type ItemRenameService struct {
	Src     ItemIDService `json:"src"`
	NewName string        `json:"new_name" binding:"required,min=1,max=255"`
}

// ItemService 处理多文件/目录相关服务
type ItemService struct {
	Items []uint `json:"items"`
	Dirs  []uint `json:"dirs"`
}

// ItemIDService 处理多文件/目录相关服务，字段值为HashID，可通过Raw()方法获取原始ID
type ItemIDService struct {
	Items      []string `json:"items"`
	Dirs       []string `json:"dirs"`
	Source     *ItemService
	Force      bool `json:"force"`
	UnlinkOnly bool `json:"unlink"`
}

// ItemDecompressService 文件解压缩任务服务
type ItemDecompressService struct {
	Src      string `json:"src"`
	Dst      string `json:"dst" binding:"required,min=1,max=65535"`
	Encoding string `json:"encoding"`
}

// ItemPropertyService 获取对象属性服务
type ItemPropertyService struct {
	ID        string `binding:"required"`
	TraceRoot bool   `form:"trace_root"`
	IsFolder  bool   `form:"is_folder"`
}

func init() {
	gob.Register(ItemIDService{})
}

type (
	DownloadWorkflowService struct {
		Src             []string `json:"src"`
		SrcFile         string   `json:"src_file"`
		Dst             string   `json:"dst" binding:"required"`
		NodeID          string   `json:"node_id"`
		PreferredNodeID string   `json:"preferred_node_id"`
	}
	CreateDownloadParamCtx struct{}
)

// resolveWorkflowNode validates an optional user selected node and returns its
// numeric ID. An empty node ID means the scheduler picks an eligible node.
func resolveWorkflowNode(c *gin.Context, nodeHashID string, capability types.NodeCapability) (int, error) {
	if nodeHashID == "" {
		return 0, nil
	}

	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)
	if user == nil || user.Edges.Group == nil || user.Edges.Group.Permissions == nil ||
		!user.Edges.Group.Permissions.Enabled(int(types.GroupPermissionSelectNode)) {
		return 0, serializer.NewError(serializer.CodeGroupNotAllowed, "Group not allowed to select node", nil)
	}

	nodeID, err := dep.HashIDEncoder().Decode(nodeHashID, hashid.NodeID)
	if err != nil {
		return 0, serializer.NewError(serializer.CodeParamErr, "Invalid node", err)
	}

	var subset []int
	if user.Edges.Group.Settings != nil {
		subset = user.Edges.Group.Settings.AvailableNodes
	}
	nodes, err := dep.NodeClient().ListActiveNodes(c, subset)
	if err != nil {
		return 0, serializer.NewError(serializer.CodeDBError, "Failed to list available nodes", err)
	}
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if node.ID != nodeID {
			continue
		}
		if node.Capabilities == nil || !node.Capabilities.Enabled(int(capability)) {
			return 0, serializer.NewError(serializer.CodeParamErr, "Selected node does not support this task", nil)
		}
		return nodeID, nil
	}

	return 0, serializer.NewError(serializer.CodeParamErr, "Selected node is not available", nil)
}

func selectWorkflowNodeID(preferredNodeID, nodeID string) string {
	if preferredNodeID != "" {
		return preferredNodeID
	}
	return nodeID
}

func (service *DownloadWorkflowService) CreateDownloadTask(c *gin.Context) ([]*TaskResponse, error) {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)
	hasher := dep.HashIDEncoder()
	m := manager.NewFileManager(dep, user)
	defer m.Recycle()

	if !user.Edges.Group.Permissions.Enabled(int(types.GroupPermissionRemoteDownload)) {
		return nil, serializer.NewError(serializer.CodeGroupNotAllowed, "Group not allowed to download files", nil)
	}

	// Src must be set
	if service.SrcFile == "" && len(service.Src) == 0 {
		return nil, serializer.NewError(serializer.CodeParamErr, "No source files", nil)
	}

	nodeID, err := resolveWorkflowNode(c, selectWorkflowNodeID(service.PreferredNodeID, service.NodeID), types.NodeCapabilityRemoteDownload)
	if err != nil {
		return nil, err
	}

	// Only one of src and src_file can be set
	if service.SrcFile != "" && len(service.Src) > 0 {
		return nil, serializer.NewError(serializer.CodeParamErr, "Invalid source files", nil)
	}

	dst, err := fs.NewUriFromString(service.Dst)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeParamErr, "Invalid destination", err)
	}

	// Validate dst
	_, err = m.Get(c, dst, dbfs.WithRequiredCapabilities(dbfs.NavigatorCapabilityCreateFile))
	if err != nil {
		return nil, serializer.NewError(serializer.CodeParamErr, "Invalid destination", err)
	}

	// 检查批量任务数量
	limit := user.Edges.Group.Settings.Aria2BatchSize
	if limit > 0 && len(service.Src) > limit {
		return nil, serializer.NewError(serializer.CodeBatchAria2Size, "", nil)
	}

	// Validate src file
	if service.SrcFile != "" {
		src, err := fs.NewUriFromString(service.SrcFile)
		if err != nil {
			return nil, serializer.NewError(serializer.CodeParamErr, "Invalid source file uri", err)
		}

		_, err = m.Get(c, src, dbfs.WithRequiredCapabilities(dbfs.NavigatorCapabilityDownloadFile))
		if err != nil {
			return nil, serializer.NewError(serializer.CodeParamErr, "Invalid source file", err)
		}
	}

	// batch creating tasks
	ae := serializer.NewAggregateError()
	tasks := make([]queue.Task, 0, len(service.Src))
	for _, src := range service.Src {
		if src == "" {
			continue
		}

		t, err := workflows.NewRemoteDownloadTask(c, src, service.SrcFile, service.Dst, nodeID)
		if err != nil {
			ae.Add(src, err)
			continue
		}

		if err := dep.RemoteDownloadQueue(c).QueueTask(c, t); err != nil {
			ae.Add(src, err)
		}

		tasks = append(tasks, t)
	}

	if service.SrcFile != "" {
		t, err := workflows.NewRemoteDownloadTask(c, "", service.SrcFile, service.Dst, nodeID)
		if err != nil {
			ae.Add(service.SrcFile, err)
		}

		if err := dep.RemoteDownloadQueue(c).QueueTask(c, t); err != nil {
			ae.Add(service.SrcFile, err)
		}

		tasks = append(tasks, t)
	}

	return lo.Map(tasks, func(item queue.Task, index int) *TaskResponse {
		return BuildTaskResponse(item, nil, hasher)
	}), ae.Aggregate()
}

type (
	ArchiveWorkflowService struct {
		Src             []string `json:"src" binding:"required"`
		Dst             string   `json:"dst" binding:"required"`
		Encoding        string   `json:"encoding"`
		Password        string   `json:"password"`
		FileMask        []string `json:"file_mask"`
		NodeID          string   `json:"node_id"`
		PreferredNodeID string   `json:"preferred_node_id"`
	}
	CreateArchiveParamCtx struct{}
)

func (service *ArchiveWorkflowService) CreateExtractTask(c *gin.Context) (*TaskResponse, error) {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)
	hasher := dep.HashIDEncoder()
	m := manager.NewFileManager(dep, user)
	defer m.Recycle()

	if !user.Edges.Group.Permissions.Enabled(int(types.GroupPermissionArchiveTask)) {
		return nil, serializer.NewError(serializer.CodeGroupNotAllowed, "Group not allowed to compress files", nil)
	}

	dst, err := fs.NewUriFromString(service.Dst)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeParamErr, "Invalid destination", err)
	}

	if len(service.Src) == 0 {
		return nil, serializer.NewError(serializer.CodeParamErr, "No source files", nil)
	}

	nodeID, err := resolveWorkflowNode(c, selectWorkflowNodeID(service.PreferredNodeID, service.NodeID), types.NodeCapabilityExtractArchive)
	if err != nil {
		return nil, err
	}

	// Validate destination
	if _, err := m.Get(c, dst, dbfs.WithRequiredCapabilities(dbfs.NavigatorCapabilityCreateFile)); err != nil {
		return nil, serializer.NewError(serializer.CodeParamErr, "Invalid destination", err)
	}

	// Create task
	t, err := workflows.NewExtractArchiveTask(c, service.Src[0], service.Dst, service.Encoding, service.Password, service.FileMask, nodeID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeCreateTaskError, "Failed to create task", err)
	}

	if err := dep.IoIntenseQueue(c).QueueTask(c, t); err != nil {
		return nil, serializer.NewError(serializer.CodeCreateTaskError, "Failed to queue task", err)
	}

	return BuildTaskResponse(t, nil, hasher), nil
}

// CreateCompressTask Create task to create an archive file
func (service *ArchiveWorkflowService) CreateCompressTask(c *gin.Context) (*TaskResponse, error) {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)
	hasher := dep.HashIDEncoder()
	m := manager.NewFileManager(dep, user)
	defer m.Recycle()

	if !user.Edges.Group.Permissions.Enabled(int(types.GroupPermissionArchiveTask)) {
		return nil, serializer.NewError(serializer.CodeGroupNotAllowed, "Group not allowed to compress files", nil)
	}

	dst, err := fs.NewUriFromString(service.Dst)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeParamErr, "Invalid destination", err)
	}

	nodeID, err := resolveWorkflowNode(c, selectWorkflowNodeID(service.PreferredNodeID, service.NodeID), types.NodeCapabilityCreateArchive)
	if err != nil {
		return nil, err
	}

	// Create a placeholder file then delete it to validate the destination
	session, err := m.PrepareUpload(c, &fs.UploadRequest{
		Props: &fs.UploadProps{
			Uri:             dst,
			Size:            0,
			UploadSessionID: uuid.Must(uuid.NewV4()).String(),
			ExpireAt:        time.Now().Add(time.Second * 3600),
		},
	})
	if err != nil {
		return nil, err
	}
	m.OnUploadFailed(c, session)

	// Create task
	t, err := workflows.NewCreateArchiveTask(c, service.Src, service.Dst, nodeID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeCreateTaskError, "Failed to create task", err)
	}

	if err := dep.IoIntenseQueue(c).QueueTask(c, t); err != nil {
		return nil, serializer.NewError(serializer.CodeCreateTaskError, "Failed to queue task", err)
	}

	return BuildTaskResponse(t, nil, hasher), nil
}

type (
	ImportWorkflowService struct {
		Src              string `json:"src" binding:"required"`
		Dst              string `json:"dst" binding:"required"`
		ExtractMediaMeta bool   `json:"extract_media_meta"`
		UserID           string `json:"user_id" binding:"required"`
		Recursive        bool   `json:"recursive"`
		PolicyID         int    `json:"policy_id" binding:"required"`
	}
	CreateImportParamCtx struct{}
)

type (
	RelocateWorkflowService struct {
		Src         []string `json:"src" binding:"required,min=1"`
		DstPolicyID string   `json:"dst_policy_id" binding:"required"`
	}
	CreateRelocateParamCtx struct{}
)

func (service *RelocateWorkflowService) CreateRelocateTask(c *gin.Context) (*TaskResponse, error) {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)
	hasher := dep.HashIDEncoder()

	if !user.Edges.Group.Permissions.Enabled(int(types.GroupPermissionRelocate)) {
		return nil, serializer.NewError(serializer.CodeGroupNotAllowed, "Group not allowed to relocate files", nil)
	}
	if len(service.Src) == 0 {
		return nil, serializer.NewError(serializer.CodeParamErr, "No source files", nil)
	}

	policyID, err := hasher.Decode(service.DstPolicyID, hashid.PolicyID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeParamErr, "Invalid storage policy", err)
	}

	policy, err := dep.StoragePolicyClient().GetPolicyByID(c, policyID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeParamErr, "Invalid storage policy", err)
	}
	if policy.Type == types.PolicyTypeLoadBalance {
		return nil, serializer.NewError(serializer.CodeParamErr, "Destination cannot be a load balance policy", nil)
	}

	allowedPolicyIDs, err := dep.GroupPolicyClient().ListGroupPolicies(c, user.Edges.Group.ID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to list group storage policies", err)
	}
	if len(allowedPolicyIDs) == 0 {
		groupPolicy, err := dep.StoragePolicyClient().GetByGroup(c, user.Edges.Group)
		if err != nil {
			return nil, serializer.NewError(serializer.CodeDBError, "Failed to get group storage policy", err)
		}
		allowedPolicyIDs = []int{groupPolicy.ID}
	}
	if !lo.Contains(allowedPolicyIDs, policyID) {
		return nil, serializer.NewError(serializer.CodeParamErr, "Storage policy is not available to your group", nil)
	}

	m := manager.NewFileManager(dep, user)
	defer m.Recycle()
	for _, src := range service.Src {
		uri, err := fs.NewUriFromString(src)
		if err != nil {
			return nil, serializer.NewError(serializer.CodeParamErr, "Invalid source", err)
		}
		if _, err := m.Get(c, uri, dbfs.WithFileEntities()); err != nil {
			return nil, serializer.NewError(serializer.CodeParamErr, "Invalid source", err)
		}
	}

	t, err := workflows.NewRelocateTask(c, service.Src, policyID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeCreateTaskError, "Failed to create task", err)
	}
	if err := dep.IoIntenseQueue(c).QueueTask(c, t); err != nil {
		return nil, serializer.NewError(serializer.CodeCreateTaskError, "Failed to queue task", err)
	}

	return BuildTaskResponse(t, nil, hasher), nil
}

func (service *ImportWorkflowService) CreateImportTask(c *gin.Context) (*TaskResponse, error) {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)
	hasher := dep.HashIDEncoder()
	m := manager.NewFileManager(dep, user)
	defer m.Recycle()

	if !user.Edges.Group.Permissions.Enabled(int(types.GroupPermissionIsAdmin)) {
		return nil, serializer.NewError(serializer.CodeGroupNotAllowed, "Only admin can import files", nil)
	}

	userId, err := hasher.Decode(service.UserID, hashid.UserID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeParamErr, "Invalid user id", err)
	}

	owner, err := dep.UserClient().GetLoginUserByID(c, userId)
	if err != nil || owner.ID == 0 {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to get user", err)
	}

	dst, err := fs.NewUriFromString(fs.NewMyUri(service.UserID))
	if err != nil {
		return nil, serializer.NewError(serializer.CodeParamErr, "Invalid destination", err)
	}

	// Create task
	t, err := workflows.NewImportTask(c, owner, service.Src, service.Recursive, dst.JoinRaw(service.Dst).String(), service.PolicyID, service.ExtractMediaMeta)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeCreateTaskError, "Failed to create task", err)
	}

	if err := dep.IoIntenseQueue(c).QueueTask(c, t); err != nil {
		return nil, serializer.NewError(serializer.CodeCreateTaskError, "Failed to queue task", err)
	}

	return BuildTaskResponse(t, nil, hasher), nil
}

type (
	ListTaskService struct {
		PageSize      int    `form:"page_size" binding:"required,min=10,max=100"`
		Category      string `form:"category" binding:"required,eq=general|eq=downloading|eq=downloaded"`
		NextPageToken string `form:"next_page_token"`
	}
	ListTaskParamCtx struct{}
)

func (service *ListTaskService) ListTasks(c *gin.Context) (*TaskListResponse, error) {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)
	hasher := dep.HashIDEncoder()
	taskClient := dep.TaskClient()

	args := &inventory.ListTaskArgs{
		PaginationArgs: &inventory.PaginationArgs{
			UseCursorPagination: true,
			PageToken:           service.NextPageToken,
			PageSize:            service.PageSize,
		},
		Types:  []string{queue.CreateArchiveTaskType, queue.ExtractArchiveTaskType, queue.RelocateTaskType, queue.ImportTaskType},
		UserID: user.ID,
	}

	if service.Category != "general" {
		args.Types = []string{queue.RemoteDownloadTaskType}
		if service.Category == "downloading" {
			args.PageSize = intsets.MaxInt
			args.Status = []task.Status{task.StatusSuspending, task.StatusProcessing, task.StatusQueued}
		} else if service.Category == "downloaded" {
			args.Status = []task.Status{task.StatusCanceled, task.StatusError, task.StatusCompleted}
		}
	}

	// Get tasks
	res, err := taskClient.List(c, args)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to query tasks", err)
	}

	tasks := make([]queue.Task, 0, len(res.Tasks))
	nodeMap := make(map[int]*ent.Node)
	for _, t := range res.Tasks {
		task, err := queue.NewTaskFromModel(t)
		if err != nil {
			return nil, serializer.NewError(serializer.CodeDBError, "Failed to parse task", err)
		}

		summary := task.Summarize(hasher)
		if summary != nil && summary.NodeID > 0 {
			if _, ok := nodeMap[summary.NodeID]; !ok {
				nodeMap[summary.NodeID] = nil
			}
		}
		tasks = append(tasks, task)
	}

	// Get nodes
	nodes, err := dep.NodeClient().ListActiveNodes(c, lo.Keys(nodeMap))
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to query nodes", err)
	}
	for _, n := range nodes {
		nodeMap[n.ID] = n
	}

	// Build response
	return BuildTaskListResponse(tasks, res, nodeMap, hasher), nil
}

func TaskPhaseProgress(c *gin.Context, taskID int) (queue.Progresses, error) {
	dep := dependency.FromContext(c)
	u := inventory.UserFromContext(c)
	r := dep.TaskRegistry()
	t, found := r.Get(taskID)
	if !found || (t.Owner().ID != u.ID && !u.Edges.Group.Permissions.Enabled(int(types.GroupPermissionIsAdmin))) {
		return queue.Progresses{}, nil
	}

	return t.Progress(c), nil
}

func CancelDownloadTask(c *gin.Context, taskID int) error {
	dep := dependency.FromContext(c)
	u := inventory.UserFromContext(c)
	r := dep.TaskRegistry()
	t, found := r.Get(taskID)
	if !found || t.Owner().ID != u.ID {
		return serializer.NewError(serializer.CodeNotFound, "Task not found", nil)
	}

	if downloadTask, ok := t.(*workflows.RemoteDownloadTask); ok {
		if err := downloadTask.CancelDownload(c); err != nil {
			return serializer.NewError(serializer.CodeInternalSetting, "Failed to cancel download task", err)
		}
	}

	return nil
}

type (
	SetDownloadFilesService struct {
		Files []*downloader.SetFileToDownloadArgs `json:"files" binding:"required"`
	}
	SetDownloadFilesParamCtx struct{}
)

func (service *SetDownloadFilesService) SetDownloadFiles(c *gin.Context, taskID int) error {
	dep := dependency.FromContext(c)
	u := inventory.UserFromContext(c)
	r := dep.TaskRegistry()

	t, found := r.Get(taskID)
	if !found || t.Owner().ID != u.ID {
		return serializer.NewError(serializer.CodeNotFound, "Task not found", nil)
	}

	status := t.Status()
	summary := t.Summarize(dep.HashIDEncoder())
	// Task must be in processing state
	if status != task.StatusSuspending && status != task.StatusProcessing {
		return serializer.NewError(serializer.CodeNotFound, "Task not in processing state", nil)
	}

	// Task must in monitoring loop
	if summary.Phase != workflows.RemoteDownloadTaskPhaseMonitor {
		return serializer.NewError(serializer.CodeNotFound, "Task not in monitoring loop", nil)
	}

	if downloadTask, ok := t.(*workflows.RemoteDownloadTask); ok {
		if err := downloadTask.SetDownloadTarget(c, service.Files...); err != nil {
			return serializer.NewError(serializer.CodeInternalSetting, "Failed to set download files", err)
		}
	}

	return nil
}

type (
	RebuildFTSIndexWorkflowService struct {
		FilteredStoragePolicy []int `json:"filtered_storage_policy"`
	}
	CreateRebuildFTSIndexParamCtx struct{}
)

func (service *RebuildFTSIndexWorkflowService) CreateRebuildFTSIndexTask(c *gin.Context) (*TaskResponse, error) {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)
	hasher := dep.HashIDEncoder()
	m := manager.NewFileManager(dep, user)
	defer m.Recycle()

	if !user.Edges.Group.Permissions.Enabled(int(types.GroupPermissionIsAdmin)) {
		return nil, serializer.NewError(serializer.CodeGroupNotAllowed, "Only admin can import files", nil)
	}

	// Create task
	t, err := workflows.NewRebuildIndexTask(c, user, service.FilteredStoragePolicy)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeCreateTaskError, "Failed to create task", err)
	}

	if err := dep.MediaMetaQueue(c).QueueTask(c, t); err != nil {
		return nil, serializer.NewError(serializer.CodeCreateTaskError, "Failed to queue task", err)
	}

	return BuildTaskResponse(t, nil, hasher), nil
}
