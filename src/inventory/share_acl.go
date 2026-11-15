package inventory

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/cloudreve/Cloudreve/v4/ent"
)

// SharePermission is the Pro fine grained permission set for one audience.
type SharePermission struct {
	CanList      bool `json:"can_list"`
	CanRead      bool `json:"can_read"`
	CanUpdate    bool `json:"can_update"`
	CanCreate    bool `json:"can_create"`
	CanDelete    bool `json:"can_delete"`
	CanShare     bool `json:"can_share"`
	Discoverable bool `json:"discoverable"`
}

const (
	// ShareAudienceAnonymous is the built in audience for anonymous visitors.
	ShareAudienceAnonymous = "anonymous"
	// ShareAudienceLoggedIn is the built in audience for any signed-in user.
	ShareAudienceLoggedIn = "logged_in"
	// ShareAudienceSameGroup applies to users sharing the owner's group.
	ShareAudienceSameGroup = "same_group"
	// ShareAudienceOtherGroup applies to users from any other group.
	ShareAudienceOtherGroup = "other_group"
)

type (
	// BuiltinSharePermission binds a builtin audience to a permission set.
	BuiltinSharePermission struct {
		ID         int             `json:"id"`
		ShareID    int             `json:"share_id"`
		Audience   string          `json:"audience"`
		Permission SharePermission `json:"permission"`
	}

	// GroupSharePermission binds a group to a permission set.
	GroupSharePermission struct {
		ID         int             `json:"id"`
		ShareID    int             `json:"share_id"`
		GroupID    int             `json:"group_id"`
		Permission SharePermission `json:"permission"`
	}

	// UserSharePermission binds a user to a permission set.
	UserSharePermission struct {
		ID         int             `json:"id"`
		ShareID    int             `json:"share_id"`
		UserID     int             `json:"user_id"`
		UserHashID string          `json:"user_hash_id,omitempty"`
		Permission SharePermission `json:"permission"`
	}

	// ShareACLClient stores Pro fine grained share permissions.
	ShareACLClient interface {
		TxOperator
		// List returns every ACL rule attached to the share.
		List(ctx context.Context, shareID int) (*ShareACL, error)
		// ListByShareIDs returns ACL rules grouped by share ID.
		ListByShareIDs(ctx context.Context, shareIDs []int) (map[int]*ShareACL, error)
		// Replace overwrites all ACL rules of the share in one transaction.
		Replace(ctx context.Context, shareID int, acl *ShareACL) error
		// DeleteByShare removes every rule of the share.
		DeleteByShare(ctx context.Context, shareID int) error
	}

	// ShareACL is the aggregated permission configuration of one share.
	ShareACL struct {
		Builtin []BuiltinSharePermission
		Group   []GroupSharePermission
		User    []UserSharePermission
	}
)

// NewShareACLClient creates a SQL backed share ACL client.
func NewShareACLClient(client *ent.Client) ShareACLClient {
	return &shareACLClient{client: client}
}

type shareACLClient struct {
	client *ent.Client
}

func (c *shareACLClient) SetClient(newClient *ent.Client) TxOperator {
	return &shareACLClient{client: newClient}
}

func (c *shareACLClient) GetClient() *ent.Client {
	return c.client
}

