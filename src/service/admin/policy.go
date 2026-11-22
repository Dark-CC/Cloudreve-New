package admin

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cloudreve/Cloudreve/v4/application/constants"
	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/cluster/routes"
	"github.com/cloudreve/Cloudreve/v4/pkg/credmanager"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/driver/cos"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/driver/ks3"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/driver/obs"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/driver/onedrive"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/driver/oneyunpan"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/driver/oss"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/driver/s3"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/manager"
	"github.com/cloudreve/Cloudreve/v4/pkg/logging"
	"github.com/cloudreve/Cloudreve/v4/pkg/util"

	"github.com/cloudreve/Cloudreve/v4/pkg/request"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/gin-gonic/gin"
)

// PathTestService 本地路径测试服务
type PathTestService struct {
	Path string `json:"path" binding:"required"`
}

// SlaveTestService 从机测试服务
type SlaveTestService struct {
	Secret string `json:"secret" binding:"required"`
	Server string `json:"server" binding:"required"`
}

type (
	SlavePingParameterCtx struct{}
	// SlavePingService ping slave node
	SlavePingService struct {
		Callback string `json:"callback" binding:"required"`
	}
)

// AddPolicyService 存储策略添加服务
type AddPolicyService struct {
	//Policy model.Policy `json:"policy" binding:"required"`
}

// PolicyService 存储策略ID服务
type PolicyService struct {
	ID     uint   `uri:"id" json:"id" binding:"required"`
	Region string `json:"region"`
}

// Delete 删除存储策略
func (service *SingleStoragePolicyService) Delete(c *gin.Context) error {
	// 禁止删除默认策略
	if service.ID == 1 {
		return serializer.NewError(serializer.CodeDeleteDefaultPolicy, "", nil)
	}

	dep := dependency.FromContext(c)
	storagePolicyClient := dep.StoragePolicyClient()

	ctx := context.WithValue(c, inventory.LoadStoragePolicyGroup{}, true)
	ctx = context.WithValue(ctx, inventory.SkipStoragePolicyCache{}, true)
	policy, err := storagePolicyClient.GetPolicyByID(ctx, service.ID)
	if err != nil {
		return serializer.NewError(serializer.CodePolicyNotExist, "", err)
	}

	// If policy is used by groups, return error. Group policies can be
	// configured through group_storage_policies in addition to the legacy
	// groups.storage_policy_id relation, so both sources must be checked.
	groupCount, err := countPolicyBoundGroups(c, dep, service.ID)
	if err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to check policy group bindings", err)
	}
	if groupCount == 0 {
		groupCount = len(policy.Edges.Groups)
	}
	if groupCount > 0 {
		return serializer.NewError(serializer.CodePolicyUsedByGroups, strconv.Itoa(groupCount), nil)
	}

	used, err := dep.FileClient().IsStoragePolicyUsedByEntities(ctx, service.ID)
	if err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to check if policy is used by entities", err)
	}

	if used {
		return serializer.NewError(serializer.CodePolicyUsedByFiles, "", nil)
	}

	err = storagePolicyClient.Delete(ctx, policy)
	if err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to delete policy", err)
	}

	forgetOneyunpanClient(policy)

	return nil
}

// Test 从机响应ping
func (service *SlavePingService) Test(c *gin.Context) error {
	master, err := url.Parse(service.Callback)
	if err != nil {
		return serializer.NewError(serializer.CodeParamErr, "Failed to parse callback url", err)
	}

	dep := dependency.FromContext(c)
	r := dep.RequestClient()
	res, err := r.Request(
		"GET",
		routes.MasterPingUrl(master).String(),
		nil,
		request.WithContext(c),
		request.WithLogger(logging.FromContext(c)),
		request.WithCorrelationID(),
		request.WithTimeout(time.Duration(10)*time.Second),
	).DecodeResponse()

	if err != nil {
		return serializer.NewError(serializer.CodeSlavePingMaster, err.Error(), nil)
	}

	version := constants.BackendVersion

	if strings.TrimSuffix(res.Data.(string), "-pro") != version {
		return serializer.NewError(serializer.CodeVersionMismatch, "Master: "+res.Data.(string)+", Slave: "+version, nil)
	}

	return nil
}

