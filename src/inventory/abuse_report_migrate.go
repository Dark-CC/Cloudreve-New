package inventory

import (
	"context"
	"database/sql"
	"fmt"

	"entgo.io/ent/dialect"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/pkg/logging"
)

// EnsureAbuseReportSchema creates the abuse_reports table when it is missing.
//
// ent's Schema.Create performs a diff against the current database, which can drop
// tables that are not part of the current schema. To stay additive and safe for
// databases created by other editions, the table is created with an explicit
// CREATE TABLE IF NOT EXISTS statement instead.
func EnsureAbuseReportSchema(ctx context.Context, client *ent.Client, l logging.Logger) error {
	drv := client.Driver()
	if drv == nil {
		return nil
	}

	var statements []string
	switch drv.Dialect() {
	case dialect.MySQL:
		statements = []string{`
CREATE TABLE IF NOT EXISTS ` + "`abuse_reports`" + ` (
  ` + "`id`" + ` bigint NOT NULL AUTO_INCREMENT,
  ` + "`created_at`" + ` datetime NOT NULL,
  ` + "`updated_at`" + ` datetime NOT NULL,
  ` + "`deleted_at`" + ` datetime NULL,
  ` + "`file_uri`" + ` text NULL,
  ` + "`category`" + ` bigint NOT NULL DEFAULT 0,
  ` + "`description`" + ` text NULL,
  ` + "`share_id`" + ` bigint NULL,
  ` + "`reported_user_id`" + ` bigint NULL,
  ` + "`reporter_id`" + ` bigint NULL,
  PRIMARY KEY (` + "`id`" + `),
  CONSTRAINT ` + "`abuse_reports_shares_abuse_reports`" + ` FOREIGN KEY (` + "`share_id`" + `) REFERENCES ` + "`shares`" + ` (` + "`id`" + `) ON DELETE SET NULL,
  CONSTRAINT ` + "`abuse_reports_users_abuse_reports_as_reported`" + ` FOREIGN KEY (` + "`reported_user_id`" + `) REFERENCES ` + "`users`" + ` (` + "`id`" + `) ON DELETE SET NULL,
  CONSTRAINT ` + "`abuse_reports_users_abuse_reports_as_reporter`" + ` FOREIGN KEY (` + "`reporter_id`" + `) REFERENCES ` + "`users`" + ` (` + "`id`" + `) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`}
	case dialect.Postgres:
		statements = []string{`
CREATE TABLE IF NOT EXISTS "abuse_reports" (
  "id" bigserial NOT NULL,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  "deleted_at" timestamptz NULL,
  "file_uri" text NULL,
  "category" bigint NOT NULL DEFAULT 0,
  "description" text NULL,
  "share_id" bigint NULL REFERENCES "shares" ("id") ON DELETE SET NULL,
  "reported_user_id" bigint NULL REFERENCES "users" ("id") ON DELETE SET NULL,
  "reporter_id" bigint NULL REFERENCES "users" ("id") ON DELETE SET NULL,
  PRIMARY KEY ("id")
)`}
	default:
		statements = []string{`
CREATE TABLE IF NOT EXISTS ` + "`abuse_reports`" + ` (
  ` + "`id`" + ` integer NOT NULL PRIMARY KEY AUTOINCREMENT,
  ` + "`created_at`" + ` datetime NOT NULL,
  ` + "`updated_at`" + ` datetime NOT NULL,
  ` + "`deleted_at`" + ` datetime NULL,
  ` + "`file_uri`" + ` text NULL,
  ` + "`category`" + ` integer NOT NULL DEFAULT 0,
  ` + "`description`" + ` text NULL,
  ` + "`share_id`" + ` integer NULL,
  ` + "`reported_user_id`" + ` integer NULL,
  ` + "`reporter_id`" + ` integer NULL,
  CONSTRAINT ` + "`abuse_reports_shares_abuse_reports`" + ` FOREIGN KEY (` + "`share_id`" + `) REFERENCES ` + "`shares`" + ` (` + "`id`" + `) ON DELETE SET NULL,
  CONSTRAINT ` + "`abuse_reports_users_abuse_reports_as_reported`" + ` FOREIGN KEY (` + "`reported_user_id`" + `) REFERENCES ` + "`users`" + ` (` + "`id`" + `) ON DELETE SET NULL,
  CONSTRAINT ` + "`abuse_reports_users_abuse_reports_as_reporter`" + ` FOREIGN KEY (` + "`reporter_id`" + `) REFERENCES ` + "`users`" + ` (` + "`id`" + `) ON DELETE SET NULL
)`}
	}

	for _, stmt := range statements {
		if err := drv.Exec(ctx, stmt, []any{}, new(sql.Result)); err != nil {
			if l != nil {
				l.Warning("Failed to ensure abuse_reports schema: %s", err)
			}
			return fmt.Errorf("failed to ensure abuse_reports schema: %w", err)
		}
	}

	return nil
}

