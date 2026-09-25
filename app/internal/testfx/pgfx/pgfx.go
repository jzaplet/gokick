// Package pgfx provisions throwaway Postgres databases for tests. Each test gets
// its own database, cloned from a template that holds the migrated schema — a clone
// takes milliseconds, so every test starts from a pristine, fully migrated
// database without paying for the migrations again, and nothing leaks between
// tests. The database is dropped when the test ends.
//
// The cluster comes from APP_TEST_DB_URL: a superuser DSN (it creates and drops
// databases) of a cluster bootstrapped by docker/postgres/initdb/01-roles.sh with
// its default passwords — the db-test compose service is exactly that, and
// `make test-pg` starts it and sets the variable. The package reads the process
// environment only, never .env.
package pgfx

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"gokick/app/infrastructure/config"
	"gokick/app/infrastructure/database"
	"gokick/app/infrastructure/postgres"
	"gokick/migrations"

	"github.com/jackc/pgx/v5"
)

// EnvTestDBURL names the variable holding the test cluster's superuser DSN.
const EnvTestDBURL = "APP_TEST_DB_URL"

// The roles of the test cluster, with the default passwords of
// docker/postgres/initdb/01-roles.sh (the password is the role name).
const (
	RoleOwner  = "gokick_owner"
	RoleApp    = "gokick_app"
	RoleSystem = "gokick_system"
)

// templateLockKey is the advisory lock that serializes template preparation
// across the test binaries `go test` runs in parallel (an arbitrary constant).
const templateLockKey = 0x676f6b69636b // "gokick"

// DB is one test's database and the DSNs of each role on it.
type DB struct {
	// AdminURL connects as the cluster superuser — for tests that need a role the
	// application must refuse.
	AdminURL  string
	OwnerURL  string
	AppURL    string
	SystemURL string
}

// Config is a config.Config wired to the database the way production is: the
// three role DSNs and the default session limits. Adjust fields as the test needs.
func (d *DB) Config() *config.Config {
	return &config.Config{
		DBDriver:           database.DriverPostgres,
		DBURL:              d.AppURL,
		DBSystemURL:        d.SystemURL,
		DBMigrateURL:       d.OwnerURL,
		DBLockTimeout:      5 * time.Second,
		DBStatementTimeout: 30 * time.Second,
		DBIdleTxTimeout:    time.Minute,
	}
}

// New returns a fresh database holding the migrated schema, dropped when the
// test ends.
func New(t *testing.T) *DB {
	t.Helper()
	tpl, err := template()
	if err != nil {
		t.Fatalf("pgfx: prepare the template database: %v", err)
	}
	return create(t, "TEMPLATE "+ident(tpl)+" STRATEGY FILE_COPY")
}

// NewEmpty returns a fresh database with no schema at all, owned by the schema
// owner — for tests of the migrations themselves.
func NewEmpty(t *testing.T) *DB {
	t.Helper()
	return create(t, "TEMPLATE template0")
}

func create(t *testing.T, source string) *DB {
	t.Helper()
	a, err := adminDB()
	if err != nil {
		t.Fatalf("pgfx: %v", err)
	}
	admin, adminURL := a.db, a.url
	name := "gokick_t_" + randomHex(8)
	ctx := context.Background()
	if _, err := admin.ExecContext(ctx,
		"CREATE DATABASE "+ident(name)+" "+source+" OWNER "+ident(RoleOwner)); err != nil {
		t.Fatalf("pgfx: create database: %v", err)
	}
	t.Cleanup(func() {
		// FORCE closes whatever the test left connected (a pool it did not close).
		if _, err := admin.ExecContext(context.Background(),
			"DROP DATABASE IF EXISTS "+ident(name)+" WITH (FORCE)"); err != nil {
			t.Errorf("pgfx: drop database %s: %v", name, err)
		}
	})
	return &DB{
		AdminURL:  withDatabase(adminURL, name, nil),
		OwnerURL:  withDatabase(adminURL, name, url.UserPassword(RoleOwner, RoleOwner)),
		AppURL:    withDatabase(adminURL, name, url.UserPassword(RoleApp, RoleApp)),
		SystemURL: withDatabase(adminURL, name, url.UserPassword(RoleSystem, RoleSystem)),
	}
}

