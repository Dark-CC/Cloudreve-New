package admin

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/boolset"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/cloudreve/Cloudreve/v4/service/audit"
	"github.com/gin-gonic/gin"
)

// AddGroupService 用户组添加服务
type AddGroupService struct {
	//Group model.Group `json:"group" binding:"required"`
}

// GroupService 用户组ID服务
type GroupService struct {
	ID uint `uri:"id" json:"id" binding:"required"`
}

// Get 获取用户组详情
func (service *GroupService) Get() serializer.Response {
	//group, err := model.GetGroupByID(service.ID)
	//if err != nil {
	//	return serializer.ErrDeprecated(serializer.CodeGroupNotFound, "", err)
	//}
	//
	//return serializer.Response{Data: group}

	return serializer.Response{}
}

// Delete 删除用户组
func (service *GroupService) Delete() serializer.Response {
	//// 查找用户组
	//group, err := model.GetGroupByID(service.ID)
	//if err != nil {
	//	return serializer.ErrDeprecated(serializer.CodeGroupNotFound, "", err)
	//}
	//
	//// 是否为系统用户组
	//if group.ID <= 3 {
	//	return serializer.ErrDeprecated(serializer.CodeInvalidActionOnSystemGroup, "", err)
	//}
	//
	//// 检查是否有用户使用
	//total := 0
	//row := model.DB.Model(&model.User{}).Where("group_id = ?", service.ID).
	//	Select("count(id)").Row()
	//row.Scan(&total)
	//if total > 0 {
	//	return serializer.ErrDeprecated(serializer.CodeGroupUsedByUser, strconv.Itoa(total), nil)
	//}
	//
	//model.DB.Delete(&group)

	return serializer.Response{}
}

func (service *SingleGroupService) Delete(c *gin.Context) error {
	if service.ID <= 3 {
		return serializer.NewError(serializer.CodeInvalidActionOnSystemGroup, "", nil)
	}

	dep := dependency.FromContext(c)
	groupClient := dep.GroupClient()

	// Any user still under this group?
	users, err := groupClient.CountUsers(c, int(service.ID))
	if err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to count users", err)
	}

	if users > 0 {
		return serializer.NewError(serializer.CodeGroupUsedByUser, strconv.Itoa(users), nil)
	}

	err = groupClient.Delete(c, service.ID)
	if err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to delete group", err)
	}

	audit.Record(c, audit.EventGroupChanged, &audit.LogEntry{
		GroupID: int(service.ID),
		Reason:  "delete",
	}, audit.WithUser(inventory.UserIDFromContext(c)))
	return nil
}

func (s *AdminListService) List(c *gin.Context) (*ListGroupResponse, error) {
	dep := dependency.FromContext(c)
	groupClient := dep.GroupClient()

	ctx := context.WithValue(c, inventory.LoadGroupPolicy{}, true)
	res, err := groupClient.ListGroups(ctx, &inventory.ListGroupParameters{
		PaginationArgs: &inventory.PaginationArgs{
			Page:     s.Page - 1,
			PageSize: s.PageSize,
			OrderBy:  s.OrderBy,
			Order:    inventory.OrderDirection(s.OrderDirection),
		},
	})

	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to list groups", err)
	}

	groups := make([]GetGroupResponse, 0, len(res.Groups))
	for _, group := range res.Groups {
		item := GetGroupResponse{Group: group}
		policies, err := dep.GroupPolicyClient().ListGroupPolicies(c, group.ID)
		if err != nil {
			return nil, serializer.NewError(serializer.CodeDBError, "Failed to load group storage policies", err)
		}
		item.StoragePolicyIDs = policies
		if item.StoragePolicies, err = loadGroupPolicyObjects(c, dep, policies); err != nil {
			return nil, err
		}
		groups = append(groups, item)
	}

	return &ListGroupResponse{
		Pagination: res.PaginationResults,
		Groups:     groups,
	}, nil
}

type (
	SingleGroupService struct {
		ID int `uri:"id" json:"id" binding:"required"`
	}
	SingleGroupParamCtx struct{}
)

const (
	countUserQuery = "countUser"
)

