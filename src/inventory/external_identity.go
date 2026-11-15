package inventory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/cloudreve/Cloudreve/v4/ent"
)

// ErrExternalIdentityNotFound is returned when no linked third-party account
// exists for the requested provider and subject.
var ErrExternalIdentityNotFound = errors.New("external identity not found")

// ExternalIdentity is a third-party account linked to a Cloudreve user.
type ExternalIdentity struct {
	ID          int
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Provider    string
	Subject     string
	Email       string
	DisplayName string
	Avatar      string
	UserID      int
}

type (
	// ExternalIdentityClient stores third-party account bindings.
	ExternalIdentityClient interface {
		// GetBySubject returns the identity linked to provider + subject.
		GetBySubject(ctx context.Context, provider, subject string) (*ExternalIdentity, error)
		// ListByUser returns all identities linked to the given user.
		ListByUser(ctx context.Context, userID int) ([]*ExternalIdentity, error)
		// Create links a third-party account to a user.
		Create(ctx context.Context, identity *ExternalIdentity) (*ExternalIdentity, error)
		// UpdateProvider rewrites a legacy provider key in place.
		UpdateProvider(ctx context.Context, id int, provider string) error
		// DeleteByUserAndProvider unlinks a provider from a user.
		DeleteByUserAndProvider(ctx context.Context, userID int, provider string) error
	}
)

// NewExternalIdentityClient creates an ExternalIdentityClient backed by SQL so
// that the additive table does not require regenerating ent code.
func NewExternalIdentityClient(client *ent.Client) ExternalIdentityClient {
	return &externalIdentityClient{client: client}
}

type externalIdentityClient struct {
	client *ent.Client
}

func (c *externalIdentityClient) GetBySubject(ctx context.Context, provider, subject string) (*ExternalIdentity, error) {
	query, args := c.rebind(`SELECT id, created_at, updated_at, provider, subject, email, display_name, avatar, user_id
FROM external_identities WHERE provider = ? AND subject = ? LIMIT 1`, provider, subject)
	rows := new(entsql.Rows)
	if err := c.client.Driver().Query(ctx, query, args, rows); err != nil {
		return nil, fmt.Errorf("failed to query external identity: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, ErrExternalIdentityNotFound
	}

	return scanExternalIdentity(rows.ColumnScanner)
}

func (c *externalIdentityClient) ListByUser(ctx context.Context, userID int) ([]*ExternalIdentity, error) {
	query, args := c.rebind(`SELECT id, created_at, updated_at, provider, subject, email, display_name, avatar, user_id
FROM external_identities WHERE user_id = ? ORDER BY id ASC`, userID)
	rows := new(entsql.Rows)
	if err := c.client.Driver().Query(ctx, query, args, rows); err != nil {
		return nil, fmt.Errorf("failed to list external identities: %w", err)
	}
	defer rows.Close()

	var res []*ExternalIdentity
	for rows.Next() {
		identity, err := scanExternalIdentity(rows.ColumnScanner)
		if err != nil {
			return nil, err
		}
		res = append(res, identity)
	}

	return res, nil
}

func (c *externalIdentityClient) Create(ctx context.Context, identity *ExternalIdentity) (*ExternalIdentity, error) {
	now := time.Now()
	var id int64
	if c.client.Driver().Dialect() == dialect.Postgres {
		query, args := c.rebind(`INSERT INTO external_identities (created_at, updated_at, provider, subject, email, display_name, avatar, user_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`, now, now, identity.Provider, identity.Subject, identity.Email, identity.DisplayName, identity.Avatar, identity.UserID)
		rows := new(entsql.Rows)
		if err := c.client.Driver().Query(ctx, query, args, rows); err != nil {
			return nil, fmt.Errorf("failed to create external identity: %w", err)
		}
		defer rows.Close()
		if !rows.Next() {
			return nil, fmt.Errorf("failed to create external identity: no id returned")
		}
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to get external identity id: %w", err)
		}
	} else {
		query, args := c.rebind(`INSERT INTO external_identities (created_at, updated_at, provider, subject, email, display_name, avatar, user_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, now, now, identity.Provider, identity.Subject, identity.Email, identity.DisplayName, identity.Avatar, identity.UserID)
		lastID, err := c.insertReturningID(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("failed to create external identity: %w", err)
		}
		id = lastID
	}

	identity.ID = int(id)
	identity.CreatedAt = now
	identity.UpdatedAt = now
	return identity, nil
}

func (c *externalIdentityClient) DeleteByUserAndProvider(ctx context.Context, userID int, provider string) error {
	query, args := c.rebind(`DELETE FROM external_identities WHERE user_id = ? AND provider = ?`, userID, provider)
	var res sql.Result
	err := c.client.Driver().Exec(ctx, query, args, &res)
	if err != nil {
		return fmt.Errorf("failed to delete external identity: %w", err)
	}

	return nil
}

func (c *externalIdentityClient) UpdateProvider(ctx context.Context, id int, provider string) error {
	query, args := c.rebind(`UPDATE external_identities SET provider = ?, updated_at = ? WHERE id = ?`,
		provider, time.Now(), id)
	var res sql.Result
	if err := c.client.Driver().Exec(ctx, query, args, &res); err != nil {
		return fmt.Errorf("failed to migrate external identity provider: %w", err)
	}
	return nil
}

// insertReturningID executes an INSERT and returns the generated id.
// Postgres rejects sql.Result.LastInsertId, so it needs RETURNING instead.
func (c *externalIdentityClient) insertReturningID(ctx context.Context, query string, args ...any) (int64, error) {
	if c.client.Driver().Dialect() == dialect.Postgres {
		query, args = c.rebind(query+" RETURNING id", args...)
		rows := new(entsql.Rows)
		if err := c.client.Driver().Query(ctx, query, args, rows); err != nil {
			return 0, err
		}
		defer rows.Close()

		if !rows.Next() {
			return 0, sql.ErrNoRows
		}

		var id int64
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		return id, nil
	}

	var res sql.Result
	if err := c.client.Driver().Exec(ctx, query, args, &res); err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (c *externalIdentityClient) rebind(query string, args ...any) (string, []any) {
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

func scanExternalIdentity(rows entsql.ColumnScanner) (*ExternalIdentity, error) {
	identity := &ExternalIdentity{}
	var email, displayName, avatar sql.NullString
	if err := rows.Scan(
		&identity.ID,
		&identity.CreatedAt,
		&identity.UpdatedAt,
		&identity.Provider,
		&identity.Subject,
		&email,
		&displayName,
		&avatar,
		&identity.UserID,
	); err != nil {
		return nil, fmt.Errorf("failed to scan external identity: %w", err)
	}

	identity.Email = email.String
	identity.DisplayName = displayName.String
	identity.Avatar = avatar.String
	return identity, nil
}