// Test 测试从机通信
func (service *SlaveTestService) Test() serializer.Response {
	//slave, err := url.Parse(service.Server)
	//if err != nil {
	//	return serializer.ParamErrDeprecated("Failed to parse slave node server URL: "+err.Error(), nil)
	//}
	//
	//controller, _ := url.Parse("/api/v3/slave/ping")
	//
	//// 请求正文
	//body := map[string]string{
	//	"callback": model.GetSiteURL().String(),
	//}
	//bodyByte, _ := json.Marshal(body)
	//
	//r := request.NewClientDeprecated()
	//res, err := r.Request(
	//	"POST",
	//	slave.ResolveReference(controller).String(),
	//	bytes.NewReader(bodyByte),
	//	request.WithTimeout(time.Duration(10)*time.Second),
	//	request.WithCredential(
	//		auth.HMACAuth{SecretKey: []byte(service.Secret)},
	//		int64(model.GetIntSetting("slave_api_timeout", 60)),
	//	),
	//).DecodeResponse()
	//if err != nil {
	//	return serializer.ParamErrDeprecated("Failed to connect to slave node: "+err.Error(), nil)
	//}
	//
	//if res.Code != 0 {
	//	return serializer.ParamErrDeprecated("Successfully connected to slave node, but slave returns: "+res.Msg, nil)
	//}

	return serializer.Response{}
}

// Test 测试本地路径
func (service *PathTestService) Test() serializer.Response {
	//policy := model.Policy{DirNameRule: service.Path}
	//path := policy.GeneratePath(1, "/My File")
	//path = filepath.Join(path, "test.txt")
	//file, err := util.CreatNestedFile(util.RelativePath(path))
	//if err != nil {
	//	return serializer.ParamErrDeprecated(fmt.Sprintf("Failed to create \"%s\": %s", path, err.Error()), nil)
	//}
	//
	//file.Close()
	//os.Remove(path)

	return serializer.Response{}
}

const (
	policyTypeCondition = "policy_type"
)

// Policies 列出存储策略
func (service *AdminListService) Policies(c *gin.Context) (*ListPolicyResponse, error) {
	dep := dependency.FromContext(c)
	storagePolicyClient := dep.StoragePolicyClient()

	ctx := context.WithValue(c, inventory.LoadStoragePolicyGroup{}, true)
	res, err := storagePolicyClient.ListPolicies(ctx, &inventory.ListPolicyParameters{
		PaginationArgs: &inventory.PaginationArgs{
			Page:     service.Page - 1,
			PageSize: service.PageSize,
			OrderBy:  service.OrderBy,
			Order:    inventory.OrderDirection(service.OrderDirection),
		},
		Type: types.PolicyType(service.Conditions[policyTypeCondition]),
	})

	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to list policies", err)
	}

	if err := enrichPolicyGroups(c, dep, res.Policies); err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to load policy groups", err)
	}

	return &ListPolicyResponse{
		Pagination: res.PaginationResults,
		Policies:   res.Policies,
	}, nil
}

// enrichPolicyGroups fills edges.groups from all group_storage_policies
// bindings. The ent relation only mirrors the legacy single policy column, so
// without this pass a policy bound through the multi-policy editor appears to
// have no groups in the admin UI.
func enrichPolicyGroups(c context.Context, dep dependency.Dep, policies []*ent.StoragePolicy) error {
	if len(policies) == 0 {
		return nil
	}

	groups, err := dep.GroupClient().ListAll(c)
	if err != nil {
		return err
	}
	groupByID := make(map[int]*ent.Group, len(groups))
	for _, group := range groups {
		groupByID[group.ID] = group
	}

	policyByID := make(map[int]*ent.StoragePolicy, len(policies))
	for _, policy := range policies {
		policyByID[policy.ID] = policy
	}

	for _, group := range groups {
		policyIDs, err := dep.GroupPolicyClient().ListGroupPolicies(c, group.ID)
		if err != nil {
			return err
		}
		for _, policyID := range policyIDs {
			policy := policyByID[policyID]
			if policy == nil {
				continue
			}
			if hasPolicyGroup(policy, group.ID) {
				continue
			}
			policy.Edges.Groups = append(policy.Edges.Groups, groupByID[group.ID])
		}
	}

	return nil
}

