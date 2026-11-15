package vas

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/pkg/crontab"
	"github.com/cloudreve/Cloudreve/v4/pkg/setting"
	"github.com/cloudreve/Cloudreve/v4/service/audit"
)

func init() {
	crontab.Register(setting.CronTypeBanUser, ExpireMemberships)
}

// ExpireMemberships reverts users whose paid group membership has expired.
// Pro stores the previous group and expiry in additive columns that are not
// part of the community ent schema, so this task uses SQL just like the group
// product fulfillment path.
func ExpireMemberships(ctx context.Context) {
	dep := dependency.FromContext(ctx)
	client := dep.DBClient()
	if client == nil {
		return
	}
	drv := client.Driver()
	if drv == nil {
		return
	}

	query, args := rebindQuery(drv.Dialect(),
		`SELECT id, group_users, previous_group, group_expires
		 FROM users
		 WHERE deleted_at IS NULL AND group_expires IS NOT NULL
		 ORDER BY id ASC`)
	rows := new(entsql.Rows)
	if err := drv.Query(ctx, query, args, rows); err != nil {
		dep.Logger().Warning("Failed to scan expired memberships: %s", err)
		return
	}

	now := time.Now()
	type expiredMembership struct {
		userID        int
		currentGroup  int
		previousGroup sql.NullInt64
		expires       time.Time
	}
	expired := make([]expiredMembership, 0)
	for rows.Next() {
		var (
			userID        int
			currentGroup  int
			previousGroup sql.NullInt64
			expiresRaw    any
		)
		if err := rows.Scan(&userID, &currentGroup, &previousGroup, &expiresRaw); err != nil {
			dep.Logger().Warning("Failed to scan expired membership: %s", err)
			continue
		}

		expires, err := parseMembershipExpiry(expiresRaw)
		if err != nil {
			dep.Logger().Warning("Failed to parse membership expiry for user %d: %s", userID, err)
			continue
		}
		if expires.After(now) {
			continue
		}
		expired = append(expired, expiredMembership{
			userID:        userID,
			currentGroup:  currentGroup,
			previousGroup: previousGroup,
			expires:       expires,
		})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		dep.Logger().Warning("Failed to iterate expired memberships: %s", err)
		return
	}
	rows.Close()

	// Resolve settings only after the result set is closed: SQLite runs the
	// community server with a single connection, so holding rows open while
	// reading settings would deadlock until the busy timeout expires.
	defaultGroup := dep.SettingProvider().DefaultGroup(ctx)
	for _, item := range expired {
		targetGroup := defaultGroup
		if item.previousGroup.Valid && item.previousGroup.Int64 > 0 {
			targetGroup = int(item.previousGroup.Int64)
		}
		if targetGroup <= 0 {
			targetGroup = defaultGroup
		}
		if _, err := dep.GroupClient().GetByID(ctx, targetGroup); err != nil {
			targetGroup = defaultGroup
		}
		if targetGroup <= 0 {
			dep.Logger().Warning("Cannot expire membership for user %d: no fallback group", item.userID)
			continue
		}

		query, args = rebindQuery(drv.Dialect(),
			`UPDATE users
			 SET group_users = ?, previous_group = NULL, group_expires = NULL
			 WHERE id = ? AND deleted_at IS NULL AND group_expires IS NOT NULL`,
			targetGroup, item.userID)
		var result sql.Result
		if err := drv.Exec(ctx, query, args, &result); err != nil {
			dep.Logger().Warning("Failed to expire membership for user %d: %s", item.userID, err)
			continue
		}
		affected, err := result.RowsAffected()
		if err != nil {
			dep.Logger().Warning("Failed to confirm membership expiry for user %d: %s", item.userID, err)
			continue
		}
		if affected == 0 {
			continue
		}

		audit.Record(ctx, audit.EventMembershipUnsub, &audit.LogEntry{
			IsSystem:    true,
			Reason:      "membership expired",
			GroupID:     targetGroup,
			GroupIDFrom: item.currentGroup,
		}, audit.WithUser(item.userID))
	}
}

func parseMembershipExpiry(value any) (time.Time, error) {
	switch v := value.(type) {
	case time.Time:
		return v, nil
	case string:
		return parseMembershipExpiryString(v)
	case []byte:
		return parseMembershipExpiryString(string(v))
	case fmt.Stringer:
		return parseMembershipExpiryString(v.String())
	default:
		return time.Time{}, fmt.Errorf("unsupported expiry value %T", value)
	}
}

func parseMembershipExpiryString(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("empty expiry")
	}

	layouts := []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05-07",
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999-07",
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported expiry format %q", value)
}
