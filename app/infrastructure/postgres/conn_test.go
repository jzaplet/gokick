package postgres_test

import (
	"context"
	"strings"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/infrastructure/postgres"
)

// countUsers counts users through c with no WHERE at all — whatever the count
// shows is what the connection's role and scope let it see.
func countUsers(t *testing.T, ctx context.Context, c postgres.Conn) int {
	t.Helper()
	var n int
	if err := c.GetContext(ctx, &n, `SELECT COUNT(*) FROM users`); err != nil {
		t.Fatalf("count users: %v", err)
	}
	return n
}

// Outside any transaction, a tenant-plane statement runs in a transaction of its
// own scoped to ctx's tenant — the path of run and event handlers. Without it,
// row-level security would show the statement nothing at all.
func TestBaseRepository_Conn_ScopesAStatementOutsideATransaction(t *testing.T) {
	_, mgr := migrated(t)
	w := seedWorld(t, mgr)
	repo := postgres.BaseRepository{DB: mgr}

	for _, tenantID := range []string{w.tenantA, w.tenantB} {
		ctx := tenantCtx(tenantID)
		if n := countUsers(t, ctx, repo.Conn(ctx)); n != 1 {
			t.Fatalf("tenant %s sees %d users, want its own 1", tenantID, n)
		}
	}
	// The cross-tenant planes read on the system role.
	for _, plane := range []shared.Plane{shared.PlanePlatform, shared.PlaneSystem} {
		ctx := shared.ContextWithPlane(context.Background(), plane)
		if n := countUsers(t, ctx, repo.Conn(ctx)); n != 2 {
			t.Fatalf("%s plane sees %d users, want both", plane, n)
		}
	}
}

// Under multitenancy a tenant-plane statement without a tenant fails — it never
// falls back to some tenant, and never runs unscoped.
func TestBaseRepository_Conn_FailsClosedWithoutATenant(t *testing.T) {
	db, _ := migrated(t)
	cfg := db.Config()
	cfg.Multitenancy = true
	repo := postgres.BaseRepository{DB: newManager(t, cfg)}

	var n int
	ctx := context.Background()
	if err := repo.Conn(ctx).GetContext(ctx, &n, `SELECT COUNT(*) FROM users`); err == nil {
		t.Fatal("a tenant-plane statement without a tenant must fail under multitenancy")
	}
}

// SystemConn joins a cross-tenant transaction (its writes commit or roll back
// with it) but never a tenant one: inside a tenant transaction it runs on the
// system pool, outside the transaction's scope.
func TestBaseRepository_SystemConn_JoinsOnlyACrossTenantTransaction(t *testing.T) {
	_, mgr := migrated(t)
	w := seedWorld(t, mgr)
	repo := postgres.BaseRepository{DB: mgr}
	const insertUser = `INSERT INTO users (id, nickname, password_hash, tenant_id)
		VALUES ($1, $2, 'h', $3)`

	// In a system transaction: its own uncommitted row is visible through SystemConn.
	sysCtx, err := mgr.BeginTx(shared.ContextWithPlane(context.Background(), shared.PlaneSystem))
	if err != nil {
		t.Fatalf("begin system tx: %v", err)
	}
	if _, err := repo.Conn(sysCtx).ExecContext(sysCtx, insertUser, newID(), "carol", w.tenantA); err != nil {
		t.Fatalf("insert in the system tx: %v", err)
	}
	if n := countUsers(t, sysCtx, repo.SystemConn(sysCtx)); n != 3 {
		t.Fatalf("SystemConn in a system tx sees %d users, want 3 (it must join the tx)", n)
	}
	if err := mgr.Rollback(sysCtx); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	// In a tenant transaction: SystemConn reads the other tenant's committed row
	// (bob), and not the tenant transaction's uncommitted one (dave).
	tenantTx, err := mgr.BeginTx(tenantCtx(w.tenantA))
	if err != nil {
		t.Fatalf("begin tenant tx: %v", err)
	}
	defer func() { _ = mgr.Rollback(tenantTx) }()
	if _, err := repo.Conn(tenantTx).ExecContext(tenantTx, insertUser, newID(), "dave", w.tenantA); err != nil {
		t.Fatalf("insert in the tenant tx: %v", err)
	}
	var seen []string
	if err := repo.SystemConn(tenantTx).SelectContext(tenantTx, &seen,
		`SELECT nickname FROM users ORDER BY nickname`); err != nil {
		t.Fatalf("read through SystemConn: %v", err)
	}
	if strings.Join(seen, ",") != "alice,bob" {
		t.Fatalf("SystemConn in a tenant tx sees %v, want the committed alice,bob — "+
			"it must not join the tenant transaction", seen)
	}
}