func hasPolicyGroup(policy *ent.StoragePolicy, groupID int) bool {
	for _, group := range policy.Edges.Groups {
		if group != nil && group.ID == groupID {
			return true
		}
	}
	return false
}

func countPolicyBoundGroups(c context.Context, dep dependency.Dep, policyID int) (int, error) {
	groups, err := dep.GroupClient().ListAll(c)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, group := range groups {
		policyIDs, err := dep.GroupPolicyClient().ListGroupPolicies(c, group.ID)
		if err != nil {
			return 0, err
		}
		for _, id := range policyIDs {
			if id == policyID {
				count++
				break
			}
		}
	}
	return count, nil
}

type (
	SingleStoragePolicyService struct {
		ID int `uri:"id" json:"id" binding:"required"`
	}
	GetStoragePolicyParamCtx struct{}
)

const (
	countEntityQuery = "countEntity"
)

func (service *SingleStoragePolicyService) Get(c *gin.Context) (*GetStoragePolicyResponse, error) {
	dep := dependency.FromContext(c)
	storagePolicyClient := dep.StoragePolicyClient()

	ctx := context.WithValue(c, inventory.LoadStoragePolicyGroup{}, true)
	ctx = context.WithValue(ctx, inventory.SkipStoragePolicyCache{}, true)
	policy, err := storagePolicyClient.GetPolicyByID(ctx, service.ID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to get policy", err)
	}

	// The ent relation only mirrors groups.storage_policy_id, so a policy bound
	// through the multi-policy group editor looks unbound on this page. Fill in
	// the real bindings the same way the policy list does.
	if err := enrichPolicyGroups(c, dep, []*ent.StoragePolicy{policy}); err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to load policy groups", err)
	}

	res := &GetStoragePolicyResponse{StoragePolicy: policy}
	if c.Query(countEntityQuery) != "" {
		count, size, err := dep.FileClient().CountEntityByStoragePolicyID(ctx, service.ID)
		if err != nil {
			return nil, serializer.NewError(serializer.CodeDBError, "Failed to count entities", err)
		}
		res.EntitiesCount = count
		res.EntitiesSize = size
	}

	return res, nil
}

type (
	CreateStoragePolicyService struct {
		Policy *ent.StoragePolicy `json:"policy" binding:"required"`
	}
	CreateStoragePolicyParamCtx struct{}
)

func (service *CreateStoragePolicyService) Create(c *gin.Context) (*GetStoragePolicyResponse, error) {
	dep := dependency.FromContext(c)
	storagePolicyClient := dep.StoragePolicyClient()

	if service.Policy.Type == types.PolicyTypeLocal {
		service.Policy.DirNameRule = util.DataPath("uploads/{uid}/{path}")
	}

	if service.Policy.Type == types.PolicyTypeOneyunpan {
		if err := validateOneyunpanPolicy(c, service.Policy); err != nil {
			return nil, err
		}
	}

	service.Policy.ID = 0
	policy, err := storagePolicyClient.Upsert(c, service.Policy)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to create policy", err)
	}

	return &GetStoragePolicyResponse{StoragePolicy: policy}, nil
}

type (
	UpdateStoragePolicyService struct {
		Policy *ent.StoragePolicy `json:"policy" binding:"required"`
	}
	UpdateStoragePolicyParamCtx struct{}
)

