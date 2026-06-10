package token_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"gokick/app/internal/testfx"
)

// The refresh_tokens schema declares user_id as NOT NULL and token_hash as
// UNIQUE (claim infra-db-security-11). Those are data-integrity guarantees,
// not cosmetics: a NULL user_id would orphan a session from any user, and a
// duplicate token_hash would let two sessions collide on the same lookup key
// (FindByHash keys on token_hash). This test pins both constraints
// BEHAVIOURALLY against the real migrated refresh_tokens table — a direct
// INSERT that violates either must be rejected by SQLite.
//
// Why behavioural (raw INSERT) rather than scraping PRAGMA table_info: a
// PRAGMA string-match would merely restate the DDL (a change-detector). An
// INSERT that the engine actually refuses proves the constraint is enforced,
// and falsifies the exact mutation that would weaken it:
//   - drop NOT NULL on user_id  -> the NULL-user_id INSERT would succeed.
//   - drop UNIQUE on token_hash -> the duplicate-hash INSERT would succeed.
//
// The cascade half of infra-db-security-11 (and infra-db-security-13) is
// already pinned by TestSqliteManager_RefreshTokensCascadeOnUserDelete in
// app/infrastructure/database; it is deliberately not duplicated here.
func TestRefreshTokensSchema_EnforcesUserIDNotNullAndHashUnique(t *testing.T) {
	ctx := context.Background()
	fx := testfx.New(t, filepath.Join(t.TempDir(), "refresh_tokens_constraints.db"))
	u := fx.SeedUser(t, "alice", "pwd", "user")

	const insert = `INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?)`
	future := time.Now().Add(time.Hour)

	// Baseline: a well-formed row inserts cleanly. This guards the negative
	// cases below from passing for the wrong reason — if the whole INSERT were
	// broken (bad column, missing table), the "rejected" assertions would be
	// satisfied spuriously. A green baseline proves the only thing failing the
	// negative cases is the constraint under test.
	if _, err := fx.DB.DB().ExecContext(ctx, insert,
		uuid.New().String(), u.ID, "baseline-hash", future, time.Now()); err != nil {
		t.Fatalf("baseline well-formed insert must succeed: %v", err)
	}

	tests := []struct {
		name      string
		userID    any    // any so we can pass a real nil for the NULL case
		tokenHash string // "baseline-hash" re-used triggers the UNIQUE clash
		wantErr   string // substring expected (upper-cased) in the constraint error
	}{
		{
			name:      "user_id NULL is rejected (NOT NULL)",
			userID:    nil,
			tokenHash: "null-user-hash",
			wantErr:   "NOT NULL",
		},
		{
			name:      "duplicate token_hash is rejected (UNIQUE)",
			userID:    u.ID,
			tokenHash: "baseline-hash", // same hash as the baseline row above
			wantErr:   "UNIQUE",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := fx.DB.DB().ExecContext(ctx, insert,
				uuid.New().String(), tc.userID, tc.tokenHash, future, time.Now())
			if err == nil {
				t.Fatalf("expected %s constraint violation, got nil error", tc.wantErr)
			}
			if !strings.Contains(strings.ToUpper(err.Error()), tc.wantErr) {
				t.Fatalf("expected %s constraint error, got: %v", tc.wantErr, err)
			}
		})
	}
}
