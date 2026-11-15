package admin

import (
	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/gin-gonic/gin"
)

type (
	// GroupPolicyService reads and writes the storage policies of a group.
	GroupPolicyService struct {
		ID        int   `uri:"id" binding:"required"`
		PolicyIDs []int `json:"policy_ids"`
	}
	GroupPolicyParamCtx struct{}

	// GroupPolicyResponse lists the bound policy IDs.
	GroupPolicyResponse struct {
		GroupID   int   `json:"group_id"`
		PolicyIDs []int `json:"policy_ids"`
	}

	// LoadBalanceService manages weighted targets of a load balance policy.
	LoadBalanceService struct {
		ID      int                           `uri:"id" binding:"required"`
		Targets []inventory.LoadBalanceTarget `json:"targets"`
	}
	LoadBalanceParamCtx struct{}

	// LoadBalanceResponse lists the configured targets.
	LoadBalanceResponse struct {
		PolicyID int                           `json:"policy_id"`
		Targets  []inventory.LoadBalanceTarget `json:"targets"`
	}
)

// Get returns the storage policy bindings of a group.
func (s *GroupPolicyService) Get(c *gin.Context) (*GroupPolicyResponse, error) {
	dep := dependency.FromContext(c)
	if _, err := dep.GroupClient().GetByID(c, s.ID); err != nil {
		return nil, serializer.NewError(serializer.CodeGroupNotFound, "Group not found", err)
	}

	policies, err := dep.GroupPolicyClient().ListGroupPolicies(c, s.ID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to list group policies", err)
	}

	return &GroupPolicyResponse{GroupID: s.ID, PolicyIDs: policies}, nil
}

// Update replaces the storage policy bindings of a group.
func (s *GroupPolicyService) Update(c *gin.Context) (*GroupPolicyResponse, error) {
	dep := dependency.FromContext(c)
	if _, err := dep.GroupClient().GetByID(c, s.ID); err != nil {
		return nil, serializer.NewError(serializer.CodeGroupNotFound, "Group not found", err)
	}

	for _, policyID := range s.PolicyIDs {
		if _, err := dep.StoragePolicyClient().GetPolicyByID(c, policyID); err != nil {
			return nil, serializer.NewError(serializer.CodePolicyNotExist, "Storage policy not found", err)
		}
	}

	if err := dep.GroupPolicyClient().ReplaceGroupPolicies(c, s.ID, s.PolicyIDs); err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to update group policies", err)
	}

	policies, err := dep.GroupPolicyClient().ListGroupPolicies(c, s.ID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to read group policies", err)
	}

	return &GroupPolicyResponse{GroupID: s.ID, PolicyIDs: policies}, nil
}

// Get returns the weighted targets of a load balance policy.
func (s *LoadBalanceService) Get(c *gin.Context) (*LoadBalanceResponse, error) {
	dep := dependency.FromContext(c)
	targets, err := dep.GroupPolicyClient().ListLoadBalanceTargets(c, s.ID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to list load balance targets", err)
	}

	res := &LoadBalanceResponse{PolicyID: s.ID, Targets: make([]inventory.LoadBalanceTarget, 0, len(targets))}
	for _, target := range targets {
		res.Targets = append(res.Targets, *target)
	}
	return res, nil
}

// Update replaces the weighted targets of a load balance policy.
func (s *LoadBalanceService) Update(c *gin.Context) (*LoadBalanceResponse, error) {
	dep := dependency.FromContext(c)
	policy, err := dep.StoragePolicyClient().GetPolicyByID(c, s.ID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodePolicyNotExist, "Storage policy not found", err)
	}

	if policy.Type != "load_balance" {
		return nil, serializer.NewError(serializer.CodeParamErr, "Storage policy is not a load balance policy", nil)
	}

	for i := range s.Targets {
		target := &s.Targets[i]
		if _, err := dep.StoragePolicyClient().GetPolicyByID(c, target.TargetPolicyID); err != nil {
			return nil, serializer.NewError(serializer.CodePolicyNotExist, "Target storage policy not found", err)
		}
		if target.Weight <= 0 {
			target.Weight = 1
		}
	}

	targets := make([]*inventory.LoadBalanceTarget, 0, len(s.Targets))
	for i := range s.Targets {
		targets = append(targets, &s.Targets[i])
	}

	if err := dep.GroupPolicyClient().ReplaceLoadBalanceTargets(c, s.ID, targets); err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to update load balance targets", err)
	}

	stored, err := dep.GroupPolicyClient().ListLoadBalanceTargets(c, s.ID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to read load balance targets", err)
	}

	res := &LoadBalanceResponse{PolicyID: s.ID, Targets: make([]inventory.LoadBalanceTarget, 0, len(stored))}
	for _, target := range stored {
		res.Targets = append(res.Targets, *target)
	}
	return res, nil
}
