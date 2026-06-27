package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunPath_NeverOpensTransaction is a STATIC conformance gate over the durable-run
// execution PLUMBING (the run worker + the run application layer): none of it may open
// a transaction. A tx in the run path would hold the global SQLite write lock for the
// run's whole lifetime (minutes to hours) and freeze every other write. This gate is a
// build-time backstop; the LOAD-BEARING protection — including for app-provided run
// HANDLERS, which live outside this file set — is the runtime guard (shared.ContextForbidTx
// → SqliteManager.BeginTx fails closed).
//
// Why a source scan and not go-arch-lint: the run worker shares this package with the
// JOB worker, which legitimately uses a transaction (runWithinTx). go-arch-lint works
// at package granularity and the transaction is injected via the shared.Transactor
// DOMAIN interface, so an import-level rule can neither isolate the run worker nor see
// the dependency. Scanning the run execution path's source for transaction-control
// syntax does both.
func TestRunPath_NeverOpensTransaction(t *testing.T) {
	t.Parallel()

	// Discover BOTH source sets by glob so a split (e.g. run_finalize.go) is auto-covered:
	// every run_*.go in this (worker) package + every file in the run application layer.
	workerRun, err := filepath.Glob("run_*.go")
	if err != nil {
		t.Fatalf("glob run_*.go: %v", err)
	}
	appRun, err := filepath.Glob(filepath.Join("..", "..", "application", "run", "*.go"))
	if err != nil {
		t.Fatalf("glob application/run: %v", err)
	}
	var files []string
	for _, f := range append(workerRun, appRun...) {
		if !strings.HasSuffix(f, "_test.go") {
			files = append(files, f)
		}
	}
	// Fail LOUD if either set vanished: filepath.Glob returns (nil, nil) on no match, so a
	// rename/restructure (e.g. application/run → a subpackage) would otherwise silently
	// shrink the gate and keep passing. run_worker.go + dispatcher.go + registry.go is the
	// floor today.
	if len(files) < 3 {
		t.Fatalf("gate scans too few files (%d) — did run_*.go or application/run move? files=%v",
			len(files), files)
	}

	// Transaction-control CALL/TYPE syntax. Covers the wrapper (SqliteManager.BeginTx) AND
	// the raw sqlx/database-sql openers (Begin/BeginTx/Beginx/BeginTxx/MustBegin), so a tx
	// opened by bypassing the wrapper is caught too — the runtime guard only sees the
	// wrapper, so the static gate must be wider. The trailing "(" / package-qualified type
	// keeps doc comments that merely mention "BeginTx" from tripping the gate.
	forbidden := []string{
		".Begin(", ".BeginTx(", ".Beginx(", ".BeginTxx(", ".MustBegin(", ".MustBeginTx(",
		".Commit(", ".Rollback(", "shared.Transactor",
	}
	for _, f := range files {
		src, readErr := os.ReadFile(f)
		if readErr != nil {
			t.Fatalf("read %s: %v", f, readErr)
		}
		for _, sym := range forbidden {
			if strings.Contains(string(src), sym) {
				t.Errorf(
					"%s references %q — the durable-run path must not open a transaction "+
						"(it runs outside-tx; persist via the Checkpointer or enqueue a command/job)",
					filepath.Base(f), sym,
				)
			}
		}
	}
}
