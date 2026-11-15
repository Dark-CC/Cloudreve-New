package inventory

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
)

type (
	// GroupPolicyBinding is one storage policy bound to a group.
	GroupPolicyBinding struct {
		GroupID         int
		StoragePolicyID int
	}

	// LoadBalanceTarget is one weighted target of a load balance policy.
	LoadBalanceTarget struct {
		ID             int `json:"id,omitempty"`
		PolicyID       int `json:"policy_id,omitempty"`
		TargetPolicyID int `json:"target_policy_id"`
		Weight         int `json:"weight"`
	}

	// GroupPolicyClient stores Pro group storage policy bindings and load
	// balance weights.
	GroupPolicyClient interface {
		TxOperator
		// ListGroupPolicies returns the policy IDs bound to the group.
		ListGroupPolicies(ctx context.Context, groupID int) ([]int, error)
		// ReplaceGroupPolicies overwrites the group bindings.
		ReplaceGroupPolicies(ctx context.Context, groupID int, policyIDs []int) error
		// AddGroupPolicy binds one policy to the group.
		AddGroupPolicy(ctx context.Context, groupID, policyID int) error
		// RemoveGroupPolicy unbinds one policy from the group.
		RemoveGroupPolicy(ctx context.Context, groupID, policyID int) error
		// ListLoadBalanceTargets returns configured weighted targets.
		ListLoadBalanceTargets(ctx context.Context, policyID int) ([]*LoadBalanceTarget, error)
		// ReplaceLoadBalanceTargets overwrites the targets of a load balance policy.
		ReplaceLoadBalanceTargets(ctx context.Context, policyID int, targets []*LoadBalanceTarget) error
	}
)

// NewGroupPolicyClient creates a SQL backed GroupPolicyClient.
func NewGroupPolicyClient(client *ent.Client) GroupPolicyClient {
	return &groupPolicyClient{client: client}
}

type groupPolicyClient struct {
	client *ent.Client
}

func (c *groupPolicyClient) SetClient(newClient *ent.Client) TxOperator {
	return &groupPolicyClient{client: newClient}
}

func (c *groupPolicyClient) GetClient() *ent.Client {
	return c.client
}

func (c *groupPolicyClient) ListGroupPolicies(ctx context.Context, groupID int) ([]int, error) {
	query, args := c.rebind(`SELECT storage_policy_id FROM group_storage_policies WHERE group_id = ? ORDER BY storage_policy_id ASC`, groupID)
	rows := new(entsql.Rows)
	if err := c.client.Driver().Query(ctx, query, args, rows); err != nil {
		return nil, fmt.Errorf("failed to list group storage policies: %w", err)
	}
	defer rows.Close()

	var res []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan group storage policy: %w", err)
		}
		res = append(res, id)
	}

	return res, nil
}

func (c *groupPolicyClient) ReplaceGroupPolicies(ctx context.Context, groupID int, policyIDs []int) error {
	query, args := c.rebind(`DELETE FROM group_storage_policies WHERE group_id = ?`, groupID)
	var res sql.Result
	if err := c.client.Driver().Exec(ctx, query, args, &res); err != nil {
		return fmt.Errorf("failed to clear group storage policies: %w", err)
	}

	seen := make(map[int]struct{}, len(policyIDs))
	for _, policyID := range policyIDs {
		if _, ok := seen[policyID]; ok {
			continue
		}
		seen[policyID] = struct{}{}

		query, args := c.rebind(`INSERT INTO group_storage_policies (group_id, storage_policy_id) VALUES (?, ?)`, groupID, policyID)
		if err := c.client.Driver().Exec(ctx, query, args, &res); err != nil {
			return fmt.Errorf("failed to bind group storage policy: %w", err)
		}
	}

	return nil
}

func (c *groupPolicyClient) AddGroupPolicy(ctx context.Context, groupID, policyID int) error {
	query, args := c.rebind(`INSERT INTO group_storage_policies (group_id, storage_policy_id) VALUES (?, ?)`, groupID, policyID)
	var res sql.Result
	if err := c.client.Driver().Exec(ctx, query, args, &res); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil
		}
		return fmt.Errorf("failed to bind group storage policy: %w", err)
	}
	return nil
}

func (c *groupPolicyClient) RemoveGroupPolicy(ctx context.Context, groupID, policyID int) error {
	query, args := c.rebind(`DELETE FROM group_storage_policies WHERE group_id = ? AND storage_policy_id = ?`, groupID, policyID)
	var res sql.Result
	if err := c.client.Driver().Exec(ctx, query, args, &res); err != nil {
		return fmt.Errorf("failed to unbind group storage policy: %w", err)
	}
	return nil
}

func (c *groupPolicyClient) ListLoadBalanceTargets(ctx context.Context, policyID int) ([]*LoadBalanceTarget, error) {
	query, args := c.rebind(
		`SELECT id, weight, storage_policy_id, load_balance_id FROM policy_load_balances WHERE load_balance_id = ? ORDER BY id ASC`,
		policyID,
	)
	rows := new(entsql.Rows)
	if err := c.client.Driver().Query(ctx, query, args, rows); err != nil {
		return nil, fmt.Errorf("failed to list load balance targets: %w", err)
	}
	defer rows.Close()

	var res []*LoadBalanceTarget
	for rows.Next() {
		target := &LoadBalanceTarget{}
		if err := rows.Scan(&target.ID, &target.Weight, &target.TargetPolicyID, &target.PolicyID); err != nil {
			return nil, fmt.Errorf("failed to scan load balance target: %w", err)
		}
		res = append(res, target)
	}

	return res, nil
}

func (c *groupPolicyClient) ReplaceLoadBalanceTargets(ctx context.Context, policyID int, targets []*LoadBalanceTarget) error {
	query, args := c.rebind(`DELETE FROM policy_load_balances WHERE load_balance_id = ?`, policyID)
	var res sql.Result
	if err := c.client.Driver().Exec(ctx, query, args, &res); err != nil {
		return fmt.Errorf("failed to clear load balance targets: %w", err)
	}

	for _, target := range targets {
		if target.TargetPolicyID == policyID {
			return serializer.NewError(serializer.CodeParamErr, "Load balance policy cannot reference itself", nil)
		}

		weight := target.Weight
		if weight <= 0 {
			weight = 1
		}

		query, args := c.rebind(
			`INSERT INTO policy_load_balances (weight, storage_policy_id, load_balance_id) VALUES (?, ?, ?)`,
			weight, target.TargetPolicyID, policyID,
		)
		if err := c.client.Driver().Exec(ctx, query, args, &res); err != nil {
			return fmt.Errorf("failed to insert load balance target: %w", err)
		}
	}

	return nil
}

func (c *groupPolicyClient) rebind(query string, args ...any) (string, []any) {
	if c.client.Driver().Dialect() != dialect.Postgres {
		return query, args
	}

	var (
		builder strings.Builder
		argIdx  int
	)
	for _, r := range query {
		if r == '?' {
			argIdx++
			builder.WriteString(fmt.Sprintf("$%d", argIdx))
			continue
		}
		builder.WriteRune(r)
	}

	return builder.String(), args
}
