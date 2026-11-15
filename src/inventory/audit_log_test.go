package inventory

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/enttest"
	"github.com/cloudreve/Cloudreve/v4/pkg/conf"
	"github.com/cloudreve/Cloudreve/v4/pkg/hashid"
	"github.com/stretchr/testify/require"
)

func TestAuditLogTimeBoundsUseLocalZone(t *testing.T) {
	originalLocal := time.Local
	time.Local = time.FixedZone("CST", 8*60*60)
	t.Cleanup(func() { time.Local = originalLocal })

	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", filepath.Join(t.TempDir(), "audit.db"))
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	hasher, err := hashid.New("audit-test")
	require.NoError(t, err)
	auditClient := NewAuditLogClient(client, conf.SQLiteDB, hasher)

	createdAt := time.Date(2026, 11, 13, 17, 53, 14, 0, time.Local)
	logEntry, err := client.AuditLog.Create().
		SetType(7).
		SetCreatedAt(createdAt).
		Save(ctx)
	require.NoError(t, err)

	since := createdAt.In(time.UTC).Add(-time.Minute)
	until := createdAt.In(time.UTC).Add(time.Minute)

	list, err := auditClient.List(ctx, &ListAuditLogArgs{
		PaginationArgs: &PaginationArgs{Page: 0, PageSize: 10},
		Since:          &since,
		Until:          &until,
	})
	require.NoError(t, err)
	require.Len(t, list.Logs, 1)
	require.Equal(t, logEntry.ID, list.Logs[0].ID)

	archived, err := auditClient.Archive(ctx, &since, &until, nil)
	require.NoError(t, err)
	require.Len(t, archived, 1)
	require.Equal(t, logEntry.ID, archived[0].ID)

	deleted, err := auditClient.DeleteByRange(ctx, &since, &until, nil)
	require.NoError(t, err)
	require.Equal(t, 1, deleted)

	_, err = auditClient.GetByID(ctx, logEntry.ID)
	require.True(t, ent.IsNotFound(err), "expected audit log to be deleted, got %v", err)
}