// validateOneyunpanPolicy pins the endpoints 1yunpan requires, verifies the
// supplied account, and derives the upload mode from the site-wide edge
// configuration. Operators cannot override these values from the policy form:
// 1yunpan only has one public API/worker pair, and the edge ticket key belongs
// to the deployment rather than to a single policy.
func validateOneyunpanPolicy(c *gin.Context, policy *ent.StoragePolicy) error {
	if policy.Settings == nil {
		policy.Settings = &types.PolicySetting{}
	}

	dep := dependency.FromContext(c)
	// Resolve the edge configuration before wiping the legacy per-policy copy,
	// so an install that has not been seeded yet still saves successfully.
	edge := oneyunpan.ResolveEdgeConfig(c, dep.SettingProvider(), policy)

	policy.Server = oneyunpan.DefaultAPIEndpoint
	policy.Settings.OneyunpanDownloadEndpoint = oneyunpan.DefaultDownloadEndpoint
	policy.Settings.OneyunpanUploadEndpoint = oneyunpan.DefaultUploadEndpoint
	// The edge settings are site-wide now; keep the policy copy empty so the
	// ticket key never gets duplicated across policies.
	policy.Settings.OneyunpanEdgeEndpoint = ""
	policy.Settings.OneyunpanTicketKey = ""
	policy.Settings.OneyunpanEdgeSite = ""

	if !edge.Enabled() {
		// Without an edge endpoint the driver can only stream through this node.
		policy.Settings.Relay = true
	} else {
		policy.Settings.Relay = false
		if _, err := oneyunpan.DecodeTicketKey(edge.TicketKey); err != nil {
			return serializer.NewError(serializer.CodeParamErr, "1yunpan edge upload requires a 32-byte ticket key (base64)", err)
		}
		if !isAllowedEdgeEndpoint(edge.Endpoint) {
			return serializer.NewError(serializer.CodeParamErr, "1yunpan edge endpoint must be an https URL (http is only allowed for localhost)", nil)
		}
		if strings.Contains(edge.Site, ".") {
			return serializer.NewError(serializer.CodeParamErr, "1yunpan edge site id must not contain a dot", nil)
		}
	}
	if policy.Settings.ChunkSize <= 0 {
		policy.Settings.ChunkSize = 25 << 20
	}

	client, err := oneyunpan.NewClient(
		policy.Server,
		policy.Settings.OneyunpanDownloadEndpoint,
		policy.AccessKey,
		policy.SecretKey,
	)
	if err != nil {
		return serializer.NewError(serializer.CodeParamErr, err.Error(), err)
	}

	if _, err := client.List(c, ""); err != nil {
		return serializer.NewError(serializer.CodeParamErr, "Failed to authenticate with 1yunpan: "+err.Error(), err)
	}
	return nil
}

// isAllowedEdgeEndpoint requires https everywhere except local development,
// where the edge may be served over plain http on the loopback interface.
func isAllowedEdgeEndpoint(raw string) bool {
	endpoint := strings.TrimSpace(raw)
	if strings.HasPrefix(endpoint, "https://") {
		return true
	}

	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "http" {
		return false
	}
	host := u.Hostname()
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// forgetOneyunpanClient drops the cached 1yunpan login for a policy's previous
// credential set so rotated passwords or endpoints are used right away.
func forgetOneyunpanClient(policy *ent.StoragePolicy) {
	if policy == nil || policy.Type != types.PolicyTypeOneyunpan {
		return
	}

	downloadHost := ""
	if policy.Settings != nil {
		downloadHost = policy.Settings.OneyunpanDownloadEndpoint
	}
	oneyunpan.ForgetCredentials(policy.Server, downloadHost, policy.AccessKey, policy.SecretKey)
}

func (service *UpdateStoragePolicyService) Update(c *gin.Context) (*GetStoragePolicyResponse, error) {
	dep := dependency.FromContext(c)
	storagePolicyClient := dep.StoragePolicyClient()

	id := c.Param("id")
	if id == "" {
		return nil, serializer.NewError(serializer.CodeParamErr, "ID is required", nil)
	}
	idInt, err := strconv.Atoi(id)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeParamErr, "Invalid ID", err)
	}

	service.Policy.ID = idInt

	if service.Policy.Type == types.PolicyTypeOneyunpan {
		if err := validateOneyunpanPolicy(c, service.Policy); err != nil {
			return nil, err
		}
	}

	if previous, err := storagePolicyClient.GetPolicyByID(c, idInt); err == nil && previous.Type == types.PolicyTypeOneyunpan {
		// Credentials may be rotated; drop the old cached login so the change
		// takes effect immediately.
		forgetOneyunpanClient(previous)
	}

	sc, tx, ctx, err := inventory.WithTx(c, storagePolicyClient)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to create transaction", err)
	}

	_, err = sc.Upsert(ctx, service.Policy)
	if err != nil {
		_ = inventory.Rollback(tx)
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to update policy", err)
	}

	if err := inventory.Commit(tx); err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to commit transaction", err)
	}

	_ = dep.KV().Delete(manager.EntityUrlCacheKeyPrefix)

	s := SingleStoragePolicyService{ID: idInt}
	return s.Get(c)
}