// EnsureAuditLogSchema creates the audit_logs table when it is missing.
// Same additive-only rationale as EnsureAbuseReportSchema.
func EnsureAuditLogSchema(ctx context.Context, client *ent.Client, l logging.Logger) error {
	drv := client.Driver()
	if drv == nil {
		return nil
	}

	var statements []string
	switch drv.Dialect() {
	case dialect.MySQL:
		statements = []string{`
CREATE TABLE IF NOT EXISTS ` + "`audit_logs`" + ` (
  ` + "`id`" + ` bigint NOT NULL AUTO_INCREMENT,
  ` + "`created_at`" + ` datetime NOT NULL,
  ` + "`updated_at`" + ` datetime NOT NULL,
  ` + "`deleted_at`" + ` datetime NULL,
  ` + "`type`" + ` bigint NOT NULL,
  ` + "`correlation_id`" + ` char(36) NULL,
  ` + "`ip`" + ` text NULL,
  ` + "`content`" + ` json NULL,
  ` + "`entity_audit_logs`" + ` bigint NULL,
  ` + "`file_audit_logs`" + ` bigint NULL,
  ` + "`share_audit_logs`" + ` bigint NULL,
  ` + "`user_audit_logs`" + ` bigint NULL,
  PRIMARY KEY (` + "`id`" + `),
  CONSTRAINT ` + "`audit_logs_entities_audit_logs`" + ` FOREIGN KEY (` + "`entity_audit_logs`" + `) REFERENCES ` + "`entities`" + ` (` + "`id`" + `) ON DELETE SET NULL,
  CONSTRAINT ` + "`audit_logs_files_audit_logs`" + ` FOREIGN KEY (` + "`file_audit_logs`" + `) REFERENCES ` + "`files`" + ` (` + "`id`" + `) ON DELETE SET NULL,
  CONSTRAINT ` + "`audit_logs_shares_audit_logs`" + ` FOREIGN KEY (` + "`share_audit_logs`" + `) REFERENCES ` + "`shares`" + ` (` + "`id`" + `) ON DELETE SET NULL,
  CONSTRAINT ` + "`audit_logs_users_audit_logs`" + ` FOREIGN KEY (` + "`user_audit_logs`" + `) REFERENCES ` + "`users`" + ` (` + "`id`" + `) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`}
	case dialect.Postgres:
		statements = []string{`
CREATE TABLE IF NOT EXISTS "audit_logs" (
  "id" bigserial NOT NULL,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  "deleted_at" timestamptz NULL,
  "type" bigint NOT NULL,
  "correlation_id" uuid NULL,
  "ip" text NULL,
  "content" jsonb NULL,
  "entity_audit_logs" bigint NULL REFERENCES "entities" ("id") ON DELETE SET NULL,
  "file_audit_logs" bigint NULL REFERENCES "files" ("id") ON DELETE SET NULL,
  "share_audit_logs" bigint NULL REFERENCES "shares" ("id") ON DELETE SET NULL,
  "user_audit_logs" bigint NULL REFERENCES "users" ("id") ON DELETE SET NULL,
  PRIMARY KEY ("id")
)`}
	default:
		statements = []string{`
CREATE TABLE IF NOT EXISTS ` + "`audit_logs`" + ` (
  ` + "`id`" + ` integer NOT NULL PRIMARY KEY AUTOINCREMENT,
  ` + "`created_at`" + ` datetime NOT NULL,
  ` + "`updated_at`" + ` datetime NOT NULL,
  ` + "`deleted_at`" + ` datetime NULL,
  ` + "`type`" + ` integer NOT NULL,
  ` + "`correlation_id`" + ` uuid NULL,
  ` + "`ip`" + ` text NULL,
  ` + "`content`" + ` json NULL,
  ` + "`entity_audit_logs`" + ` integer NULL,
  ` + "`file_audit_logs`" + ` integer NULL,
  ` + "`share_audit_logs`" + ` integer NULL,
  ` + "`user_audit_logs`" + ` integer NULL,
  CONSTRAINT ` + "`audit_logs_entities_audit_logs`" + ` FOREIGN KEY (` + "`entity_audit_logs`" + `) REFERENCES ` + "`entities`" + ` (` + "`id`" + `) ON DELETE SET NULL,
  CONSTRAINT ` + "`audit_logs_files_audit_logs`" + ` FOREIGN KEY (` + "`file_audit_logs`" + `) REFERENCES ` + "`files`" + ` (` + "`id`" + `) ON DELETE SET NULL,
  CONSTRAINT ` + "`audit_logs_shares_audit_logs`" + ` FOREIGN KEY (` + "`share_audit_logs`" + `) REFERENCES ` + "`shares`" + ` (` + "`id`" + `) ON DELETE SET NULL,
  CONSTRAINT ` + "`audit_logs_users_audit_logs`" + ` FOREIGN KEY (` + "`user_audit_logs`" + `) REFERENCES ` + "`users`" + ` (` + "`id`" + `) ON DELETE SET NULL
)`}
	}

	for _, stmt := range statements {
		if err := drv.Exec(ctx, stmt, []any{}, new(sql.Result)); err != nil {
			if l != nil {
				l.Warning("Failed to ensure audit_logs schema: %s", err)
			}
			return fmt.Errorf("failed to ensure audit_logs schema: %w", err)
		}
	}

	return nil
}