func (c *shareACLClient) List(ctx context.Context, shareID int) (*ShareACL, error) {
	res := &ShareACL{}

	query, args := c.rebind(
		`SELECT id, share_id, audience, can_list, can_read, can_update, can_create, can_delete, can_share, discoverable
FROM share_builtin_permissions WHERE share_id = ? ORDER BY id ASC`, shareID,
	)
	rows := new(entsql.Rows)
	if err := c.client.Driver().Query(ctx, query, args, rows); err != nil {
		return nil, fmt.Errorf("failed to list builtin share permissions: %w", err)
	}
	for rows.Next() {
		item := BuiltinSharePermission{}
		if err := rows.Scan(&item.ID, &item.ShareID, &item.Audience, &item.Permission.CanList, &item.Permission.CanRead,
			&item.Permission.CanUpdate, &item.Permission.CanCreate, &item.Permission.CanDelete,
			&item.Permission.CanShare, &item.Permission.Discoverable); err != nil {
			rows.Close()
			return nil, fmt.Errorf("failed to scan builtin share permission: %w", err)
		}
		res.Builtin = append(res.Builtin, item)
	}
	rows.Close()

	query, args = c.rebind(
		`SELECT id, share_id, group_id, can_list, can_read, can_update, can_create, can_delete, can_share, discoverable
FROM share_group_permissions WHERE share_id = ? ORDER BY id ASC`, shareID,
	)
	rows = new(entsql.Rows)
	if err := c.client.Driver().Query(ctx, query, args, rows); err != nil {
		return nil, fmt.Errorf("failed to list group share permissions: %w", err)
	}
	for rows.Next() {
		item := GroupSharePermission{}
		if err := rows.Scan(&item.ID, &item.ShareID, &item.GroupID, &item.Permission.CanList, &item.Permission.CanRead,
			&item.Permission.CanUpdate, &item.Permission.CanCreate, &item.Permission.CanDelete,
			&item.Permission.CanShare, &item.Permission.Discoverable); err != nil {
			rows.Close()
			return nil, fmt.Errorf("failed to scan group share permission: %w", err)
		}
		res.Group = append(res.Group, item)
	}
	rows.Close()

	query, args = c.rebind(
		`SELECT id, share_id, user_id, can_list, can_read, can_update, can_create, can_delete, can_share, discoverable
FROM share_user_permissions WHERE share_id = ? ORDER BY id ASC`, shareID,
	)
	rows = new(entsql.Rows)
	if err := c.client.Driver().Query(ctx, query, args, rows); err != nil {
		return nil, fmt.Errorf("failed to list user share permissions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		item := UserSharePermission{}
		if err := rows.Scan(&item.ID, &item.ShareID, &item.UserID, &item.Permission.CanList, &item.Permission.CanRead,
			&item.Permission.CanUpdate, &item.Permission.CanCreate, &item.Permission.CanDelete,
			&item.Permission.CanShare, &item.Permission.Discoverable); err != nil {
			return nil, fmt.Errorf("failed to scan user share permission: %w", err)
		}
		res.User = append(res.User, item)
	}

	return res, nil
}

func (c *shareACLClient) ListByShareIDs(ctx context.Context, shareIDs []int) (map[int]*ShareACL, error) {
	res := make(map[int]*ShareACL, len(shareIDs))
	const chunkSize = 500
	for start := 0; start < len(shareIDs); start += chunkSize {
		end := start + chunkSize
		if end > len(shareIDs) {
			end = len(shareIDs)
		}
		chunk, err := c.listByShareIDsChunk(ctx, shareIDs[start:end])
		if err != nil {
			return nil, err
		}
		for shareID, acl := range chunk {
			res[shareID] = acl
		}
	}
	return res, nil
}

func (c *shareACLClient) listByShareIDsChunk(ctx context.Context, shareIDs []int) (map[int]*ShareACL, error) {
	res := make(map[int]*ShareACL, len(shareIDs))
	if len(shareIDs) == 0 {
		return res, nil
	}

	placeholders, args := c.inPlaceholders(shareIDs)
	type query struct {
		stmt string
		scan func(rows *entsql.Rows) error
	}
	queries := []query{
		{
			stmt: `SELECT id, share_id, audience, can_list, can_read, can_update, can_create, can_delete, can_share, discoverable
FROM share_builtin_permissions WHERE share_id IN (` + placeholders + `) ORDER BY id ASC`,
			scan: func(rows *entsql.Rows) error {
				item := BuiltinSharePermission{}
				if err := rows.Scan(&item.ID, &item.ShareID, &item.Audience, &item.Permission.CanList, &item.Permission.CanRead,
					&item.Permission.CanUpdate, &item.Permission.CanCreate, &item.Permission.CanDelete,
					&item.Permission.CanShare, &item.Permission.Discoverable); err != nil {
					return err
				}
				acl := res[item.ShareID]
				if acl == nil {
					acl = &ShareACL{}
					res[item.ShareID] = acl
				}
				acl.Builtin = append(acl.Builtin, item)
				return nil
			},
		},
		{
			stmt: `SELECT id, share_id, group_id, can_list, can_read, can_update, can_create, can_delete, can_share, discoverable
FROM share_group_permissions WHERE share_id IN (` + placeholders + `) ORDER BY id ASC`,
			scan: func(rows *entsql.Rows) error {
				item := GroupSharePermission{}
				if err := rows.Scan(&item.ID, &item.ShareID, &item.GroupID, &item.Permission.CanList, &item.Permission.CanRead,
					&item.Permission.CanUpdate, &item.Permission.CanCreate, &item.Permission.CanDelete,
					&item.Permission.CanShare, &item.Permission.Discoverable); err != nil {
					return err
				}
				acl := res[item.ShareID]
				if acl == nil {
					acl = &ShareACL{}
					res[item.ShareID] = acl
				}
				acl.Group = append(acl.Group, item)
				return nil
			},
		},
		{
			stmt: `SELECT id, share_id, user_id, can_list, can_read, can_update, can_create, can_delete, can_share, discoverable
FROM share_user_permissions WHERE share_id IN (` + placeholders + `) ORDER BY id ASC`,
			scan: func(rows *entsql.Rows) error {
				item := UserSharePermission{}
				if err := rows.Scan(&item.ID, &item.ShareID, &item.UserID, &item.Permission.CanList, &item.Permission.CanRead,
					&item.Permission.CanUpdate, &item.Permission.CanCreate, &item.Permission.CanDelete,
					&item.Permission.CanShare, &item.Permission.Discoverable); err != nil {
					return err
				}
				acl := res[item.ShareID]
				if acl == nil {
					acl = &ShareACL{}
					res[item.ShareID] = acl
				}
				acl.User = append(acl.User, item)
				return nil
			},
		},
	}

	for _, item := range queries {
		stmt, queryArgs := c.rebind(item.stmt, args...)
		rows := new(entsql.Rows)
		if err := c.client.Driver().Query(ctx, stmt, queryArgs, rows); err != nil {
			return nil, fmt.Errorf("failed to list share permissions: %w", err)
		}
		for rows.Next() {
			if err := item.scan(rows); err != nil {
				rows.Close()
				return nil, fmt.Errorf("failed to scan share permission: %w", err)
			}
		}
		rows.Close()
	}

	return res, nil
}

func (c *shareACLClient) Replace(ctx context.Context, shareID int, acl *ShareACL) error {
	for _, stmt := range []string{
		`DELETE FROM share_builtin_permissions WHERE share_id = ?`,
		`DELETE FROM share_group_permissions WHERE share_id = ?`,
		`DELETE FROM share_user_permissions WHERE share_id = ?`,
	} {
		query, args := c.rebind(stmt, shareID)
		var res sql.Result
		if err := c.client.Driver().Exec(ctx, query, args, &res); err != nil {
			return fmt.Errorf("failed to clear share permissions: %w", err)
		}
	}

	if acl != nil {
		for _, item := range acl.Builtin {
			query, args := c.rebind(
				`INSERT INTO share_builtin_permissions (share_id, audience, can_list, can_read, can_update, can_create, can_delete, can_share, discoverable)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				shareID, item.Audience, item.Permission.CanList, item.Permission.CanRead, item.Permission.CanUpdate,
				item.Permission.CanCreate, item.Permission.CanDelete, item.Permission.CanShare, item.Permission.Discoverable,
			)
			var res sql.Result
			if err := c.client.Driver().Exec(ctx, query, args, &res); err != nil {
				return fmt.Errorf("failed to insert builtin share permission: %w", err)
			}
		}

		for _, item := range acl.Group {
			query, args := c.rebind(
				`INSERT INTO share_group_permissions (share_id, group_id, can_list, can_read, can_update, can_create, can_delete, can_share, discoverable)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				shareID, item.GroupID, item.Permission.CanList, item.Permission.CanRead, item.Permission.CanUpdate,
				item.Permission.CanCreate, item.Permission.CanDelete, item.Permission.CanShare, item.Permission.Discoverable,
			)
			var res sql.Result
			if err := c.client.Driver().Exec(ctx, query, args, &res); err != nil {
				return fmt.Errorf("failed to insert group share permission: %w", err)
			}
		}

		for _, item := range acl.User {
			query, args := c.rebind(
				`INSERT INTO share_user_permissions (share_id, user_id, can_list, can_read, can_update, can_create, can_delete, can_share, discoverable)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				shareID, item.UserID, item.Permission.CanList, item.Permission.CanRead, item.Permission.CanUpdate,
				item.Permission.CanCreate, item.Permission.CanDelete, item.Permission.CanShare, item.Permission.Discoverable,
			)
			var res sql.Result
			if err := c.client.Driver().Exec(ctx, query, args, &res); err != nil {
				return fmt.Errorf("failed to insert user share permission: %w", err)
			}
		}
	}

	return nil
}

func (c *shareACLClient) DeleteByShare(ctx context.Context, shareID int) error {
	for _, stmt := range []string{
		`DELETE FROM share_builtin_permissions WHERE share_id = ?`,
		`DELETE FROM share_group_permissions WHERE share_id = ?`,
		`DELETE FROM share_user_permissions WHERE share_id = ?`,
	} {
		query, args := c.rebind(stmt, shareID)
		var res sql.Result
		if err := c.client.Driver().Exec(ctx, query, args, &res); err != nil {
			return fmt.Errorf("failed to delete share permissions: %w", err)
		}
	}

	return nil
}

// EffectivePermission resolves the permission set that applies to a viewer.
//
// Explicit user rules win over everything. An explicit group rule wins over
// builtin audiences. Without one, the logged-in / anonymous audience is
// combined with the owner-relative same/other-group audience. Combining those
// two builtin dimensions preserves Pro's ability to grant "all signed-in
// users" read access while still narrowing it for same/other groups.
func (acl *ShareACL) EffectivePermission(userID, userGroupID, ownerGroupID int, anonymous bool) (SharePermission, bool) {
	if acl == nil {
		return SharePermission{}, false
	}

	if userID > 0 {
		for _, item := range acl.User {
			if item.UserID == userID {
				return item.Permission, true
			}
		}
	}

	if !anonymous && userGroupID > 0 {
		for _, item := range acl.Group {
			if item.GroupID == userGroupID {
				return item.Permission, true
			}
		}
	}

	var (
		base    SharePermission
		matched bool
	)
	for _, item := range acl.Builtin {
		if anonymous && item.Audience == ShareAudienceAnonymous {
			base = item.Permission
			matched = true
		}
		if !anonymous && item.Audience == ShareAudienceLoggedIn {
			base = item.Permission
			matched = true
		}
	}

	if !anonymous && userGroupID > 0 {
		for _, item := range acl.Builtin {
			if item.Audience == ShareAudienceSameGroup && userGroupID == ownerGroupID {
				return mergeSharePermissions(base, item.Permission), true
			}
			if item.Audience == ShareAudienceOtherGroup && userGroupID != ownerGroupID {
				return mergeSharePermissions(base, item.Permission), true
			}
		}
	}

	return base, matched
}

func mergeSharePermissions(base, overlay SharePermission) SharePermission {
	return SharePermission{
		CanList:      base.CanList || overlay.CanList,
		CanRead:      base.CanRead || overlay.CanRead,
		CanUpdate:    base.CanUpdate || overlay.CanUpdate,
		CanCreate:    base.CanCreate || overlay.CanCreate,
		CanDelete:    base.CanDelete || overlay.CanDelete,
		CanShare:     base.CanShare || overlay.CanShare,
		Discoverable: base.Discoverable || overlay.Discoverable,
	}
}

func (c *shareACLClient) rebind(query string, args ...any) (string, []any) {
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

func (c *shareACLClient) inPlaceholders(ids []int) (string, []any) {
	args := make([]any, len(ids))
	placeholders := make([]string, len(ids))
	for i, id := range ids {
		args[i] = id
		placeholders[i] = "?"
	}
	return strings.Join(placeholders, ","), args
}