func (s *SingleGroupService) Get(c *gin.Context) (*GetGroupResponse, error) {
	dep := dependency.FromContext(c)
	groupClient := dep.GroupClient()

	ctx := context.WithValue(c, inventory.LoadGroupPolicy{}, true)
	group, err := groupClient.GetByID(ctx, s.ID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to get group", err)
	}

	res := &GetGroupResponse{Group: group}
	policies, err := dep.GroupPolicyClient().ListGroupPolicies(c, s.ID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to load group storage policies", err)
	}
	res.StoragePolicyIDs = policies
	if res.StoragePolicies, err = loadGroupPolicyObjects(c, dep, policies); err != nil {
		return nil, err
	}

	if c.Query(countUserQuery) != "" {
		totalUsers, err := groupClient.CountUsers(ctx, int(s.ID))
		if err != nil {
			return nil, serializer.NewError(serializer.CodeDBError, "Failed to count users", err)
		}
		res.TotalUsers = totalUsers
	}

	return res, nil
}

func loadGroupPolicyObjects(c *gin.Context, dep dependency.Dep, policyIDs []int) ([]*GroupStoragePolicy, error) {
	policies := make([]*GroupStoragePolicy, 0, len(policyIDs))
	for _, policyID := range policyIDs {
		policy, err := dep.StoragePolicyClient().GetPolicyByID(c, policyID)
		if err != nil {
			return nil, serializer.NewError(serializer.CodeDBError, "Failed to load storage policy", err)
		}
		policies = append(policies, &GroupStoragePolicy{
			ID:        policy.ID,
			Name:      policy.Name,
			Type:      policy.Type,
			MaxSize:   policy.MaxSize,
			CreatedAt: policy.CreatedAt,
			UpdatedAt: policy.UpdatedAt,
		})
	}
	return policies, nil
}

type (
	UpsertGroupService struct {
		Group            *groupPayload `json:"group" binding:"required"`
		StoragePolicyIDs []int         `json:"storage_policy_ids"`
	}
	UpsertGroupParamCtx struct{}

	// groupPayload mirrors ent.Group but accepts both shapes of the
	// storage policy edge. Pro's admin UI round-trips edges.storage_policies as
	// an array, while this code base models the legacy column as a single
	// object, so binding directly into ent.Group fails with a JSON type error.
	groupPayload struct {
		ID              int                   `json:"id"`
		Name            *string               `json:"name"`
		MaxStorage      *int64                `json:"max_storage"`
		SpeedLimit      *int                  `json:"speed_limit"`
		Permissions     *boolset.BooleanSet   `json:"permissions"`
		Settings        *types.GroupSetting   `json:"settings"`
		StoragePolicyID *int                  `json:"storage_policy_id"`
		Edges           *groupPayloadEdges    `json:"edges"`
	}
	groupPayloadEdges struct {
		StoragePolicies json.RawMessage `json:"storage_policies"`
	}
)

func (s *UpsertGroupService) Update(c *gin.Context) (*GetGroupResponse, error) {
	dep := dependency.FromContext(c)
	groupClient := dep.GroupClient()

	if s.Group.ID == 0 {
		return nil, serializer.NewError(serializer.CodeParamErr, "ID is required", nil)
	}

	if _, err := groupClient.GetByID(c, s.Group.ID); err != nil {
		return nil, serializer.NewError(serializer.CodeGroupNotFound, "Group not found", err)
	}

	group, err := s.Group.toEnt()
	if err != nil {
		return nil, err
	}
	s.normalizeLegacyPolicy(group)

	// Initial admin group have to be admin
	if group.ID == 1 && (group.Permissions == nil || !group.Permissions.Enabled(int(types.GroupPermissionIsAdmin))) {
		return nil, serializer.NewError(serializer.CodeParamErr, "Initial admin group have to be admin", nil)
	}

	updated, err := groupClient.Upsert(c, group)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to update group", err)
	}
	if err := syncGroupPolicies(c, dep, s.Group.ID, s.StoragePolicyIDs); err != nil {
		return nil, err
	}

	audit.Record(c, audit.EventGroupChanged, &audit.LogEntry{
		GroupID: updated.ID,
		Name:    updated.Name,
	}, audit.WithUser(inventory.UserIDFromContext(c)))
	service := &SingleGroupService{ID: updated.ID}
	return service.Get(c)
}

