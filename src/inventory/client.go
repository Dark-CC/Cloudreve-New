package inventory

import (
	"context"
	rawsql "database/sql"
	"database/sql/driver"
	"fmt"
	"os"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/cloudreve/Cloudreve/v4/ent"
	_ "github.com/cloudreve/Cloudreve/v4/ent/runtime"
	"github.com/cloudreve/Cloudreve/v4/inventory/debug"
	"github.com/cloudreve/Cloudreve/v4/pkg/cache"
	"github.com/cloudreve/Cloudreve/v4/pkg/conf"
	"github.com/cloudreve/Cloudreve/v4/pkg/logging"
	"github.com/cloudreve/Cloudreve/v4/pkg/util"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	"modernc.org/sqlite"
)

const (
	DBVersionPrefix           = "db_version_"
	EnvDefaultOverwritePrefix = "CR_SETTING_DEFAULT_"
	EnvEnableAria2            = "CR_ENABLE_ARIA2"
	// kvSettingPrefix mirrors setting.KvSettingPrefix. It is duplicated here to
	// avoid an import cycle, since pkg/setting depends on this package.
	kvSettingPrefix = "setting_"
)

// InitializeDBClient runs migration and returns a new ent.Client with additional configurations
// for hooks and interceptors.
func InitializeDBClient(l logging.Logger,
	client *ent.Client, kv cache.Driver, requiredDbVersion string) (*ent.Client, error) {
	ctx := context.WithValue(context.Background(), logging.LoggerCtx{}, l)
	if needMigration(client, ctx, requiredDbVersion) {
		// Run the auto migration tool.
		if err := migrate(l, client, ctx, kv, requiredDbVersion); err != nil {
			return nil, fmt.Errorf("failed to migrate database: %w", err)
		}
	} else {
		l.Info("Database schema is up to date.")
	}

	// Additive schema reconciliation for tables we own. This is idempotent and
	// never drops or alters existing tables, so it is safe to run on every boot.
	if err := EnsureAbuseReportSchema(ctx, client, l); err != nil {
		return nil, err
	}

	if err := EnsureAuditLogSchema(ctx, client, l); err != nil {
		return nil, err
	}

	if err := EnsureExternalIdentitySchema(ctx, client, l); err != nil {
		return nil, err
	}

	if err := EnsureProColumns(ctx, client, l); err != nil {
		return nil, err
	}

	if err := EnsureVASSchema(ctx, client, l); err != nil {
		return nil, err
	}

	if err := MigrateVASProductTypes(ctx, client, l); err != nil {
		return nil, err
	}

	if err := EnsureOpenIDsSchema(ctx, client, l); err != nil {
		return nil, err
	}

	if err := EnsureShareACLSchema(ctx, client, l); err != nil {
		return nil, err
	}

	if err := EnsureGroupPolicySchema(ctx, client, l); err != nil {
		return nil, err
	}

	if err := EnsureDefaultSettings(ctx, client, kv, l); err != nil {
		return nil, err
	}

	if err := EnsureOneyunpanEdgeSettings(ctx, client, kv, l); err != nil {
		return nil, err
	}

	//createMockData(client, ctx)
	return client, nil
}

// EnsureDefaultSettings inserts settings introduced by additive features into
// existing installations. Existing values are never overwritten.
func EnsureDefaultSettings(ctx context.Context, client *ent.Client, kv cache.Driver, l logging.Logger) error {
	existing, err := client.Setting.Query().All(ctx)
	if err != nil {
		return fmt.Errorf("failed to query existing settings: %w", err)
	}

	present := make(map[string]struct{}, len(existing))
	for _, row := range existing {
		present[row.Name] = struct{}{}
	}

	inserted := 0
	for name, value := range DefaultSettings {
		if _, ok := present[name]; ok {
			continue
		}

		if override, ok := os.LookupEnv(EnvDefaultOverwritePrefix + name); ok {
			value = override
		}

		if _, err := client.Setting.Create().SetName(name).SetValue(value).Save(ctx); err != nil {
			return fmt.Errorf("failed to insert default setting %q: %w", name, err)
		}
		inserted++
	}

	if inserted > 0 {
		if l != nil {
			l.Info("Inserted %d missing default setting(s).", inserted)
		}
		// Only drop cached setting values; wiping the whole KV store would also
		// discard session and one-time ticket entries.
		if err := kv.Delete(kvSettingPrefix); err != nil && l != nil {
			l.Warning("Failed to clear setting cache: %s", err)
		}
	}

	return nil
}

