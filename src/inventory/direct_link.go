package inventory

import (
	"context"
	"database/sql"
	"fmt"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/directlink"
	"github.com/cloudreve/Cloudreve/v4/ent/schema"
	"github.com/cloudreve/Cloudreve/v4/pkg/conf"
	"github.com/cloudreve/Cloudreve/v4/pkg/hashid"
)

const (
	DirectLinkTypeFile   = 0
	DirectLinkTypeFolder = 1
)

type (
	DirectLinkClient interface {
		TxOperator
		// GetByNameID get direct link by name and id
		GetByNameID(ctx context.Context, id int, name string) (*ent.DirectLink, error)
		// GetByID get direct link by id
		GetByID(ctx context.Context, id int) (*ent.DirectLink, error)
		// GetType gets the type of direct link, file or folder
		GetType(ctx context.Context, id int) (int, error)
		// SetType sets the type of direct link
		SetType(ctx context.Context, id int, linkType int) error
		// Delete delete direct link by id
		Delete(ctx context.Context, id int) error
	}
	LoadDirectLinkFile struct{}
)

func NewDirectLinkClient(client *ent.Client, dbType conf.DBType, hasher hashid.Encoder) DirectLinkClient {
	return &directLinkClient{
		client:      client,
		hasher:      hasher,
		maxSQlParam: sqlParamLimit(dbType),
	}
}

type directLinkClient struct {
	maxSQlParam int
	client      *ent.Client
	hasher      hashid.Encoder
}

func (c *directLinkClient) SetClient(newClient *ent.Client) TxOperator {
	return &directLinkClient{client: newClient, hasher: c.hasher, maxSQlParam: c.maxSQlParam}
}

func (c *directLinkClient) GetClient() *ent.Client {
	return c.client
}

func (d *directLinkClient) GetByID(ctx context.Context, id int) (*ent.DirectLink, error) {
	return withDirectLinkEagerLoading(ctx, d.client.DirectLink.Query().Where(directlink.ID(id))).
		First(ctx)
}

func (d *directLinkClient) GetByNameID(ctx context.Context, id int, name string) (*ent.DirectLink, error) {
	res, err := withDirectLinkEagerLoading(ctx, d.client.DirectLink.Query().Where(directlink.ID(id), directlink.Name(name))).
		First(ctx)
	if err != nil {
		return nil, err
	}

	// Increase download counter
	_, _ = d.client.DirectLink.Update().Where(directlink.ID(res.ID)).SetDownloads(res.Downloads + 1).Save(ctx)

	return res, nil
}

func (d *directLinkClient) Delete(ctx context.Context, id int) error {
	ctx = schema.SkipSoftDelete(ctx)
	_, err := d.client.DirectLink.Delete().Where(directlink.ID(id)).Exec(ctx)
	return err
}

// GetType reads the Pro-only direct link type column. It is kept out of the
// generated ent model so schema generation stays untouched.
func (d *directLinkClient) GetType(ctx context.Context, id int) (int, error) {
	query := "SELECT type FROM direct_links WHERE id = ?"
	if d.client.Driver().Dialect() == dialect.Postgres {
		query = "SELECT type FROM direct_links WHERE id = $1"
	}

	rows := new(entsql.Rows)
	if err := d.client.Driver().Query(ctx, query, []any{id}, rows); err != nil {
		return DirectLinkTypeFile, err
	}
	defer rows.Close()

	if !rows.Next() {
		return DirectLinkTypeFile, sql.ErrNoRows
	}

	var linkType int
	if err := rows.Scan(&linkType); err != nil {
		return DirectLinkTypeFile, err
	}
	return linkType, rows.Err()
}

func (d *directLinkClient) SetType(ctx context.Context, id int, linkType int) error {
	query := "UPDATE direct_links SET type = ? WHERE id = ?"
	args := []any{linkType, id}
	if d.client.Driver().Dialect() == dialect.Postgres {
		query = "UPDATE direct_links SET type = $1 WHERE id = $2"
	}

	if err := d.client.Driver().Exec(ctx, query, args, new(sql.Result)); err != nil {
		return fmt.Errorf("failed to update direct link type: %w", err)
	}
	return nil
}

func withDirectLinkEagerLoading(ctx context.Context, q *ent.DirectLinkQuery) *ent.DirectLinkQuery {
	if v, ok := ctx.Value(LoadDirectLinkFile{}).(bool); ok && v {
		q.WithFile(func(m *ent.FileQuery) {
			withFileEagerLoading(ctx, m)
		})
	}
	return q
}
