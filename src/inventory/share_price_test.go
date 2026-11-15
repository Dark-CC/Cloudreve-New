package inventory

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/enttest"
	"github.com/cloudreve/Cloudreve/v4/ent/user"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/boolset"
	"github.com/cloudreve/Cloudreve/v4/pkg/conf"
	"github.com/cloudreve/Cloudreve/v4/pkg/hashid"
	"github.com/stretchr/testify/require"
)

// newPriceTestShare creates a share row plus the additive price columns that a
// Pro-migrated database would already have.
func newPriceTestShare(t *testing.T, extraPointsColumn bool) (*ent.Client, ShareClient, int) {
	t.Helper()

	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", filepath.Join(t.TempDir(), "shares.db"))
	t.Cleanup(func() { _ = client.Close() })

	drv := client.Driver()
	exec := func(query string) {
		require.NoError(t, drv.Exec(ctx, query, []any{}, new(sql.Result)))
	}
	exec(`ALTER TABLE shares ADD COLUMN price integer NOT NULL DEFAULT 0`)
	if extraPointsColumn {
		exec(`ALTER TABLE shares ADD COLUMN price_points integer NOT NULL DEFAULT 0`)
	}

	groupModel := client.Group.Create().
		SetName("price-test").
		SetMaxStorage(0).
		SetSpeedLimit(0).
		SetPermissions(&boolset.BooleanSet{}).
		SetSettings(&types.GroupSetting{}).
		SaveX(ctx)
	userModel := client.User.Create().
		SetEmail("price@example.com").
		SetNick("price").
		SetPassword("x").
		SetStatus(user.StatusActive).
		SetGroupUsers(groupModel.ID).
		SaveX(ctx)
	file := client.File.Create().
		SetName("a.bin").
		SetType(0).
		SetOwnerID(userModel.ID).
		SaveX(ctx)
	share := client.Share.Create().
		SetUserID(userModel.ID).
		SetFileID(file.ID).
		SaveX(ctx)

	hasher, err := hashid.New("share-price-test")
	require.NoError(t, err)
	return client, NewShareClient(client, conf.SQLiteDB, hasher), share.ID
}

func TestSharePriceFallsBackToProColumn(t *testing.T) {
	client, shares, shareID := newPriceTestShare(t, true)
	ctx := context.Background()

	require.NoError(t, client.Driver().Exec(ctx,
		`UPDATE shares SET price = 0, price_points = 25 WHERE id = ?`,
		[]any{shareID}, new(sql.Result)))

	price, err := shares.GetPrice(ctx, shareID)
	require.NoError(t, err)
	require.Equal(t, int64(25), price, "Pro-era price_points must not be read as free")
}

func TestSharePricePrefersCeColumnWhenSet(t *testing.T) {
	client, shares, shareID := newPriceTestShare(t, true)
	ctx := context.Background()

	require.NoError(t, client.Driver().Exec(ctx,
		`UPDATE shares SET price = 40, price_points = 25 WHERE id = ?`,
		[]any{shareID}, new(sql.Result)))

	price, err := shares.GetPrice(ctx, shareID)
	require.NoError(t, err)
	require.Equal(t, int64(40), price)
}

func TestSetPriceMirrorsIntoProColumn(t *testing.T) {
	client, shares, shareID := newPriceTestShare(t, true)
	ctx := context.Background()

	require.NoError(t, shares.SetPrice(ctx, shareID, 70))

	var price, points int64
	rows := new(entsql.Rows)
	require.NoError(t, client.Driver().Query(ctx,
		`SELECT price, price_points FROM shares WHERE id = ?`,
		[]any{shareID}, rows))
	defer rows.Close()
	require.True(t, rows.Next())
	require.NoError(t, rows.Scan(&price, &points))
	require.Equal(t, int64(70), price)
	require.Equal(t, int64(70), points)
}

func TestSharePriceOnPlainCeSchema(t *testing.T) {
	_, shares, shareID := newPriceTestShare(t, false)
	ctx := context.Background()

	price, err := shares.GetPrice(ctx, shareID)
	require.NoError(t, err)
	require.Equal(t, int64(0), price)

	require.NoError(t, shares.SetPrice(ctx, shareID, 15))
	price, err = shares.GetPrice(ctx, shareID)
	require.NoError(t, err)
	require.Equal(t, int64(15), price)
}

func TestBackfillSharePrice(t *testing.T) {
	client, _, shareID := newPriceTestShare(t, true)
	ctx := context.Background()

	require.NoError(t, client.Driver().Exec(ctx,
		`UPDATE shares SET price = 0, price_points = 33 WHERE id = ?`,
		[]any{shareID}, new(sql.Result)))

	require.NoError(t, backfillSharePrice(ctx, client, nil))

	var price int64
	rows := new(entsql.Rows)
	require.NoError(t, client.Driver().Query(ctx,
		`SELECT price FROM shares WHERE id = ?`, []any{shareID}, rows))
	defer rows.Close()
	require.True(t, rows.Next())
	require.NoError(t, rows.Scan(&price))
	require.Equal(t, int64(33), price)
}
