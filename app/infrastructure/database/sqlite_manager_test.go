package database_test

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gokick/app/infrastructure/config"
	"gokick/app/infrastructure/database"
)

// TestSqliteManager_ConcurrentTxWritesDoNotReturnBusy reproduces the
// "sqlite3: database is locked" regression caused by BEGIN DEFERRED +
// no busy_timeout. The handler pattern read → CPU-hold → write inside
// one transaction must survive a concurrent committed write on a
// sibling connection. Without _txlock=immediate this fails almost
// every run with SQLITE_BUSY_SNAPSHOT during the UPDATE; with
// IMMEDIATE the writers serialize at BEGIN and all goroutines win.
func TestSqliteManager_ConcurrentTxWritesDoNotReturnBusy(t *testing.T) {
	mgr := newTestManager(t)

	if _, err := mgr.DB().Exec(`CREATE TABLE counters (id INTEGER PRIMARY KEY, val INTEGER NOT NULL)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := mgr.DB().Exec(`INSERT INTO counters (id, val) VALUES (1, 0)`); err != nil {
		t.Fatalf("seed row: %v", err)
	}

	const (
		goroutines = 4
		iterations = 25
		// Simulates work held inside the tx (think bcrypt). Long enough
		// that a sibling writer is virtually guaranteed to commit during
		// the window — that's what made BUSY_SNAPSHOT flake into a
		// near-certainty in serve mode.
		holdInsideTx = 5 * time.Millisecond
	)

	var wg sync.WaitGroup
	errs := make(chan error, goroutines*iterations)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				if err := bumpCounterInTx(mgr, holdInsideTx); err != nil {
					errs <- err
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			if strings.Contains(err.Error(), "database is locked") {
				t.Fatalf("regression: %v", err)
			}
			t.Fatalf("unexpected error: %v", err)
		}
	}

	var got int
	if err := mgr.DB().Get(&got, `SELECT val FROM counters WHERE id = 1`); err != nil {
		t.Fatalf("read counter: %v", err)
	}
	if want := goroutines * iterations; got != want {
		t.Fatalf("counter mismatch (lost updates): got %d, want %d", got, want)
	}
}

// bumpCounterInTx mirrors the read-then-CPU-then-write shape that
// CreateUser uses: SELECT the current value, hold the tx open for a
// CPU-ish moment, then UPDATE based on what was read. With IMMEDIATE
// locking each call serializes cleanly; with DEFERRED it races the
// snapshot.
func bumpCounterInTx(mgr *database.SqliteManager, hold time.Duration) error {
	ctx, err := mgr.BeginTx(context.Background())
	if err != nil {
		return err
	}

	tx := database.TxFromContext(ctx)
	var val int
	if err := tx.Get(&val, `SELECT val FROM counters WHERE id = 1`); err != nil {
		_ = mgr.Rollback(ctx)
		return err
	}

	time.Sleep(hold)

	if _, err := tx.Exec(`UPDATE counters SET val = ? WHERE id = 1`, val+1); err != nil {
		_ = mgr.Rollback(ctx)
		return err
	}

	return mgr.Commit(ctx)
}

func newTestManager(t *testing.T) *database.SqliteManager {
	t.Helper()
	cfg := &config.Config{
		DBPath: filepath.Join(t.TempDir(), "test.db"),
	}
	mgr, err := database.NewSqliteManager(cfg)
	if err != nil {
		t.Fatalf("open manager: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Close() })
	return mgr
}
