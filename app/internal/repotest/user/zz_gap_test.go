package user_test

import (
	"testing"
	"time"

	"gokick/app/domain/shared"
	"gokick/app/internal/testfx"

	"github.com/google/uuid"
)

// Closes infra-db-security-10 at the repository layer: the users-table schema
// guards role CHECK(role IN ('superadmin','admin','user')) and nickname NOT NULL UNIQUE
// actually reject bad rows. Nothing else in the suite asserts these constraints.
// (The other assigned claims for this package — app-events-audit-29, roadmap-71,
// infra-db-security-20 — are already pinned by the survives-rollback tests in
// zz_audit_test.go; app-events-audit-34's 15m lock duration is pinned by the
// application-layer TestLoginHandler_LockDurationIs15Minutes.)

// rawInsertUser inserts a users row straight through the raw pool, bypassing the
// repository and its value-object validation, so the DB-level CHECK / UNIQUE
// constraints are what's under test. Returns the driver error verbatim.
func rawInsertUser(t *testing.T, fx *testfx.Fixture, nickname, role string) error {
	t.Helper()
	const q = `INSERT INTO users (id, nickname, password_hash, email, role, tenant_id, active, created_at, updated_at)
		VALUES (?, ?, 'hash', 'e@example.com', ?, ?, ?, ?, ?)`
	now := time.Now().UTC()
	_, err := fx.RawExec(
		q,
		uuid.NewString(),
		nickname,
		role,
		shared.DefaultTenantID,
		true,
		now,
		now,
	)
	return err
}

// TestUsersTableConstraints pins the users-table schema guards from
// infra-db-security-10: role is CHECK(role IN ('superadmin','admin','user')) and nickname
// is NOT NULL UNIQUE. Removing either constraint from the init migration
// (20260327000001_init_schema.sql) would let these raw inserts succeed
// and fail the test.
func TestUsersTableConstraints(t *testing.T) {
	t.Run("role CHECK rejects an unknown role", func(t *testing.T) {
		fx := testfx.New(t)

		// Sanity: valid roles insert fine through the same path, so a failure
		// below is the CHECK firing, not a broken INSERT. 'superadmin' is asserted
		// accepted at the DB level on purpose — gating who may create one is the
		// application/CLI layer's job, not the schema's.
		if err := rawInsertUser(t, fx, "valid-admin", "admin"); err != nil {
			t.Fatalf("valid role insert should succeed: %v", err)
		}
		if err := rawInsertUser(t, fx, "valid-superadmin", "superadmin"); err != nil {
			t.Fatalf("superadmin must be accepted by the DB-level CHECK: %v", err)
		}

		err := rawInsertUser(t, fx, "villain", "superhero")
		if err == nil {
			t.Fatal("role CHECK must reject role='superhero'")
		}
		if got := fx.Violated(err); got != testfx.Check {
			t.Fatalf("expected a CHECK violation, got %q: %v", got, err)
		}
	})

	t.Run("nickname UNIQUE rejects a duplicate", func(t *testing.T) {
		fx := testfx.New(t)

		if err := rawInsertUser(t, fx, "dup", "user"); err != nil {
			t.Fatalf("first insert should succeed: %v", err)
		}
		err := rawInsertUser(t, fx, "dup", "user")
		if err == nil {
			t.Fatal("nickname UNIQUE must reject a duplicate nickname")
		}
		if got := fx.Violated(err); got != testfx.Unique {
			t.Fatalf("expected a UNIQUE constraint violation, got %q: %v", got, err)
		}
	})
}