// NewRawEntClient returns a new ent.Client without additional configurations.
func NewRawEntClient(l logging.Logger, config conf.ConfigProvider) (*ent.Client, error) {
	l.Info("Initializing database connection...")
	dbConfig := config.Database()
	confDBType := dbConfig.Type
	if confDBType == conf.SQLite3DB || confDBType == "" {
		confDBType = conf.SQLiteDB
	}
	if confDBType == conf.MariaDB {
		confDBType = conf.MySqlDB
	}

	var (
		err    error
		client *sql.Driver
	)

	// Check if the database type is supported.
	if confDBType != conf.SQLiteDB && confDBType != conf.MySqlDB && confDBType != conf.PostgresDB {
		return nil, fmt.Errorf("unsupported database type: %s", confDBType)
	}
	// If Database connection string provided, use it directly.
	if dbConfig.DatabaseURL != "" {
		l.Info("Connect to database with connection string")
		client, err = sql.Open(string(confDBType), dbConfig.DatabaseURL)
	} else {

		switch confDBType {
		case conf.SQLiteDB:
			dbFile := util.RelativePath(dbConfig.DBFile)
			l.Info("Connect to SQLite database %q.", dbFile)
			client, err = sql.Open("sqlite3", util.RelativePath(dbConfig.DBFile))
		case conf.PostgresDB:
			l.Info("Connect to Postgres database %q.", dbConfig.Host)
			client, err = sql.Open("postgres", fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=disable",
				dbConfig.Host,
				dbConfig.User,
				dbConfig.Password,
				dbConfig.Name,
				dbConfig.Port))
		case conf.MySqlDB, conf.MsSqlDB:
			l.Info("Connect to MySQL/SQLServer database %q.", dbConfig.Host)
			var host string
			if dbConfig.UnixSocket {
				host = fmt.Sprintf("unix(%s)",
					dbConfig.Host)
			} else {
				host = fmt.Sprintf("(%s:%d)",
					dbConfig.Host,
					dbConfig.Port)
			}

			client, err = sql.Open(string(confDBType), fmt.Sprintf("%s:%s@%s/%s?charset=%s&parseTime=True&loc=Local",
				dbConfig.User,
				dbConfig.Password,
				host,
				dbConfig.Name,
				dbConfig.Charset))
		default:
			return nil, fmt.Errorf("unsupported database type %q", confDBType)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to open database: %w", err)
		}

	}
	// Set connection pool
	db := client.DB()
	db.SetMaxIdleConns(50)
	if confDBType == "sqlite" || confDBType == "UNSET" {
		db.SetMaxOpenConns(1)
	} else {
		db.SetMaxOpenConns(100)
	}

	// Set timeout
	db.SetConnMaxLifetime(time.Second * 30)

	driverOpt := ent.Driver(client)

	// Enable verbose logging for debug mode.
	if config.System().Debug {
		l.Debug("Debug mode is enabled for DB client.")
		driverOpt = ent.Driver(debug.DebugWithContext(client, func(ctx context.Context, i ...any) {
			logging.FromContext(ctx).Debug(i[0].(string), i[1:]...)
		}))
	}

	return ent.NewClient(driverOpt), nil
}

type sqlite3Driver struct {
	*sqlite.Driver
}

type sqlite3DriverConn interface {
	Exec(string, []driver.Value) (driver.Result, error)
}

func (d sqlite3Driver) Open(name string) (conn driver.Conn, err error) {
	conn, err = d.Driver.Open(name)
	if err != nil {
		return
	}
	_, err = conn.(sqlite3DriverConn).Exec("PRAGMA foreign_keys = ON;", nil)
	if err != nil {
		_ = conn.Close()
	}
	return
}

func init() {
	rawsql.Register("sqlite3", sqlite3Driver{Driver: &sqlite.Driver{}})
}
