package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"gokick/app/domain/shared"
	"gokick/app/infrastructure/database"
	"gokick/app/infrastructure/postgres"
	"gokick/app/internal/testfx/pgfx"

	"github.com/jmoiron/sqlx"
)

// Every pooled connection starts with the session settings the adapter relies on:
// UTC, the application name, and the configured lock/statement/idle limits.
func TestManager_SessionSettings(t *testing.T) {
	db, _ := migrated(t)
	cfg := db.Config()
	cfg.DBLockTimeout = 1500 * time.Millisecond
	cfg.DBStatementTimeout = 0 // disabled
	mgr := newManager(t, cfg)

	for _, pool := range []*sqlx.DB{mgr.App(), mgr.System()} {
		for setting, want := range map[string]string{
			"TimeZone":                            "UTC",
			"application_name":                    "gokick",
			"lock_timeout":                        "1500ms",
			"statement_timeout":                   "0",
			"idle_in_transaction_session_timeout": "1min",
		} {
			var got string
			if err := pool.Get(&got, "SELECT current_setting($1)", setting); err != nil {
				t.Fatalf("read %s: %v", setting, err)
			}
			if got != want {
				t.Errorf("%s = %q, want %q", setting, got, want)
			}
		}
	}
}

// The plane picks the role: the tenant plane runs as gokick_app, the platform and
// system planes as gokick_system.
func TestManager_BeginTx_PlanePicksTheRole(t *testing.T) {
	_, mgr := migrated(t)
	for plane, want := range map[shared.Plane]string{
		shared.PlaneTenant:   "gokick_app",
		shared.PlanePlatform: "gokick_system",
		shared.PlaneSystem:   "gokick_system",
	} {
		ctx := shared.ContextWithPlane(context.Background(), plane)
		inTx(t, mgr, ctx, func(tx *sqlx.Tx) {
			var role string
			if err := tx.Get(&role, "SELECT current_user"); err != nil {
				t.Fatal(err)
			}
			if role != want {
				t.Errorf("%s plane runs as %q, want %q", plane, role, want)
			}
		})
	}
}