type (
	CreateStoragePolicyCorsService struct {
		Policy *ent.StoragePolicy `json:"policy" binding:"required"`
	}
	CreateStoragePolicyCorsParamCtx struct{}
)

func (service *CreateStoragePolicyCorsService) Create(c *gin.Context) error {
	dep := dependency.FromContext(c)

	switch service.Policy.Type {
	case types.PolicyTypeOss:
		handler, err := oss.New(c, service.Policy, dep.SettingProvider(), dep.ConfigProvider(), dep.Logger(), dep.MimeDetector(c))
		if err != nil {
			return serializer.NewError(serializer.CodeDBError, "Failed to create oss driver", err)
		}
		if err := handler.CORS(); err != nil {
			return serializer.NewError(serializer.CodeInternalSetting, "Failed to create cors: "+err.Error(), err)
		}

		return nil

	case types.PolicyTypeCos:
		handler, err := cos.New(c, service.Policy, dep.SettingProvider(), dep.ConfigProvider(), dep.Logger(), dep.MimeDetector(c))
		if err != nil {
			return serializer.NewError(serializer.CodeDBError, "Failed to create cos driver", err)
		}

		if err := handler.CORS(); err != nil {
			return serializer.NewError(serializer.CodeInternalSetting, "Failed to create cors: "+err.Error(), err)
		}

		return nil

	case types.PolicyTypeS3:
		handler, err := s3.New(c, service.Policy, dep.SettingProvider(), dep.ConfigProvider(), dep.Logger(), dep.MimeDetector(c))
		if err != nil {
			return serializer.NewError(serializer.CodeDBError, "Failed to create s3 driver", err)
		}

		if err := handler.CORS(); err != nil {
			return serializer.NewError(serializer.CodeInternalSetting, "Failed to create cors: "+err.Error(), err)
		}

		return nil

	case types.PolicyTypeKs3:
		handler, err := ks3.New(c, service.Policy, dep.SettingProvider(), dep.ConfigProvider(), dep.Logger(), dep.MimeDetector(c))
		if err != nil {
			return serializer.NewError(serializer.CodeDBError, "Failed to create ks3 driver", err)
		}

		if err := handler.CORS(); err != nil {
			return serializer.NewError(serializer.CodeInternalSetting, "Failed to create cors: "+err.Error(), err)
		}

		return nil
	case types.PolicyTypeObs:
		handler, err := obs.New(c, service.Policy, dep.SettingProvider(), dep.ConfigProvider(), dep.Logger(), dep.MimeDetector(c))
		if err != nil {
			return serializer.NewError(serializer.CodeDBError, "Failed to create obs driver", err)
		}

		if err := handler.CORS(); err != nil {
			return serializer.NewError(serializer.CodeInternalSetting, "Failed to create cors: "+err.Error(), err)
		}

		return nil
	default:
		return serializer.NewError(serializer.CodeParamErr, "Unsupported policy type", nil)
	}
}

type (
	GetOauthRedirectService struct {
		ID     int    `json:"id" binding:"required"`
		Secret string `json:"secret" binding:"required"`
		AppID  string `json:"app_id" binding:"required"`
	}
	GetOauthRedirectParamCtx struct{}
)

// GetOAuth 获取 OneDrive OAuth 地址
func (service *GetOauthRedirectService) GetOAuth(c *gin.Context) (string, error) {
	dep := dependency.FromContext(c)
	storagePolicyClient := dep.StoragePolicyClient()

	policy, err := storagePolicyClient.GetPolicyByID(c, service.ID)
	if err != nil || policy.Type != types.PolicyTypeOd {
		return "", serializer.NewError(serializer.CodePolicyNotExist, "", nil)
	}

	// Update to latest redirect url
	policy.Settings.OauthRedirect = routes.MasterPolicyOAuthCallback(dep.SettingProvider().SiteURL(c)).String()
	policy.SecretKey = service.Secret
	policy.BucketName = service.AppID
	policy, err = storagePolicyClient.Upsert(c, policy)
	if err != nil {
		return "", serializer.NewError(serializer.CodeDBError, "Failed to update policy", err)
	}

	client := onedrive.NewClient(policy, dep.RequestClient(), dep.CredManager(), dep.Logger(), dep.SettingProvider(), 0)
	redirect := client.OAuthURL(context.Background(), []string{
		"offline_access",
		"files.readwrite.all",
	})

	return redirect, nil
}

