package postgres_test

import (
	"context"
	"testing"
	"time"

	"gokick/app/domain/shared"
	"gokick/app/infrastructure/database"
	"gokick/app/infrastructure/postgres"

	"github.com/jmoiron/sqlx"
)

// The row-level security suite: tenant isolation enforced by the DATABASE, with no
// repository code involved. Every query here is deliberately naive — no
// WHERE tenant_id — because that is the bug the policies exist to contain: a
// forgotten filter must yield the active tenant's rows, never another tenant's.

// world is two tenants, each with a user, a refresh token and a run, seeded on the
// system plane (as every fixture is).
type world struct {
	tenantA, tenantB string
	userA, userB     string
}

func seedWorld(t *testing.T, mgr *postgres.Manager) world {
	t.Helper()
	w := world{tenantA: newID(), tenantB: newID(), userA: newID(), userB: newID()}
	sys := mgr.System()
	for _, s := range []struct {
		q    string
		args []any
	}{
		{`INSERT INTO tenants (id, name) VALUES ($1, 'acme'), ($2, 'globex')`,
			[]any{w.tenantA, w.tenantB}},
		{`INSERT INTO users (id, nickname, password_hash, tenant_id)
		  VALUES ($1, 'alice', 'h', $2), ($3, 'bob', 'h', $4)`,
			[]any{w.userA, w.tenantA, w.userB, w.tenantB}},
		{`INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at)
		  VALUES ($1, $2, 'hash-a', $5), ($3, $4, 'hash-b', $5)`,
			[]any{newID(), w.userA, newID(), w.userB, time.Now().Add(time.Hour)}},
		{`INSERT INTO runs (id, kind, tenant_id, payload, run_at)
		  VALUES ($1, 'k', $2, '\x00', statement_timestamp()),
		         ($3, 'k', $4, '\x00', statement_timestamp())`,
			[]any{newID(), w.tenantA, newID(), w.tenantB}},
	} {
		if _, err := sys.Exec(s.q, s.args...); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return w
}

func tenantCtx(tenantID string) context.Context {
	return shared.ContextWithTenantID(context.Background(), tenantID)
}

func count(t *testing.T, q sqlx.Queryer, query string, args ...any) int {
	t.Helper()
	var n int
	if err := sqlx.Get(q, &n, query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// A query with no WHERE tenant_id sees only the active tenant's rows.
func TestRLS_TenantSeesOnlyItsOwnRows(t *testing.T) {
	_, mgr := migrated(t)
	w := seedWorld(t, mgr)

	inTx(t, mgr, tenantCtx(w.tenantA), func(tx *sqlx.Tx) {
		for table, want := range map[string]int{
			"users": 1, "runs": 1, "refresh_tokens": 1, "tenants": 1,
		} {
			if got := count(t, tx, "SELECT count(*) FROM "+table); got != want {
				t.Errorf("tenant A sees %d rows of %s, want %d", got, table, want)
			}
		}
		var nickname string
		if err := tx.Get(&nickname, "SELECT nickname FROM users"); err != nil {
			t.Fatal(err)
		}
		if nickname != "alice" {
			t.Errorf("tenant A reads user %q, want its own alice", nickname)
		}
		var tenantName string
		if err := tx.Get(&tenantName, "SELECT name FROM tenants"); err != nil {
			t.Fatal(err)
		}
		if tenantName != "acme" {
			t.Errorf("tenant A reads tenant %q, want its own acme", tenantName)
		}
	})
}

// Another tenant's row cannot be changed or deleted by id: the statement matches
// nothing.
func TestRLS_ForeignRowsAreInvisibleToWrites(t *testing.T) {
	_, mgr := migrated(t)
	w := seedWorld(t, mgr)

	inTx(t, mgr, tenantCtx(w.tenantA), func(tx *sqlx.Tx) {
		for _, stmt := range []string{
			`UPDATE users SET email = 'pwned@example.com' WHERE id = $1`,
			`DELETE FROM refresh_tokens WHERE user_id = $1`,
			`DELETE FROM users WHERE id = $1`,
		} {
			res, err := tx.Exec(stmt, w.userB)
			if err != nil {
				t.Fatalf("%s: %v", stmt, err)
			}
			if n, _ := res.RowsAffected(); n != 0 {
				t.Errorf("%s touched %d of tenant B's rows, want 0", stmt, n)
			}
		}
		res, err := tx.Exec(
			`UPDATE runs SET cancel_requested = true WHERE tenant_id = $1`,
			w.tenantB,
		)
		if err != nil {
			t.Fatal(err)
		}
		if n, _ := res.RowsAffected(); n != 0 {
			t.Errorf("tenant A cancelled %d of tenant B's runs, want 0", n)
		}
	})
	// Nothing changed on tenant B's side.
	if got := count(t, mgr.System(),
		`SELECT count(*) FROM users WHERE id = $1 AND email IS NULL`, w.userB); got != 1 {
		t.Fatal("tenant B's user was modified from tenant A")
	}
}

// A row stamped with another tenant cannot be written — neither inserted, nor
// moved there by an update (the policies' WITH CHECK).
func TestRLS_WritesIntoAnotherTenantFail(t *testing.T) {
	_, mgr := migrated(t)
	w := seedWorld(t, mgr)

	for _, tc := range []struct {
		name string
		stmt string
		args []any
	}{
		{"insert a user into B", `INSERT INTO users (id, nickname, password_hash, tenant_id)
			VALUES ($1, 'mallory', 'h', $2)`, []any{newID(), w.tenantB}},
		{"move own user to B", `UPDATE users SET tenant_id = $1 WHERE id = $2`,
			[]any{w.tenantB, w.userA}},
		{"enqueue a run for B", `INSERT INTO runs (id, kind, tenant_id, payload, run_at)
			VALUES ($1, 'k', $2, '\x00', statement_timestamp())`, []any{newID(), w.tenantB}},
		{"mint a token for B's user", `INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at)
			VALUES ($1, $2, 'stolen', statement_timestamp())`, []any{newID(), w.userB}},
	} {
		inTx(t, mgr, tenantCtx(w.tenantA), func(tx *sqlx.Tx) {
			_, err := tx.Exec(tc.stmt, tc.args...)
			if sqlState(err) != codeInsufficientPrivilege {
				t.Errorf("%s: got %v, want a row-level security violation (%s)",
					tc.name, err, codeInsufficientPrivilege)
			}
		})
	}
}

// The tenant plane has no access at all to the tenant registry's writes and to the
// audit log; the system plane may only append to the log and read it back.
func TestRLS_GrantsLimitWhatEachPlaneMayDo(t *testing.T) {
	_, mgr := migrated(t)
	w := seedWorld(t, mgr)

	for _, stmt := range []string{
		`INSERT INTO tenants (id, name) VALUES ('` + newID() + `', 'rogue')`,
		`UPDATE tenants SET name = 'renamed'`,
		`DELETE FROM tenants`,
		`SELECT count(*) FROM audit_log`,
		`INSERT INTO audit_log (id, action) VALUES ('` + newID() + `', 'x')`,
	} {
		inTx(t, mgr, tenantCtx(w.tenantA), func(tx *sqlx.Tx) {
			if _, err := tx.Exec(stmt); sqlState(err) != codeInsufficientPrivilege {
				t.Errorf("tenant plane %q: got %v, want permission denied", stmt, err)
			}
		})
	}

	sys := shared.ContextWithPlane(context.Background(), shared.PlaneSystem)
	inTx(t, mgr, sys, func(tx *sqlx.Tx) {
		if _, err := tx.Exec(`INSERT INTO audit_log (id, action, metadata)
			VALUES ($1, 'auth.login.failed', '{"reason":"test"}')`, newID()); err != nil {
			t.Fatalf("system plane must append to the audit log: %v", err)
		}
		if got := count(t, tx, `SELECT count(*) FROM audit_log`); got != 1 {
			t.Fatalf("system plane reads %d audit rows, want 1", got)
		}
	})
	for _, stmt := range []string{`UPDATE audit_log SET action = 'x'`, `DELETE FROM audit_log`} {
		inTx(t, mgr, sys, func(tx *sqlx.Tx) {
			if _, err := tx.Exec(stmt); sqlState(err) != codeInsufficientPrivilege {
				t.Errorf("system plane %q: got %v — the audit log must be append-only", stmt, err)
			}
		})
	}
}

// The cross-tenant planes see every tenant.
func TestRLS_SystemAndPlatformPlanesSeeEveryTenant(t *testing.T) {
	_, mgr := migrated(t)
	w := seedWorld(t, mgr)
	for _, plane := range []shared.Plane{shared.PlaneSystem, shared.PlanePlatform} {
		ctx := shared.ContextWithPlane(tenantCtx(w.tenantA), plane)
		inTx(t, mgr, ctx, func(tx *sqlx.Tx) {
			if got := count(t, tx, `SELECT count(*) FROM users WHERE tenant_id IN ($1, $2)`,
				w.tenantA, w.tenantB); got != 2 {
				t.Errorf("%s plane sees %d of the 2 users, want both", plane, got)
			}
		})
	}
}

// Outside a scoped transaction the tenant role sees nothing — fail closed, not
// "every tenant".
func TestRLS_UnscopedTenantRoleSeesNothing(t *testing.T) {
	_, mgr := migrated(t)
	seedWorld(t, mgr)
	for _, table := range []string{"users", "runs", "refresh_tokens", "tenants"} {
		if got := count(t, mgr.App(), "SELECT count(*) FROM "+table); got != 0 {
			t.Errorf("the tenant role outside a transaction sees %d rows of %s, want 0", got, table)
		}
	}
}

// A tenant scope naming a tenant with no rows sees no rows.
func TestRLS_UnknownTenantSeesNothing(t *testing.T) {
	_, mgr := migrated(t)
	seedWorld(t, mgr)
	inTx(t, mgr, tenantCtx(newID()), func(tx *sqlx.Tx) {
		if got := count(t, tx, "SELECT count(*) FROM users"); got != 0 {
			t.Fatalf("an unknown tenant sees %d users, want 0", got)
		}
	})
}

// Uniqueness is global, as on SQLite: a nickname taken in another tenant stays
// taken, though the tenant cannot see the row holding it.
func TestRLS_UniquenessSpansTenants(t *testing.T) {
	_, mgr := migrated(t)
	w := seedWorld(t, mgr)
	inTx(t, mgr, tenantCtx(w.tenantA), func(tx *sqlx.Tx) {
		_, err := tx.Exec(`INSERT INTO users (id, nickname, password_hash, tenant_id)
			VALUES ($1, 'bob', 'h', $2)`, newID(), w.tenantA)
		if sqlState(err) != codeUniqueViolation {
			t.Fatalf("taking tenant B's nickname: got %v, want a unique violation", err)
		}
	})
}

// A tenant's own writes go through — the wall is not a wall around everything.
func TestRLS_TenantWritesItsOwnRows(t *testing.T) {
	_, mgr := migrated(t)
	w := seedWorld(t, mgr)
	txCtx, err := mgr.BeginTx(tenantCtx(w.tenantA))
	if err != nil {
		t.Fatal(err)
	}
	tx := database.TxFromContext(txCtx)
	for _, s := range []struct {
		q    string
		args []any
	}{
		{`INSERT INTO users (id, nickname, password_hash, tenant_id) VALUES ($1, 'carol', 'h', $2)`,
			[]any{newID(), w.tenantA}},
		{`UPDATE users SET email = 'alice@example.com' WHERE id = $1`, []any{w.userA}},
		{`INSERT INTO runs (id, kind, tenant_id, payload, run_at)
			VALUES ($1, 'k', $2, '\x00', statement_timestamp())`, []any{newID(), w.tenantA}},
		{`DELETE FROM refresh_tokens WHERE user_id = $1`, []any{w.userA}},
	} {
		if _, err := tx.Exec(s.q, s.args...); err != nil {
			t.Fatalf("%s: %v", s.q, err)
		}
	}
	if err := mgr.Commit(txCtx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if got := count(t, mgr.System(),
		`SELECT count(*) FROM users WHERE tenant_id = $1`, w.tenantA); got != 2 {
		t.Fatalf("tenant A has %d users after its writes, want 2", got)
	}
	if got := count(t, mgr.System(),
		`SELECT count(*) FROM refresh_tokens WHERE user_id = $1`, w.userA); got != 0 {
		t.Fatalf("tenant A's own token survived its revocation: %d left", got)
	}
}