// adminConn is the superuser pool and the URL it was opened from.
type adminConn struct {
	db  *sql.DB
	url *url.URL
}

// adminDB opens the superuser pool once per test binary.
var adminDB = sync.OnceValues(func() (*adminConn, error) {
	raw := os.Getenv(EnvTestDBURL)
	if raw == "" {
		return nil, fmt.Errorf("%s is not set — `make test-pg` starts the db-test "+
			"container and sets it; to use another cluster, point it at a superuser "+
			"postgres://user:password@host:port/postgres URL", EnvTestDBURL)
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return nil, fmt.Errorf("%s must be a postgres:// URL", EnvTestDBURL)
	}
	db, err := sql.Open(postgres.DriverName, raw)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(context.Background()); err != nil {
		return nil, fmt.Errorf("connect to %s: %w", EnvTestDBURL, err)
	}
	return &adminConn{db: db, url: u}, nil
})

// template returns the name of the migrated template database, preparing it on
// first use. The name carries a hash of the migration set, so changing a
// migration yields a new template instead of reusing a stale schema.
var template = sync.OnceValues(func() (string, error) {
	a, err := adminDB()
	if err != nil {
		return "", err
	}
	hash, err := migrationsHash()
	if err != nil {
		return "", err
	}
	name := "gokick_tpl_" + hash[:16]

	ctx := context.Background()
	conn, err := a.db.Conn(ctx) // a session advisory lock needs one pinned connection
	if err != nil {
		return "", err
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", templateLockKey); err != nil {
		return "", err
	}
	defer func() {
		_, _ = conn.ExecContext(
			context.Background(),
			"SELECT pg_advisory_unlock($1)",
			templateLockKey,
		)
	}()

	var exists bool
	if err := conn.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", name).Scan(&exists); err != nil {
		return "", err
	}
	if exists {
		return name, nil
	}
	// Build under a temporary name and rename when done: a run that dies halfway
	// leaves a *_build database, which the next run drops, never a half-migrated
	// template.
	build := name + "_build"
	for _, stmt := range []string{
		"DROP DATABASE IF EXISTS " + ident(build) + " WITH (FORCE)",
		"CREATE DATABASE " + ident(build) + " TEMPLATE template0 OWNER " + ident(RoleOwner),
	} {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			return "", err
		}
	}
	ownerURL := withDatabase(a.url, build, url.UserPassword(RoleOwner, RoleOwner))
	if err := migrate(ctx, ownerURL); err != nil {
		return "", fmt.Errorf("migrate the template: %w", err)
	}
	if _, err := conn.ExecContext(ctx,
		"ALTER DATABASE "+ident(build)+" RENAME TO "+ident(name)); err != nil {
		return "", err
	}
	return name, nil
})

// migrate applies the Postgres migration set as the schema owner.
func migrate(ctx context.Context, ownerURL string) error {
	db, err := postgres.OpenOwner(ownerURL)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	provider, err := postgres.NewMigrationProvider(db)
	if err != nil {
		return err
	}
	_, err = provider.Up(ctx)
	return err
}

// migrationsHash fingerprints the Postgres migration set (names and contents).
func migrationsHash() (string, error) {
	names, err := fs.Glob(migrations.Postgres, "*.sql")
	if err != nil {
		return "", err
	}
	sort.Strings(names)
	h := sha256.New()
	for _, n := range names {
		b, err := fs.ReadFile(migrations.Postgres, n)
		if err != nil {
			return "", err
		}
		_, _ = fmt.Fprintf(h, "%s\x00%d\x00", n, len(b)) // a hash.Hash write never fails
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// withDatabase returns base pointed at database name, as user (nil keeps base's).
func withDatabase(base *url.URL, name string, user *url.Userinfo) string {
	u := *base
	u.Path = "/" + name
	u.RawPath = ""
	if user != nil {
		u.User = user
	}
	return u.String()
}

func ident(name string) string { return pgx.Identifier{name}.Sanitize() }

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