func GetPolicyOAuthURL(c *gin.Context) string {
	dep := dependency.FromContext(c)
	return routes.MasterPolicyOAuthCallback(dep.SettingProvider().SiteURL(c)).String()
}

// GetOauthCredentialStatus returns last refresh time of oauth credential
func (service *SingleStoragePolicyService) GetOauthCredentialStatus(c *gin.Context) (*OauthCredentialStatus, error) {
	dep := dependency.FromContext(c)
	storagePolicyClient := dep.StoragePolicyClient()

	policy, err := storagePolicyClient.GetPolicyByID(c, service.ID)
	if err != nil || policy.Type != types.PolicyTypeOd {
		return nil, serializer.NewError(serializer.CodePolicyNotExist, "", nil)
	}

	if policy.AccessKey == "" {
		return &OauthCredentialStatus{Valid: false}, nil
	}

	token, err := dep.CredManager().Obtain(c, onedrive.CredentialKey(policy.ID))
	if err != nil {
		if errors.Is(err, credmanager.ErrNotFound) {
			return &OauthCredentialStatus{Valid: false}, nil
		}

		return nil, serializer.NewError(serializer.CodeDBError, "Failed to get credential", err)
	}

	return &OauthCredentialStatus{Valid: true, LastRefreshTime: token.RefreshedAt()}, nil
}

type (
	FinishOauthCallbackService struct {
		Code  string `json:"code" binding:"required"`
		State string `json:"state" binding:"required"`
	}
	FinishOauthCallbackParamCtx struct{}
)

func (service *FinishOauthCallbackService) Finish(c *gin.Context) error {
	dep := dependency.FromContext(c)
	storagePolicyClient := dep.StoragePolicyClient()

	policyId, err := strconv.Atoi(service.State)
	if err != nil {
		return serializer.NewError(serializer.CodeParamErr, "Invalid state", err)
	}

	policy, err := storagePolicyClient.GetPolicyByID(c, policyId)
	if err != nil {
		return serializer.NewError(serializer.CodePolicyNotExist, "", nil)
	}

	if policy.Type != types.PolicyTypeOd {
		return serializer.NewError(serializer.CodeParamErr, "Invalid policy type", nil)
	}

	client := onedrive.NewClient(policy, dep.RequestClient(), dep.CredManager(), dep.Logger(), dep.SettingProvider(), 0)
	credential, err := client.ObtainToken(c, onedrive.WithCode(service.Code))
	if err != nil {
		return serializer.NewError(serializer.CodeParamErr, "Failed to obtain token: "+err.Error(), err)
	}

	credManager := dep.CredManager()
	err = credManager.Upsert(c, credential)
	if err != nil {
		return serializer.NewError(serializer.CodeInternalSetting, "Failed to upsert credential", err)
	}

	_, err = credManager.Obtain(c, onedrive.CredentialKey(policy.ID))
	if err != nil {
		return serializer.NewError(serializer.CodeInternalSetting, "Failed to obtain credential", err)
	}

	return nil
}

func (service *SingleStoragePolicyService) GetSharePointDriverRoot(c *gin.Context) (string, error) {
	dep := dependency.FromContext(c)
	storagePolicyClient := dep.StoragePolicyClient()

	policy, err := storagePolicyClient.GetPolicyByID(c, service.ID)
	if err != nil {
		return "", serializer.NewError(serializer.CodePolicyNotExist, "", nil)
	}

	if policy.Type != types.PolicyTypeOd {
		return "", serializer.NewError(serializer.CodeParamErr, "Invalid policy type", nil)
	}

	client := onedrive.NewClient(policy, dep.RequestClient(), dep.CredManager(), dep.Logger(), dep.SettingProvider(), 0)
	root, err := client.GetSiteIDByURL(c, c.Query("url"))
	if err != nil {
		return "", serializer.NewError(serializer.CodeInternalSetting, "Failed to get site id", err)
	}

	return fmt.Sprintf("sites/%s/drive", root), nil
}