// A tenant-plane transaction is scoped to the tenant in ctx — and only for its own
// lifetime: the next transaction on the same pooled connection starts unscoped.
func TestManager_BeginTx_ScopesTheTenantPerTransaction(t *testing.T) {
	db, _ := migrated(t)
	cfg := db.Config()
	cfg.DBMaxConns = 1 // one connection: every transaction below reuses it
	mgr := newManager(t, cfg)

	tenant := newID()
	ctx := shared.ContextWithTenantID(context.Background(), tenant)
	txCtx, err := mgr.BeginTx(ctx)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	var scoped string
	if err := database.TxFromContext(txCtx).Get(&scoped,
		"SELECT current_setting('app.tenant_id')"); err != nil {
		t.Fatal(err)
	}
	if scoped != tenant {
		t.Fatalf("app.tenant_id = %q, want %q", scoped, tenant)
	}
	if err := mgr.Commit(txCtx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	var after string
	if err := mgr.App().Get(&after,
		"SELECT coalesce(current_setting('app.tenant_id', true), '')"); err != nil {
		t.Fatal(err)
	}
	if after != "" {
		t.Fatalf("app.tenant_id leaked past the transaction on the pooled connection: %q", after)
	}
}

// Without a tenant in ctx the tenant plane scopes to the default tenant in
// single-tenant mode, and refuses to open at all under multitenancy.
func TestManager_BeginTx_MissingTenant(t *testing.T) {
	db, mgr := migrated(t)
	inTx(t, mgr, context.Background(), func(tx *sqlx.Tx) {
		var scoped string
		if err := tx.Get(&scoped, "SELECT current_setting('app.tenant_id')"); err != nil {
			t.Fatal(err)
		}
		if scoped != shared.DefaultTenantID {
			t.Fatalf("single-tenant scope = %q, want the default tenant", scoped)
		}
	})

	cfg := db.Config()
	cfg.Multitenancy = true
	multi := newManager(t, cfg)
	if _, err := multi.BeginTx(context.Background()); err == nil {
		t.Fatal("a tenant-plane transaction without a tenant must fail closed under multitenancy")
	}
	if _, _, err := multi.BeginReadTx(context.Background()); err == nil {
		t.Fatal("a tenant-plane read without a tenant must fail closed under multitenancy")
	}
	// The cross-tenant planes need no tenant.
	sys := shared.ContextWithPlane(context.Background(), shared.PlaneSystem)
	inTx(t, multi, sys, func(*sqlx.Tx) {})
}

// A durable run handler's no-transaction zone holds on Postgres too.
func TestManager_BeginTx_RefusesTheNoTxZone(t *testing.T) {
	_, mgr := migrated(t)
	if _, err := mgr.BeginTx(shared.ContextForbidTx(context.Background())); err == nil {
		t.Fatal("BeginTx must fail in a no-transaction zone")
	}
}

// A read transaction is READ ONLY, carries the tenant scope, and hands its
// connection back when it ends.
func TestManager_BeginReadTx(t *testing.T) {
	db, _ := migrated(t)
	cfg := db.Config()
	cfg.DBMaxConns = 1 // a read transaction that never ended would starve the next one
	mgr := newManager(t, cfg)

	tenant := newID()
	ctx := shared.ContextWithTenantID(context.Background(), tenant)
	for range 3 {
		txCtx, end, err := mgr.BeginReadTx(ctx)
		if err != nil {
			t.Fatalf("BeginReadTx: %v", err)
		}
		tx := database.TxFromContext(txCtx)
		if tx == nil {
			t.Fatal("BeginReadTx must put its transaction in ctx")
		}
		var scoped string
		if err := tx.Get(&scoped, "SELECT current_setting('app.tenant_id')"); err != nil {
			t.Fatal(err)
		}
		if scoped != tenant {
			t.Fatalf("read scope = %q, want %q", scoped, tenant)
		}
		_, err = tx.Exec(`UPDATE users SET email = 'x' WHERE false`)
		if sqlState(err) != codeReadOnlyTransaction {
			t.Fatalf(
				"a write in a read transaction: got %v, want SQLSTATE %s",
				err,
				codeReadOnlyTransaction,
			)
		}
		end()
	}
}

// A query dispatched inside a command joins the command's transaction instead of
// opening a second one.
func TestManager_BeginReadTx_JoinsAnOpenTransaction(t *testing.T) {
	_, mgr := migrated(t)
	txCtx, err := mgr.BeginTx(context.Background())
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer func() { _ = mgr.Rollback(txCtx) }()
	readCtx, end, err := mgr.BeginReadTx(txCtx)
	if err != nil {
		t.Fatalf("BeginReadTx: %v", err)
	}
	end()
	if database.TxFromContext(readCtx) != database.TxFromContext(txCtx) {
		t.Fatal("BeginReadTx inside a transaction must join it")
	}
	// Ending the joined read left the outer transaction usable.
	if _, err := database.TxFromContext(txCtx).Exec("SELECT 1"); err != nil {
		t.Fatalf("outer transaction after the joined read: %v", err)
	}
}

func TestManager_CommitWithoutTransaction(t *testing.T) {
	_, mgr := migrated(t)
	if err := mgr.Commit(context.Background()); err == nil {
		t.Fatal("Commit without a transaction in ctx must fail")
	}
	if err := mgr.Rollback(context.Background()); err == nil {
		t.Fatal("Rollback without a transaction in ctx must fail")
	}
}

// A malformed DSN fails NewManager naming the variable, never echoing the DSN.
func TestNewManager_InvalidDSN(t *testing.T) {
	db := pgfx.New(t)
	cfg := db.Config()
	cfg.DBSystemURL = "postgres://u:secret-pw@host:notaport/db"
	_, err := postgres.NewManager(cfg)
	if err == nil {
		t.Fatal("NewManager must reject a malformed DSN")
	}
	if !strings.Contains(err.Error(), "APP_DB_SYSTEM_URL") ||
		strings.Contains(err.Error(), "secret-pw") {
		t.Fatalf("error must name the variable and hide the DSN: %v", err)
	}
}