// EnsureExternalIdentitySchema creates the external_identities table when it is
// missing. The table stores third-party sign-in bindings (OIDC/Logto/QQ).
// Same additive-only rationale as EnsureAbuseReportSchema.
func EnsureExternalIdentitySchema(ctx context.Context, client *ent.Client, l logging.Logger) error {
	drv := client.Driver()
	if drv == nil {
		return nil
	}

	var statements []string
	switch drv.Dialect() {
	case dialect.MySQL:
		statements = []string{`
CREATE TABLE IF NOT EXISTS ` + "`external_identities`" + ` (
  ` + "`id`" + ` bigint NOT NULL AUTO_INCREMENT,
  ` + "`created_at`" + ` datetime NOT NULL,
  ` + "`updated_at`" + ` datetime NOT NULL,
  ` + "`provider`" + ` varchar(64) NOT NULL,
  ` + "`subject`" + ` varchar(255) NOT NULL,
  ` + "`email`" + ` varchar(255) NULL,
  ` + "`display_name`" + ` varchar(255) NULL,
  ` + "`avatar`" + ` text NULL,
  ` + "`user_id`" + ` bigint NOT NULL,
  PRIMARY KEY (` + "`id`" + `),
  UNIQUE KEY ` + "`external_identities_provider_subject`" + ` (` + "`provider`" + `, ` + "`subject`" + `),
  UNIQUE KEY ` + "`external_identities_user_provider`" + ` (` + "`user_id`" + `, ` + "`provider`" + `),
  CONSTRAINT ` + "`external_identities_users_external_identities`" + ` FOREIGN KEY (` + "`user_id`" + `) REFERENCES ` + "`users`" + ` (` + "`id`" + `) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`}
	case dialect.Postgres:
		statements = []string{`
CREATE TABLE IF NOT EXISTS "external_identities" (
  "id" bigserial NOT NULL,
  "created_at" timestamptz NOT NULL,
  "updated_at" timestamptz NOT NULL,
  "provider" varchar(64) NOT NULL,
  "subject" varchar(255) NOT NULL,
  "email" varchar(255) NULL,
  "display_name" varchar(255) NULL,
  "avatar" text NULL,
  "user_id" bigint NOT NULL REFERENCES "users" ("id") ON DELETE CASCADE,
  PRIMARY KEY ("id"),
  UNIQUE ("provider", "subject"),
  UNIQUE ("user_id", "provider")
)`}
	default:
		statements = []string{`
CREATE TABLE IF NOT EXISTS ` + "`external_identities`" + ` (
  ` + "`id`" + ` integer NOT NULL PRIMARY KEY AUTOINCREMENT,
  ` + "`created_at`" + ` datetime NOT NULL,
  ` + "`updated_at`" + ` datetime NOT NULL,
  ` + "`provider`" + ` varchar(64) NOT NULL,
  ` + "`subject`" + ` varchar(255) NOT NULL,
  ` + "`email`" + ` varchar(255) NULL,
  ` + "`display_name`" + ` varchar(255) NULL,
  ` + "`avatar`" + ` text NULL,
  ` + "`user_id`" + ` integer NOT NULL,
  UNIQUE (` + "`provider`" + `, ` + "`subject`" + `),
  UNIQUE (` + "`user_id`" + `, ` + "`provider`" + `),
  CONSTRAINT ` + "`external_identities_users_external_identities`" + ` FOREIGN KEY (` + "`user_id`" + `) REFERENCES ` + "`users`" + ` (` + "`id`" + `) ON DELETE CASCADE
)`}
	}

	for _, stmt := range statements {
		if err := drv.Exec(ctx, stmt, []any{}, new(sql.Result)); err != nil {
			if l != nil {
				l.Warning("Failed to ensure external_identities schema: %s", err)
			}
			return fmt.Errorf("failed to ensure external_identities schema: %w", err)
		}
	}

	return nil
}
