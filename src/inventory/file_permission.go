package inventory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/pkg/boolset"
)

// FilePermissionSetting is the Pro fine grained permission setting stored on a
// file. Values are base64 encoded boolsets so the JSON is interchangeable with
// Cloudreve Pro.
type FilePermissionSetting struct {
	Anonymous     string            `json:"anonymous,omitempty"`
	Everyone      string            `json:"everyone,omitempty"`
	SameGroup     string            `json:"same_group,omitempty"`
	Other         string            `json:"other,omitempty"`
	UserExplicit  map[string]string `json:"user_explicit,omitempty"`
	GroupExplicit map[string]string `json:"group_explicit,omitempty"`
}

// FilePermissionClient reads and writes Pro's per-file permissions.
type FilePermissionClient interface {
	Get(ctx context.Context, fileID int) (*FilePermissionSetting, bool, error)
	Set(ctx context.Context, fileIDs []int, setting *FilePermissionSetting) error
	Delete(ctx context.Context, fileIDs []int) error
	// Effective resolves the nearest inherited permission setting for a file.
	Effective(ctx context.Context, fileID, userID, userGroupID, ownerGroupID int, anonymous bool) (SharePermission, bool, error)
}

// NewFilePermissionClient creates a SQL backed file permission client.
func NewFilePermissionClient(client *ent.Client) FilePermissionClient {
	return &filePermissionClient{client: client}
}

type filePermissionClient struct {
	client *ent.Client
}