func (s *UpsertGroupService) Create(c *gin.Context) (*GetGroupResponse, error) {
	dep := dependency.FromContext(c)
	groupClient := dep.GroupClient()

	if s.Group.ID > 0 {
		return nil, serializer.NewError(serializer.CodeParamErr, "ID must be 0", nil)
	}

	group, err := s.Group.toEnt()
	if err != nil {
		return nil, err
	}
	s.normalizeLegacyPolicy(group)

	created, err := groupClient.Upsert(c, group)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to create group", err)
	}
	if err := syncGroupPolicies(c, dep, created.ID, s.StoragePolicyIDs); err != nil {
		return nil, err
	}

	audit.Record(c, audit.EventGroupChanged, &audit.LogEntry{
		GroupID: created.ID,
		Name:    created.Name,
	}, audit.WithUser(inventory.UserIDFromContext(c)))
	service := &SingleGroupService{ID: created.ID}
	return service.Get(c)
}

func syncGroupPolicies(c *gin.Context, dep dependency.Dep, groupID int, policyIDs []int) error {
	if policyIDs == nil {
		return nil
	}

	for _, policyID := range policyIDs {
		if _, err := dep.StoragePolicyClient().GetPolicyByID(c, policyID); err != nil {
			return serializer.NewError(serializer.CodePolicyNotExist, "Storage policy not found", err)
		}
	}
	if err := dep.GroupPolicyClient().ReplaceGroupPolicies(c, groupID, policyIDs); err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to update group storage policies", err)
	}
	return nil
}

// toEnt converts the admin payload into the ent model.
func (p *groupPayload) toEnt() (*ent.Group, error) {
	if p == nil {
		return nil, serializer.NewError(serializer.CodeParamErr, "Group is required", nil)
	}

	group := &ent.Group{ID: p.ID}
	if p.Name != nil {
		group.Name = *p.Name
	}
	if p.MaxStorage != nil {
		group.MaxStorage = *p.MaxStorage
	}
	if p.SpeedLimit != nil {
		group.SpeedLimit = *p.SpeedLimit
	}
	group.Permissions = p.Permissions
	group.Settings = p.Settings
	if p.StoragePolicyID != nil {
		group.StoragePolicyID = *p.StoragePolicyID
	}

	policyID, err := p.storagePolicyID()
	if err != nil {
		return nil, err
	}
	if policyID > 0 {
		group.StoragePolicyID = policyID
		group.Edges.StoragePolicies = &ent.StoragePolicy{ID: policyID}
	}

	return group, nil
}

// storagePolicyID accepts the single relation this code base uses, Pro's array
// form, and a bare numeric id.
func (p *groupPayload) storagePolicyID() (int, error) {
	if p.Edges == nil || len(p.Edges.StoragePolicies) == 0 {
		return 0, nil
	}

	raw := p.Edges.StoragePolicies
	var single struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(raw, &single); err == nil {
		return single.ID, nil
	}

	var list []struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(raw, &list); err == nil {
		if len(list) == 0 {
			return 0, nil
		}
		return list[0].ID, nil
	}

	return 0, serializer.NewError(serializer.CodeParamErr, "Invalid storage policy selection", nil)
}

// normalizeLegacyPolicy keeps the legacy groups.storage_policy_id column in
// sync with the first item in the Pro-compatible multi-policy selection.
func (s *UpsertGroupService) normalizeLegacyPolicy(group *ent.Group) {
	if group == nil {
		return
	}
	if group.Edges.StoragePolicies != nil && group.Edges.StoragePolicies.ID > 0 {
		return
	}
	if len(s.StoragePolicyIDs) > 0 {
		group.Edges.StoragePolicies = &ent.StoragePolicy{ID: s.StoragePolicyIDs[0]}
		group.StoragePolicyID = s.StoragePolicyIDs[0]
	}
}
