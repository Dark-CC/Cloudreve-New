package inventory

import (
	"context"
	"database/sql"
	"fmt"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/setting"
	"github.com/cloudreve/Cloudreve/v4/pkg/logging"
)

// ExecuteStatements runs every statement in order and returns the first error.
// Callers use it for additive schema work where partial creation is harmless
// because every statement is idempotent.
func ExecuteStatements(ctx context.Context, client *ent.Client, l logging.Logger, statements []string) error {
	drv := client.Driver()
	if drv == nil {
		return nil
	}

	for _, stmt := range statements {
		if err := drv.Exec(ctx, stmt, []any{}, new(sql.Result)); err != nil {
			if l != nil {
				l.Warning("Failed to execute additive schema statement: %s", err)
			}
			return fmt.Errorf("failed to execute additive schema statement: %w", err)
		}
	}

	return nil
}

// EnsureVASSchema creates tables backing credits, gift codes, storage packs,
// payments and share purchases. Statement order matters because of foreign keys.
func EnsureVASSchema(ctx context.Context, client *ent.Client, l logging.Logger) error {
	drv := client.Driver()
	if drv == nil {
		return nil
	}

	var statements []string
	switch drv.Dialect() {
	case dialect.MySQL:
		statements = []string{
			`CREATE TABLE IF NOT EXISTS ` + "`gift_codes`" + ` (
  ` + "`id`" + ` bigint NOT NULL AUTO_INCREMENT,
  ` + "`created_at`" + ` datetime NOT NULL,
  ` + "`updated_at`" + ` datetime NOT NULL,
  ` + "`deleted_at`" + ` datetime NULL,
  ` + "`code`" + ` char(36) NOT NULL,
  ` + "`used`" + ` bool NOT NULL DEFAULT false,
  ` + "`qyt`" + ` bigint NOT NULL DEFAULT 0,
  ` + "`product_props`" + ` json NOT NULL,
  ` + "`used_by`" + ` bigint NULL,
  PRIMARY KEY (` + "`id`" + `),
  UNIQUE KEY ` + "`gift_codes_code`" + ` (` + "`code`" + `),
  CONSTRAINT ` + "`gift_codes_users_used_by`" + ` FOREIGN KEY (` + "`used_by`" + `) REFERENCES ` + "`users`" + ` (` + "`id`" + `) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS ` + "`payments`" + ` (
  ` + "`id`" + ` bigint NOT NULL AUTO_INCREMENT,
  ` + "`created_at`" + ` datetime NOT NULL,
  ` + "`updated_at`" + ` datetime NOT NULL,
  ` + "`deleted_at`" + ` datetime NULL,
  ` + "`trade_no`" + ` text NOT NULL,
  ` + "`product_type`" + ` int NOT NULL DEFAULT 0,
  ` + "`product_props`" + ` json NOT NULL,
  ` + "`provider_id`" + ` char(36) NULL,
  ` + "`name`" + ` text NOT NULL,
  ` + "`price_unit`" + ` bigint NOT NULL DEFAULT 0,
  ` + "`price_mark`" + ` text NULL,
  ` + "`price_id`" + ` text NOT NULL,
  ` + "`provider_reference_id`" + ` text NULL,
  ` + "`price_one_unit`" + ` bigint NOT NULL DEFAULT 0,
  ` + "`qyt`" + ` bigint NOT NULL DEFAULT 0,
  ` + "`status`" + ` varchar(32) NOT NULL DEFAULT 'created',
  ` + "`anonymous_email`" + ` text NULL,
  ` + "`language`" + ` text NULL,
  ` + "`creator`" + ` bigint NULL,
  PRIMARY KEY (` + "`id`" + `),
  CONSTRAINT ` + "`payments_users_payments`" + ` FOREIGN KEY (` + "`creator`" + `) REFERENCES ` + "`users`" + ` (` + "`id`" + `) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS ` + "`storage_packs`" + ` (
  ` + "`id`" + ` bigint NOT NULL AUTO_INCREMENT,
  ` + "`name`" + ` text NOT NULL,
  ` + "`active_since`" + ` datetime NULL,
  ` + "`expire_at`" + ` datetime NULL,
  ` + "`size`" + ` bigint NOT NULL DEFAULT 0,
  ` + "`user_storage_packs`" + ` bigint NULL,
  PRIMARY KEY (` + "`id`" + `),
  CONSTRAINT ` + "`storage_packs_users_storage_packs`" + ` FOREIGN KEY (` + "`user_storage_packs`" + `) REFERENCES ` + "`users`" + ` (` + "`id`" + `) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS ` + "`share_purchases`" + ` (
  ` + "`id`" + ` bigint NOT NULL AUTO_INCREMENT,
  ` + "`created_at`" + ` datetime NOT NULL,
  ` + "`updated_at`" + ` datetime NOT NULL,
  ` + "`deleted_at`" + ` datetime NULL,
  ` + "`ticket_id`" + ` char(36) NULL,
  ` + "`payment_id`" + ` bigint NULL,
  ` + "`share_id`" + ` bigint NULL,
  ` + "`user_id`" + ` bigint NULL,
  PRIMARY KEY (` + "`id`" + `),
  UNIQUE KEY ` + "`share_purchases_ticket_id`" + ` (` + "`ticket_id`" + `),
  CONSTRAINT ` + "`share_purchases_payments_share_purchases`" + ` FOREIGN KEY (` + "`payment_id`" + `) REFERENCES ` + "`payments`" + ` (` + "`id`" + `) ON DELETE SET NULL,
  CONSTRAINT ` + "`share_purchases_shares_share_purchases`" + ` FOREIGN KEY (` + "`share_id`" + `) REFERENCES ` + "`shares`" + ` (` + "`id`" + `) ON DELETE SET NULL,
  CONSTRAINT ` + "`share_purchases_users_share_purchases`" + ` FOREIGN KEY (` + "`user_id`" + `) REFERENCES ` + "`users`" + ` (` + "`id`" + `) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS ` + "`credit_changes`" + ` (
  ` + "`id`" + ` bigint NOT NULL AUTO_INCREMENT,
  ` + "`created_at`" + ` datetime NOT NULL,
  ` + "`user_id`" + ` bigint NOT NULL,
  ` + "`change`" + ` bigint NOT NULL,
  ` + "`balance`" + ` bigint NOT NULL,
  ` + "`reason`" + ` varchar(64) NOT NULL,
  ` + "`detail`" + ` text NULL,
  ` + "`payment_id`" + ` bigint NULL,
  PRIMARY KEY (` + "`id`" + `),
  KEY ` + "`credit_changes_user_id`" + ` (` + "`user_id`" + `, ` + "`id`" + `),
  CONSTRAINT ` + "`credit_changes_users_credit_changes`" + ` FOREIGN KEY (` + "`user_id`" + `) REFERENCES ` + "`users`" + ` (` + "`id`" + `) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		}
	case dialect.Postgres:
		statements = []string{
			`CREATE TABLE IF NOT EXISTS "gift_codes" (
  "id" bigserial NOT NULL,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  "deleted_at" timestamptz NULL,
  "code" uuid NOT NULL,
  "used" bool NOT NULL DEFAULT false,
  "qyt" bigint NOT NULL DEFAULT 0,
  "product_props" jsonb NOT NULL,
  "used_by" bigint NULL REFERENCES "users" ("id") ON DELETE SET NULL,
  PRIMARY KEY ("id"),
  UNIQUE ("code")
)`,
			`CREATE TABLE IF NOT EXISTS "payments" (
  "id" bigserial NOT NULL,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  "deleted_at" timestamptz NULL,
  "trade_no" text NOT NULL,
  "product_type" int NOT NULL DEFAULT 0,
  "product_props" jsonb NOT NULL,
  "provider_id" uuid NULL,
  "name" text NOT NULL,
  "price_unit" bigint NOT NULL DEFAULT 0,
  "price_mark" text NULL,
  "price_id" text NOT NULL,
  "provider_reference_id" text NULL,
  "price_one_unit" bigint NOT NULL DEFAULT 0,
  "qyt" bigint NOT NULL DEFAULT 0,
  "status" varchar(32) NOT NULL DEFAULT 'created',
  "anonymous_email" text NULL,
  "language" text NULL,
  "creator" bigint NULL REFERENCES "users" ("id") ON DELETE SET NULL,
  PRIMARY KEY ("id")
)`,
			`CREATE TABLE IF NOT EXISTS "storage_packs" (
  "id" bigserial NOT NULL,
  "name" text NOT NULL,
  "active_since" timestamptz NULL,
  "expire_at" timestamptz NULL,
  "size" bigint NOT NULL DEFAULT 0,
  "user_storage_packs" bigint NULL REFERENCES "users" ("id") ON DELETE SET NULL,
  PRIMARY KEY ("id")
)`,
			`CREATE TABLE IF NOT EXISTS "share_purchases" (
  "id" bigserial NOT NULL,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  "deleted_at" timestamptz NULL,
  "ticket_id" uuid NULL,
  "payment_id" bigint NULL REFERENCES "payments" ("id") ON DELETE SET NULL,
  "share_id" bigint NULL REFERENCES "shares" ("id") ON DELETE SET NULL,
  "user_id" bigint NULL REFERENCES "users" ("id") ON DELETE SET NULL,
  PRIMARY KEY ("id"),
  UNIQUE ("ticket_id")
)`,
			`CREATE TABLE IF NOT EXISTS "credit_changes" (
  "id" bigserial NOT NULL,
  "created_at" timestamptz NOT NULL,
  "user_id" bigint NOT NULL REFERENCES "users" ("id") ON DELETE CASCADE,
  "change" bigint NOT NULL,
  "balance" bigint NOT NULL,
  "reason" varchar(64) NOT NULL,
  "detail" text NULL,
  "payment_id" bigint NULL,
  PRIMARY KEY ("id")
)`,
			`CREATE INDEX IF NOT EXISTS "credit_changes_user_id" ON "credit_changes" ("user_id", "id")`,
		}
	default:
		statements = []string{
			`CREATE TABLE IF NOT EXISTS ` + "`gift_codes`" + ` (
  ` + "`id`" + ` integer NOT NULL PRIMARY KEY AUTOINCREMENT,
  ` + "`created_at`" + ` datetime NOT NULL,
  ` + "`updated_at`" + ` datetime NOT NULL,
  ` + "`deleted_at`" + ` datetime NULL,
  ` + "`code`" + ` uuid NOT NULL,
  ` + "`used`" + ` bool NOT NULL DEFAULT (false),
  ` + "`qyt`" + ` integer NOT NULL DEFAULT 0,
  ` + "`product_props`" + ` json NOT NULL,
  ` + "`used_by`" + ` integer NULL,
  CONSTRAINT ` + "`gift_codes_users_used_by`" + ` FOREIGN KEY (` + "`used_by`" + `) REFERENCES ` + "`users`" + ` (` + "`id`" + `) ON DELETE SET NULL
)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS ` + "`gift_codes_code`" + ` ON ` + "`gift_codes`" + ` (` + "`code`" + `)`,
			`CREATE TABLE IF NOT EXISTS ` + "`payments`" + ` (
  ` + "`id`" + ` integer NOT NULL PRIMARY KEY AUTOINCREMENT,
  ` + "`created_at`" + ` datetime NOT NULL,
  ` + "`updated_at`" + ` datetime NOT NULL,
  ` + "`deleted_at`" + ` datetime NULL,
  ` + "`trade_no`" + ` text NOT NULL,
  ` + "`product_type`" + ` integer NOT NULL DEFAULT 0,
  ` + "`product_props`" + ` json NOT NULL,
  ` + "`provider_id`" + ` uuid NULL,
  ` + "`name`" + ` text NOT NULL,
  ` + "`price_unit`" + ` integer NOT NULL DEFAULT 0,
  ` + "`price_mark`" + ` text NULL,
  ` + "`price_id`" + ` text NOT NULL,
  ` + "`provider_reference_id`" + ` text NULL,
  ` + "`price_one_unit`" + ` integer NOT NULL DEFAULT 0,
  ` + "`qyt`" + ` integer NOT NULL DEFAULT 0,
  ` + "`status`" + ` text NOT NULL DEFAULT ('created'),
  ` + "`anonymous_email`" + ` text NULL,
  ` + "`language`" + ` text NULL,
  ` + "`creator`" + ` integer NULL,
  CONSTRAINT ` + "`payments_users_payments`" + ` FOREIGN KEY (` + "`creator`" + `) REFERENCES ` + "`users`" + ` (` + "`id`" + `) ON DELETE SET NULL
)`,
			`CREATE TABLE IF NOT EXISTS ` + "`storage_packs`" + ` (
  ` + "`id`" + ` integer NOT NULL PRIMARY KEY AUTOINCREMENT,
  ` + "`name`" + ` text NOT NULL,
  ` + "`active_since`" + ` datetime NULL,
  ` + "`expire_at`" + ` datetime NULL,
  ` + "`size`" + ` integer NOT NULL DEFAULT 0,
  ` + "`user_storage_packs`" + ` integer NULL,
  CONSTRAINT ` + "`storage_packs_users_storage_packs`" + ` FOREIGN KEY (` + "`user_storage_packs`" + `) REFERENCES ` + "`users`" + ` (` + "`id`" + `) ON DELETE SET NULL
)`,
			`CREATE TABLE IF NOT EXISTS ` + "`share_purchases`" + ` (
  ` + "`id`" + ` integer NOT NULL PRIMARY KEY AUTOINCREMENT,
  ` + "`created_at`" + ` datetime NOT NULL,
  ` + "`updated_at`" + ` datetime NOT NULL,
  ` + "`deleted_at`" + ` datetime NULL,
  ` + "`ticket_id`" + ` uuid NULL,
  ` + "`payment_id`" + ` integer NULL,
  ` + "`share_id`" + ` integer NULL,
  ` + "`user_id`" + ` integer NULL,
  CONSTRAINT ` + "`share_purchases_payments_share_purchases`" + ` FOREIGN KEY (` + "`payment_id`" + `) REFERENCES ` + "`payments`" + ` (` + "`id`" + `) ON DELETE SET NULL,
  CONSTRAINT ` + "`share_purchases_shares_share_purchases`" + ` FOREIGN KEY (` + "`share_id`" + `) REFERENCES ` + "`shares`" + ` (` + "`id`" + `) ON DELETE SET NULL,
  CONSTRAINT ` + "`share_purchases_users_share_purchases`" + ` FOREIGN KEY (` + "`user_id`" + `) REFERENCES ` + "`users`" + ` (` + "`id`" + `) ON DELETE SET NULL
)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS ` + "`share_purchases_ticket_id`" + ` ON ` + "`share_purchases`" + ` (` + "`ticket_id`" + `)`,
			`CREATE TABLE IF NOT EXISTS ` + "`credit_changes`" + ` (
  ` + "`id`" + ` integer NOT NULL PRIMARY KEY AUTOINCREMENT,
  ` + "`created_at`" + ` datetime NOT NULL,
  ` + "`user_id`" + ` integer NOT NULL,
  ` + "`change`" + ` integer NOT NULL,
  ` + "`balance`" + ` integer NOT NULL,
  ` + "`reason`" + ` varchar(64) NOT NULL,
  ` + "`detail`" + ` text NULL,
  ` + "`payment_id`" + ` integer NULL,
  CONSTRAINT ` + "`credit_changes_users_credit_changes`" + ` FOREIGN KEY (` + "`user_id`" + `) REFERENCES ` + "`users`" + ` (` + "`id`" + `) ON DELETE CASCADE
)`,
			`CREATE INDEX IF NOT EXISTS ` + "`credit_changes_user_id`" + ` ON ` + "`credit_changes`" + ` (` + "`user_id`" + `, ` + "`id`" + `)`,
		}
	}

	if err := ExecuteStatements(ctx, client, l, statements); err != nil {
		return err
	}

	// Older community builds stored Go's zero time for permanent packs. SQLite
	// keeps that sentinel as an ordinary text value, so normalize it to NULL.
	if drv.Dialect() != dialect.Postgres && drv.Dialect() != dialect.MySQL {
		cleanup := `UPDATE storage_packs SET expire_at = NULL WHERE expire_at IS NOT NULL AND expire_at < '1900-01-01 00:00:00'`
		if err := drv.Exec(ctx, cleanup, []any{}, new(sql.Result)); err != nil {
			if l != nil {
				l.Warning("Failed to normalize permanent storage pack expiry: %s", err)
			}
		}
	}

	return nil
}

// EnsureShareACLSchema creates the Pro-only fine grained share permission
// tables together with the ancestor-permission helper table.
func EnsureShareACLSchema(ctx context.Context, client *ent.Client, l logging.Logger) error {
	drv := client.Driver()
	if drv == nil {
		return nil
	}

	var statements []string
	switch drv.Dialect() {
	case dialect.MySQL:
		statements = []string{
			`CREATE TABLE IF NOT EXISTS ` + "`file_ancestor_permissions`" + ` (
  ` + "`file_id`" + ` bigint NOT NULL,
  ` + "`descendant_permission_id`" + ` bigint NOT NULL,
  PRIMARY KEY (` + "`file_id`" + `, ` + "`descendant_permission_id`" + `),
  CONSTRAINT ` + "`file_ancestor_permissions_file_id`" + ` FOREIGN KEY (` + "`file_id`" + `) REFERENCES ` + "`files`" + ` (` + "`id`" + `) ON DELETE CASCADE,
  CONSTRAINT ` + "`file_ancestor_permissions_descendant_permission_id`" + ` FOREIGN KEY (` + "`descendant_permission_id`" + `) REFERENCES ` + "`files`" + ` (` + "`id`" + `) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS ` + "`share_builtin_permissions`" + ` (
  ` + "`id`" + ` bigint NOT NULL AUTO_INCREMENT,
  ` + "`audience`" + ` varchar(32) NOT NULL,
  ` + "`can_list`" + ` bool NOT NULL DEFAULT false,
  ` + "`can_read`" + ` bool NOT NULL DEFAULT false,
  ` + "`can_update`" + ` bool NOT NULL DEFAULT false,
  ` + "`can_create`" + ` bool NOT NULL DEFAULT false,
  ` + "`can_delete`" + ` bool NOT NULL DEFAULT false,
  ` + "`can_share`" + ` bool NOT NULL DEFAULT false,
  ` + "`discoverable`" + ` bool NOT NULL DEFAULT false,
  ` + "`share_id`" + ` bigint NOT NULL,
  PRIMARY KEY (` + "`id`" + `),
  CONSTRAINT ` + "`share_builtin_permissions_shares_builtin_permissions`" + ` FOREIGN KEY (` + "`share_id`" + `) REFERENCES ` + "`shares`" + ` (` + "`id`" + `) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS ` + "`share_group_permissions`" + ` (
  ` + "`id`" + ` bigint NOT NULL AUTO_INCREMENT,
  ` + "`can_list`" + ` bool NOT NULL DEFAULT false,
  ` + "`can_read`" + ` bool NOT NULL DEFAULT false,
  ` + "`can_update`" + ` bool NOT NULL DEFAULT false,
  ` + "`can_create`" + ` bool NOT NULL DEFAULT false,
  ` + "`can_delete`" + ` bool NOT NULL DEFAULT false,
  ` + "`can_share`" + ` bool NOT NULL DEFAULT false,
  ` + "`discoverable`" + ` bool NOT NULL DEFAULT false,
  ` + "`group_id`" + ` bigint NOT NULL,
  ` + "`share_id`" + ` bigint NOT NULL,
  PRIMARY KEY (` + "`id`" + `),
  CONSTRAINT ` + "`share_group_permissions_groups_share_permissions`" + ` FOREIGN KEY (` + "`group_id`" + `) REFERENCES ` + "`groups`" + ` (` + "`id`" + `) ON DELETE CASCADE,
  CONSTRAINT ` + "`share_group_permissions_shares_group_permissions`" + ` FOREIGN KEY (` + "`share_id`" + `) REFERENCES ` + "`shares`" + ` (` + "`id`" + `) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS ` + "`share_user_permissions`" + ` (
  ` + "`id`" + ` bigint NOT NULL AUTO_INCREMENT,
  ` + "`can_list`" + ` bool NOT NULL DEFAULT false,
  ` + "`can_read`" + ` bool NOT NULL DEFAULT false,
  ` + "`can_update`" + ` bool NOT NULL DEFAULT false,
  ` + "`can_create`" + ` bool NOT NULL DEFAULT false,
  ` + "`can_delete`" + ` bool NOT NULL DEFAULT false,
  ` + "`can_share`" + ` bool NOT NULL DEFAULT false,
  ` + "`discoverable`" + ` bool NOT NULL DEFAULT false,
  ` + "`share_id`" + ` bigint NOT NULL,
  ` + "`user_id`" + ` bigint NOT NULL,
  PRIMARY KEY (` + "`id`" + `),
  CONSTRAINT ` + "`share_user_permissions_shares_user_permissions`" + ` FOREIGN KEY (` + "`share_id`" + `) REFERENCES ` + "`shares`" + ` (` + "`id`" + `) ON DELETE CASCADE,
  CONSTRAINT ` + "`share_user_permissions_users_share_permissions`" + ` FOREIGN KEY (` + "`user_id`" + `) REFERENCES ` + "`users`" + ` (` + "`id`" + `) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		}
	case dialect.Postgres:
		statements = []string{
			`CREATE TABLE IF NOT EXISTS "file_ancestor_permissions" (
  "file_id" bigint NOT NULL REFERENCES "files" ("id") ON DELETE CASCADE,
  "descendant_permission_id" bigint NOT NULL REFERENCES "files" ("id") ON DELETE CASCADE,
  PRIMARY KEY ("file_id", "descendant_permission_id")
)`,
			`CREATE TABLE IF NOT EXISTS "share_builtin_permissions" (
  "id" bigserial NOT NULL,
  "audience" varchar(32) NOT NULL,
  "can_list" bool NOT NULL DEFAULT false,
  "can_read" bool NOT NULL DEFAULT false,
  "can_update" bool NOT NULL DEFAULT false,
  "can_create" bool NOT NULL DEFAULT false,
  "can_delete" bool NOT NULL DEFAULT false,
  "can_share" bool NOT NULL DEFAULT false,
  "discoverable" bool NOT NULL DEFAULT false,
  "share_id" bigint NOT NULL REFERENCES "shares" ("id") ON DELETE CASCADE,
  PRIMARY KEY ("id")
)`,
			`CREATE TABLE IF NOT EXISTS "share_group_permissions" (
  "id" bigserial NOT NULL,
  "can_list" bool NOT NULL DEFAULT false,
  "can_read" bool NOT NULL DEFAULT false,
  "can_update" bool NOT NULL DEFAULT false,
  "can_create" bool NOT NULL DEFAULT false,
  "can_delete" bool NOT NULL DEFAULT false,
  "can_share" bool NOT NULL DEFAULT false,
  "discoverable" bool NOT NULL DEFAULT false,
  "group_id" bigint NOT NULL REFERENCES "groups" ("id") ON DELETE CASCADE,
  "share_id" bigint NOT NULL REFERENCES "shares" ("id") ON DELETE CASCADE,
  PRIMARY KEY ("id")
)`,
			`CREATE TABLE IF NOT EXISTS "share_user_permissions" (
  "id" bigserial NOT NULL,
  "can_list" bool NOT NULL DEFAULT false,
  "can_read" bool NOT NULL DEFAULT false,
  "can_update" bool NOT NULL DEFAULT false,
  "can_create" bool NOT NULL DEFAULT false,
  "can_delete" bool NOT NULL DEFAULT false,
  "can_share" bool NOT NULL DEFAULT false,
  "discoverable" bool NOT NULL DEFAULT false,
  "share_id" bigint NOT NULL REFERENCES "shares" ("id") ON DELETE CASCADE,
  "user_id" bigint NOT NULL REFERENCES "users" ("id") ON DELETE CASCADE,
  PRIMARY KEY ("id")
)`,
		}
	default:
		statements = []string{
			`CREATE TABLE IF NOT EXISTS ` + "`file_ancestor_permissions`" + ` (
  ` + "`file_id`" + ` integer NOT NULL,
  ` + "`descendant_permission_id`" + ` integer NOT NULL,
  PRIMARY KEY (` + "`file_id`" + `, ` + "`descendant_permission_id`" + `),
  CONSTRAINT ` + "`file_ancestor_permissions_file_id`" + ` FOREIGN KEY (` + "`file_id`" + `) REFERENCES ` + "`files`" + ` (` + "`id`" + `) ON DELETE CASCADE,
  CONSTRAINT ` + "`file_ancestor_permissions_descendant_permission_id`" + ` FOREIGN KEY (` + "`descendant_permission_id`" + `) REFERENCES ` + "`files`" + ` (` + "`id`" + `) ON DELETE CASCADE
)`,
			`CREATE TABLE IF NOT EXISTS ` + "`share_builtin_permissions`" + ` (
  ` + "`id`" + ` integer NOT NULL PRIMARY KEY AUTOINCREMENT,
  ` + "`audience`" + ` text NOT NULL,
  ` + "`can_list`" + ` bool NOT NULL DEFAULT (false),
  ` + "`can_read`" + ` bool NOT NULL DEFAULT (false),
  ` + "`can_update`" + ` bool NOT NULL DEFAULT (false),
  ` + "`can_create`" + ` bool NOT NULL DEFAULT (false),
  ` + "`can_delete`" + ` bool NOT NULL DEFAULT (false),
  ` + "`can_share`" + ` bool NOT NULL DEFAULT (false),
  ` + "`discoverable`" + ` bool NOT NULL DEFAULT (false),
  ` + "`share_id`" + ` integer NOT NULL,
  CONSTRAINT ` + "`share_builtin_permissions_shares_builtin_permissions`" + ` FOREIGN KEY (` + "`share_id`" + `) REFERENCES ` + "`shares`" + ` (` + "`id`" + `) ON DELETE CASCADE
)`,
			`CREATE TABLE IF NOT EXISTS ` + "`share_group_permissions`" + ` (
  ` + "`id`" + ` integer NOT NULL PRIMARY KEY AUTOINCREMENT,
  ` + "`can_list`" + ` bool NOT NULL DEFAULT (false),
  ` + "`can_read`" + ` bool NOT NULL DEFAULT (false),
  ` + "`can_update`" + ` bool NOT NULL DEFAULT (false),
  ` + "`can_create`" + ` bool NOT NULL DEFAULT (false),
  ` + "`can_delete`" + ` bool NOT NULL DEFAULT (false),
  ` + "`can_share`" + ` bool NOT NULL DEFAULT (false),
  ` + "`discoverable`" + ` bool NOT NULL DEFAULT (false),
  ` + "`group_id`" + ` integer NOT NULL,
  ` + "`share_id`" + ` integer NOT NULL,
  CONSTRAINT ` + "`share_group_permissions_groups_share_permissions`" + ` FOREIGN KEY (` + "`group_id`" + `) REFERENCES ` + "`groups`" + ` (` + "`id`" + `) ON DELETE CASCADE,
  CONSTRAINT ` + "`share_group_permissions_shares_group_permissions`" + ` FOREIGN KEY (` + "`share_id`" + `) REFERENCES ` + "`shares`" + ` (` + "`id`" + `) ON DELETE CASCADE
)`,
			`CREATE TABLE IF NOT EXISTS ` + "`share_user_permissions`" + ` (
  ` + "`id`" + ` integer NOT NULL PRIMARY KEY AUTOINCREMENT,
  ` + "`can_list`" + ` bool NOT NULL DEFAULT (false),
  ` + "`can_read`" + ` bool NOT NULL DEFAULT (false),
  ` + "`can_update`" + ` bool NOT NULL DEFAULT (false),
  ` + "`can_create`" + ` bool NOT NULL DEFAULT (false),
  ` + "`can_delete`" + ` bool NOT NULL DEFAULT (false),
  ` + "`can_share`" + ` bool NOT NULL DEFAULT (false),
  ` + "`discoverable`" + ` bool NOT NULL DEFAULT (false),
  ` + "`share_id`" + ` integer NOT NULL,
  ` + "`user_id`" + ` integer NOT NULL,
  CONSTRAINT ` + "`share_user_permissions_shares_user_permissions`" + ` FOREIGN KEY (` + "`share_id`" + `) REFERENCES ` + "`shares`" + ` (` + "`id`" + `) ON DELETE CASCADE,
  CONSTRAINT ` + "`share_user_permissions_users_share_permissions`" + ` FOREIGN KEY (` + "`user_id`" + `) REFERENCES ` + "`users`" + ` (` + "`id`" + `) ON DELETE CASCADE
)`,
		}
	}

	return ExecuteStatements(ctx, client, l, statements)
}

// EnsureGroupPolicySchema creates the many-to-many storage policy binding used
// by Pro groups, plus load balancing policy weights.
func EnsureGroupPolicySchema(ctx context.Context, client *ent.Client, l logging.Logger) error {
	drv := client.Driver()
	if drv == nil {
		return nil
	}

	var statements []string
	switch drv.Dialect() {
	case dialect.MySQL:
		statements = []string{
			`CREATE TABLE IF NOT EXISTS ` + "`group_storage_policies`" + ` (
  ` + "`group_id`" + ` bigint NOT NULL,
  ` + "`storage_policy_id`" + ` bigint NOT NULL,
  PRIMARY KEY (` + "`group_id`" + `, ` + "`storage_policy_id`" + `),
  CONSTRAINT ` + "`group_storage_policies_group_id`" + ` FOREIGN KEY (` + "`group_id`" + `) REFERENCES ` + "`groups`" + ` (` + "`id`" + `) ON DELETE CASCADE,
  CONSTRAINT ` + "`group_storage_policies_storage_policy_id`" + ` FOREIGN KEY (` + "`storage_policy_id`" + `) REFERENCES ` + "`storage_policies`" + ` (` + "`id`" + `) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS ` + "`policy_load_balances`" + ` (
  ` + "`id`" + ` bigint NOT NULL AUTO_INCREMENT,
  ` + "`weight`" + ` int NOT NULL DEFAULT 1,
  ` + "`storage_policy_id`" + ` bigint NOT NULL,
  ` + "`load_balance_id`" + ` bigint NOT NULL,
  PRIMARY KEY (` + "`id`" + `),
  CONSTRAINT ` + "`policy_load_balances_storage_policies_policy`" + ` FOREIGN KEY (` + "`storage_policy_id`" + `) REFERENCES ` + "`storage_policies`" + ` (` + "`id`" + `) ON DELETE CASCADE,
  CONSTRAINT ` + "`policy_load_balances_storage_policies_target`" + ` FOREIGN KEY (` + "`load_balance_id`" + `) REFERENCES ` + "`storage_policies`" + ` (` + "`id`" + `) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		}
	case dialect.Postgres:
		statements = []string{
			`CREATE TABLE IF NOT EXISTS "group_storage_policies" (
  "group_id" bigint NOT NULL REFERENCES "groups" ("id") ON DELETE CASCADE,
  "storage_policy_id" bigint NOT NULL REFERENCES "storage_policies" ("id") ON DELETE CASCADE,
  PRIMARY KEY ("group_id", "storage_policy_id")
)`,
			`CREATE TABLE IF NOT EXISTS "policy_load_balances" (
  "id" bigserial NOT NULL,
  "weight" int NOT NULL DEFAULT 1,
  "storage_policy_id" bigint NOT NULL REFERENCES "storage_policies" ("id") ON DELETE CASCADE,
  "load_balance_id" bigint NOT NULL REFERENCES "storage_policies" ("id") ON DELETE CASCADE,
  PRIMARY KEY ("id")
)`,
		}
	default:
		statements = []string{
			`CREATE TABLE IF NOT EXISTS ` + "`group_storage_policies`" + ` (
  ` + "`group_id`" + ` integer NOT NULL,
  ` + "`storage_policy_id`" + ` integer NOT NULL,
  PRIMARY KEY (` + "`group_id`" + `, ` + "`storage_policy_id`" + `),
  CONSTRAINT ` + "`group_storage_policies_group_id`" + ` FOREIGN KEY (` + "`group_id`" + `) REFERENCES ` + "`groups`" + ` (` + "`id`" + `) ON DELETE CASCADE,
  CONSTRAINT ` + "`group_storage_policies_storage_policy_id`" + ` FOREIGN KEY (` + "`storage_policy_id`" + `) REFERENCES ` + "`storage_policies`" + ` (` + "`id`" + `) ON DELETE CASCADE
)`,
			`CREATE TABLE IF NOT EXISTS ` + "`policy_load_balances`" + ` (
  ` + "`id`" + ` integer NOT NULL PRIMARY KEY AUTOINCREMENT,
  ` + "`weight`" + ` integer NOT NULL DEFAULT 1,
  ` + "`storage_policy_id`" + ` integer NOT NULL,
  ` + "`load_balance_id`" + ` integer NOT NULL,
  CONSTRAINT ` + "`policy_load_balances_storage_policies_policy`" + ` FOREIGN KEY (` + "`storage_policy_id`" + `) REFERENCES ` + "`storage_policies`" + ` (` + "`id`" + `) ON DELETE CASCADE,
  CONSTRAINT ` + "`policy_load_balances_storage_policies_target`" + ` FOREIGN KEY (` + "`load_balance_id`" + `) REFERENCES ` + "`storage_policies`" + ` (` + "`id`" + `) ON DELETE CASCADE
)`,
		}
	}

	return ExecuteStatements(ctx, client, l, statements)
}

// MigrateVASProductTypes converts orders written with the community
// edition's original product type order to the target wire values. The migration is
// recorded in settings so repeated boots never reinterpret already migrated
// orders again.
func MigrateVASProductTypes(ctx context.Context, client *ent.Client, l logging.Logger) error {
	drv := client.Driver()
	if drv == nil {
		return nil
	}

	const versionKey = "vas_product_type_version"
	stored, err := client.Setting.Query().Where(setting.Name(versionKey)).Only(ctx)
	if err == nil && stored.Value == "1" {
		return nil
	}
	versionMissing := ent.IsNotFound(err)
	if err != nil && !versionMissing {
		return fmt.Errorf("failed to read VAS product type migration version: %w", err)
	}

	tx, err := client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("failed to start VAS product type migration: %w", err)
	}

	query, args := "UPDATE payments SET product_type = CASE product_type WHEN 0 THEN 4 WHEN 1 THEN 3 WHEN 3 THEN 1 ELSE product_type END WHERE product_type IN (0, 1, 3)", []any{}
	if drv.Dialect() == dialect.Postgres {
		query = "UPDATE payments SET product_type = CASE product_type WHEN 0 THEN 4 WHEN 1 THEN 3 WHEN 3 THEN 1 ELSE product_type END WHERE product_type IN (0, 1, 3)"
	}
	var res sql.Result
	if err := tx.Client().Driver().Exec(ctx, query, args, &res); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("failed to migrate VAS product types: %w", err)
	}

	if versionMissing {
		if _, createErr := tx.Setting.Create().SetName(versionKey).SetValue("1").Save(ctx); createErr != nil {
			_ = tx.Rollback()
			return fmt.Errorf("failed to record VAS product type migration: %w", createErr)
		}
	} else if err := tx.Setting.Update().Where(setting.Name(versionKey)).SetValue("1").Exec(ctx); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("failed to record VAS product type migration: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit VAS product type migration: %w", err)
	}
	if l != nil {
		l.Info("Migrated VAS product types to Pro-compatible values.")
	}
	return nil
}

// EnsureOpenIDsSchema creates the upstream open_ids table. This implementation keeps
// external_identities as the full profile store and mirrors links into open_ids
// so databases remain interchangeable with Pro installations.
func EnsureOpenIDsSchema(ctx context.Context, client *ent.Client, l logging.Logger) error {
	drv := client.Driver()
	if drv == nil {
		return nil
	}

	var statements []string
	switch drv.Dialect() {
	case dialect.MySQL:
		statements = []string{`
CREATE TABLE IF NOT EXISTS ` + "`open_ids`" + ` (
  ` + "`id`" + ` bigint NOT NULL AUTO_INCREMENT,
  ` + "`created_at`" + ` datetime NOT NULL,
  ` + "`updated_at`" + ` datetime NOT NULL,
  ` + "`deleted_at`" + ` datetime NULL,
  ` + "`provider`" + ` int NOT NULL,
  ` + "`sub`" + ` text NOT NULL,
  ` + "`user_id`" + ` bigint NULL,
  PRIMARY KEY (` + "`id`" + `),
  KEY ` + "`open_ids_user_id`" + ` (` + "`user_id`" + `),
  KEY ` + "`open_ids_provider_sub`" + ` (` + "`provider`" + `, ` + "`sub`" + `(191)),
  CONSTRAINT ` + "`open_ids_users_open_id`" + ` FOREIGN KEY (` + "`user_id`" + `) REFERENCES ` + "`users`" + ` (` + "`id`" + `) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`}
	case dialect.Postgres:
		statements = []string{`
CREATE TABLE IF NOT EXISTS "open_ids" (
  "id" bigserial NOT NULL,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  "deleted_at" timestamptz NULL,
  "provider" integer NOT NULL,
  "sub" text NOT NULL,
  "user_id" bigint NULL REFERENCES "users" ("id") ON DELETE SET NULL,
  PRIMARY KEY ("id")
);
CREATE INDEX IF NOT EXISTS "open_ids_user_id" ON "open_ids" ("user_id");
CREATE INDEX IF NOT EXISTS "open_ids_provider_sub" ON "open_ids" ("provider", "sub")`}
	default:
		statements = []string{`
CREATE TABLE IF NOT EXISTS ` + "`open_ids`" + ` (
  ` + "`id`" + ` integer NOT NULL PRIMARY KEY AUTOINCREMENT,
  ` + "`created_at`" + ` datetime NOT NULL,
  ` + "`updated_at`" + ` datetime NOT NULL,
  ` + "`deleted_at`" + ` datetime NULL,
  ` + "`provider`" + ` integer NOT NULL,
  ` + "`sub`" + ` text NOT NULL,
  ` + "`user_id`" + ` integer NULL,
  CONSTRAINT ` + "`open_ids_users_open_id`" + ` FOREIGN KEY (` + "`user_id`" + `) REFERENCES ` + "`users`" + ` (` + "`id`" + `) ON DELETE SET NULL
)`,
			`CREATE INDEX IF NOT EXISTS ` + "`open_ids_user_id`" + ` ON ` + "`open_ids`" + ` (` + "`user_id`" + `)`,
			`CREATE INDEX IF NOT EXISTS ` + "`open_ids_provider_sub`" + ` ON ` + "`open_ids`" + ` (` + "`provider`" + `, ` + "`sub`" + `)`}
	}

	for _, stmt := range statements {
		if err := drv.Exec(ctx, stmt, []any{}, new(sql.Result)); err != nil {
			if l != nil {
				l.Warning("Failed to ensure open_ids schema: %s", err)
			}
			return fmt.Errorf("failed to ensure open_ids schema: %w", err)
		}
	}

	return nil
}

// EnsureProColumns adds the Pro-only columns on users and shares. Each dialect
// gets an explicit statement because column types differ; the idempotency is
// provided by existsColumn, so repeated boots stay silent.
func EnsureProColumns(ctx context.Context, client *ent.Client, l logging.Logger) error {
	drv := client.Driver()
	if drv == nil {
		return nil
	}

	type column struct {
		table string
		name  string
		mysql string
		pg    string
		other string
	}

	columns := []column{
		{"users", "credit", "BIGINT NOT NULL DEFAULT 0", "bigint NOT NULL DEFAULT 0", "integer NOT NULL DEFAULT 0"},
		{"users", "group_expires", "DATETIME NULL", "timestamptz NULL", "datetime NULL"},
		{"users", "notify_date", "DATETIME NULL", "timestamptz NULL", "datetime NULL"},
		{"users", "unmanaged_email", "TINYINT(1) NULL", "bool NULL", "bool NULL"},
		{"users", "previous_group", "BIGINT NULL", "bigint NULL", "integer NULL"},
		{"shares", "price", "BIGINT NOT NULL DEFAULT 0", "bigint NOT NULL DEFAULT 0", "integer NOT NULL DEFAULT 0"},
		{"shares", "permissions", "JSON NULL", "jsonb NULL", "json NULL"},
		{"direct_links", "type", "INT NOT NULL DEFAULT 0", "integer NOT NULL DEFAULT 0", "integer NOT NULL DEFAULT 0"},
		{"files", "permissions", "JSON NULL", "jsonb NULL", "json NULL"},
	}

	for _, col := range columns {
		exists, err := existsColumn(ctx, client, col.table, col.name)
		if err != nil {
			if l != nil {
				l.Warning("Failed to inspect %s.%s: %s", col.table, col.name, err)
			}
			continue
		}
		if exists {
			continue
		}

		var stmt string
		switch drv.Dialect() {
		case dialect.MySQL:
			stmt = "ALTER TABLE `" + col.table + "` ADD COLUMN `" + col.name + "` " + col.mysql
		case dialect.Postgres:
			stmt = `ALTER TABLE "` + col.table + `" ADD COLUMN "` + col.name + `" ` + col.pg
		default:
			stmt = `ALTER TABLE "` + col.table + `" ADD COLUMN "` + col.name + `" ` + col.other
		}

		if err := ExecuteStatements(ctx, client, l, []string{stmt}); err != nil {
			if l != nil {
				l.Warning("Failed to add column %s.%s: %s", col.table, col.name, err)
			}
		}
	}

	// Databases migrated from Pro carry the price in price_points while the
	// This schema reads price. Backfill once so migrated paid shares keep their
	// price instead of silently becoming free.
	if err := backfillSharePrice(ctx, client, l); err != nil && l != nil {
		l.Warning("Failed to backfill share price: %s", err)
	}

	return nil
}

// backfillSharePrice copies price_points into the price column for
// rows that have not been touched yet. It is a no-op when price_points is
// absent or already mirrored.
func backfillSharePrice(ctx context.Context, client *ent.Client, l logging.Logger) error {
	drv := client.Driver()
	if drv == nil {
		return nil
	}

	hasPrice, err := existsColumn(ctx, client, "shares", "price")
	if err != nil || !hasPrice {
		return err
	}
	hasPoints, err := existsColumn(ctx, client, "shares", "price_points")
	if err != nil || !hasPoints {
		return err
	}

	return ExecuteStatements(ctx, client, l, []string{
		`UPDATE shares SET price = price_points WHERE COALESCE(price, 0) = 0 AND COALESCE(price_points, 0) > 0`,
	})
}

// existsColumn reports whether table already has the given column. Unknown
// dialects fall back to "assume present" so the ALTER is simply skipped.
func existsColumn(ctx context.Context, client *ent.Client, table, column string) (bool, error) {
	drv := client.Driver()
	if drv == nil {
		return true, nil
	}

	var (
		query string
		args  = []any{column}
	)
	switch drv.Dialect() {
	case dialect.MySQL:
		query = "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?"
		args = []any{table, column}
	case dialect.Postgres:
		query = "SELECT COUNT(*) FROM information_schema.columns WHERE table_name = $1 AND column_name = $2"
		args = []any{table, column}
	default:
		query = "SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?"
		args = []any{table, column}
	}

	rows := new(entsql.Rows)
	if err := drv.Query(ctx, query, args, rows); err != nil {
		return false, err
	}
	defer rows.Close()

	if !rows.Next() {
		return false, rows.Err()
	}

	var count int
	if err := rows.Scan(&count); err != nil {
		return false, err
	}

	return count > 0, rows.Err()
}