func (c *filePermissionClient) Get(ctx context.Context, fileID int) (*FilePermissionSetting, bool, error) {
	query, args := c.rebind(`SELECT permissions FROM files WHERE id = ?`, fileID)
	rows := new(entsql.Rows)
	if err := c.client.Driver().Query(ctx, query, args, rows); err != nil {
		return nil, false, fmt.Errorf("failed to query file permissions: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, false, rows.Err()
	}

	var raw []byte
	if err := rows.Scan(&raw); err != nil {
		return nil, false, fmt.Errorf("failed to scan file permissions: %w", err)
	}
	if len(raw) == 0 {
		return nil, false, nil
	}

	setting := &FilePermissionSetting{}
	if err := json.Unmarshal(raw, setting); err != nil {
		return nil, false, fmt.Errorf("failed to decode file permissions: %w", err)
	}
	return setting, true, rows.Err()
}

func (c *filePermissionClient) Set(ctx context.Context, fileIDs []int, setting *FilePermissionSetting) error {
	if len(fileIDs) == 0 {
		return nil
	}
	if setting == nil {
		setting = &FilePermissionSetting{}
	}

	raw, err := json.Marshal(setting)
	if err != nil {
		return fmt.Errorf("failed to encode file permissions: %w", err)
	}

	for _, fileID := range fileIDs {
		query, args := c.rebind(`UPDATE files SET permissions = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, string(raw), fileID)
		var result sql.Result
		if err := c.client.Driver().Exec(ctx, query, args, &result); err != nil {
			return fmt.Errorf("failed to update file permissions: %w", err)
		}
		if err := c.replaceAncestorLinks(ctx, fileID); err != nil {
			return err
		}
	}
	return nil
}

func (c *filePermissionClient) Delete(ctx context.Context, fileIDs []int) error {
	for _, fileID := range fileIDs {
		query, args := c.rebind(`UPDATE files SET permissions = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, fileID)
		var result sql.Result
		if err := c.client.Driver().Exec(ctx, query, args, &result); err != nil {
			return fmt.Errorf("failed to clear file permissions: %w", err)
		}
		if err := c.replaceAncestorLinks(ctx, fileID); err != nil {
			return err
		}
	}
	return nil
}

func (c *filePermissionClient) Effective(ctx context.Context, fileID, userID, userGroupID, ownerGroupID int, anonymous bool) (SharePermission, bool, error) {
	seen := make(map[int]bool)
	current := fileID
	for current > 0 && len(seen) < 128 {
		if seen[current] {
			break
		}
		seen[current] = true

		setting, found, err := c.Get(ctx, current)
		if err != nil {
			return SharePermission{}, false, err
		}
		if found {
			permission, matched := setting.Effective(userID, userGroupID, ownerGroupID, anonymous)
			if matched {
				return permission, true, nil
			}
		}

		parent, err := c.parentID(ctx, current)
		if err != nil {
			return SharePermission{}, false, err
		}
		current = parent
	}
	return SharePermission{}, false, nil
}

func (c *filePermissionClient) parentID(ctx context.Context, fileID int) (int, error) {
	query, args := c.rebind(`SELECT file_children FROM files WHERE id = ?`, fileID)
	rows := new(entsql.Rows)
	if err := c.client.Driver().Query(ctx, query, args, rows); err != nil {
		return 0, fmt.Errorf("failed to query file parent: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return 0, rows.Err()
	}
	var parent sql.NullInt64
	if err := rows.Scan(&parent); err != nil {
		return 0, err
	}
	if !parent.Valid {
		return 0, nil
	}
	return int(parent.Int64), nil
}

func (c *filePermissionClient) replaceAncestorLinks(ctx context.Context, fileID int) error {
	query, args := c.rebind(`DELETE FROM file_ancestor_permissions WHERE descendant_permission_id = ?`, fileID)
	var result sql.Result
	if err := c.client.Driver().Exec(ctx, query, args, &result); err != nil {
		return fmt.Errorf("failed to clear file ancestor permissions: %w", err)
	}

	seen := make(map[int]bool)
	current, err := c.parentID(ctx, fileID)
	if err != nil {
		return err
	}
	for current > 0 && len(seen) < 128 {
		if seen[current] {
			break
		}
		seen[current] = true

		query, args = c.rebind(`INSERT INTO file_ancestor_permissions (file_id, descendant_permission_id) VALUES (?, ?)`, current, fileID)
		if err := c.client.Driver().Exec(ctx, query, args, &result); err != nil {
			return fmt.Errorf("failed to insert file ancestor permissions: %w", err)
		}

		current, err = c.parentID(ctx, current)
		if err != nil {
			return err
		}
	}
	return nil
}

// Effective resolves the permission set for a viewer. Explicit user rules win
// over explicit group rules, then group relationships, everyone, and finally
// the anonymous audience.
func (s *FilePermissionSetting) Effective(userID, userGroupID, ownerGroupID int, anonymous bool) (SharePermission, bool) {
	if s == nil {
		return SharePermission{}, false
	}

	if userID > 0 && s.UserExplicit != nil {
		if raw, ok := s.UserExplicit[strconv.Itoa(userID)]; ok {
			if permission, ok := parsePermissionBits(raw); ok {
				return permission, true
			}
		}
	}
	if !anonymous && userGroupID > 0 && s.GroupExplicit != nil {
		if raw, ok := s.GroupExplicit[strconv.Itoa(userGroupID)]; ok {
			if permission, ok := parsePermissionBits(raw); ok {
				return permission, true
			}
		}
	}
	if !anonymous && userGroupID > 0 && ownerGroupID > 0 {
		if userGroupID == ownerGroupID && s.SameGroup != "" {
			return mustPermissionBits(s.SameGroup)
		}
		if userGroupID != ownerGroupID && s.Other != "" {
			return mustPermissionBits(s.Other)
		}
	}
	if anonymous && s.Anonymous != "" {
		return mustPermissionBits(s.Anonymous)
	}
	if s.Everyone != "" {
		return mustPermissionBits(s.Everyone)
	}
	return SharePermission{}, false
}

func parsePermissionBits(raw string) (SharePermission, bool) {
	bs, err := boolset.FromString(raw)
	if err != nil {
		return SharePermission{}, false
	}
	return SharePermission{
		CanList:      bs.Enabled(0),
		CanRead:      bs.Enabled(1),
		CanUpdate:    bs.Enabled(2),
		CanCreate:    bs.Enabled(3),
		CanDelete:    bs.Enabled(4),
		CanShare:     bs.Enabled(5),
		Discoverable: bs.Enabled(6),
	}, true
}

func mustPermissionBits(raw string) (SharePermission, bool) {
	permission, _ := parsePermissionBits(raw)
	return permission, true
}

// ValidatePermissionSetting rejects unknown bits and ensures write permissions
// are only granted when the owner's group explicitly allows it.
func ValidatePermissionSetting(setting *FilePermissionSetting, groupAllowedToSetAnonymous, groupAllowedToSetExplicit bool) error {
	if setting == nil {
		return nil
	}

	check := func(name, raw string) error {
		if raw == "" {
			return nil
		}
		permission, ok := parsePermissionBits(raw)
		if !ok {
			return fmt.Errorf("invalid permission set for %s", name)
		}
		if !groupAllowedToSetAnonymous && (permission.CanUpdate || permission.CanCreate || permission.CanDelete || permission.CanShare) {
			return fmt.Errorf("your group cannot grant write permissions to %s", name)
		}
		return nil
	}

	for name, raw := range map[string]string{
		"anonymous":  setting.Anonymous,
		"everyone":   setting.Everyone,
		"same_group": setting.SameGroup,
		"other":      setting.Other,
	} {
		if err := check(name, raw); err != nil {
			return err
		}
	}

	if !groupAllowedToSetExplicit && (len(setting.UserExplicit) > 0 || len(setting.GroupExplicit) > 0) {
		return fmt.Errorf("your group cannot set explicit user or group permissions")
	}
	for key, raw := range setting.UserExplicit {
		if _, err := strconv.Atoi(key); err != nil || strings.TrimSpace(key) == "" {
			return fmt.Errorf("invalid explicit user id %q", key)
		}
		if err := check("user "+key, raw); err != nil {
			return err
		}
	}
	for key, raw := range setting.GroupExplicit {
		if _, err := strconv.Atoi(key); err != nil || strings.TrimSpace(key) == "" {
			return fmt.Errorf("invalid explicit group id %q", key)
		}
		if err := check("group "+key, raw); err != nil {
			return err
		}
	}
	return nil
}

func (c *filePermissionClient) rebind(query string, args ...any) (string, []any) {
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
