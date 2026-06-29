package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunPath_NeverOpensTransaction is a STATIC conformance gate over the durable-run
// execution PLUMBING (the run worker + the run application layer): none of it may DRIVE
// a transaction (BeginTx/Commit/Rollback). A tx wrapped around the handler would hold
// the global SQLite write lock for the run's whole lifetime (minutes to hours) and
// freeze every other write — the failure the outside-tx model exists to avoid.
//
// It does NOT forbid the plumbing from HOLDING a shared.Transactor: the worker injects
// one into the handler ctx so a handler can open its OWN short transaction via
// shared.WithTx (atomic multi-row writes it scopes itself). WithTx lives in domain/shared
// (outside this file set) and is the only sanctioned caller of BeginTx/Commit/Rollback in
// the run path; forbidding that call syntax HERE keeps the plumbing from wrapping the
// handler. The LOAD-BEARING guard against an accidental implicit tx in an app HANDLER is
// the runtime check (shared.ContextForbidTx → SqliteManager.BeginTx fails closed);
// shared.WithTx clears the marker only for its own short scope.
//
// Why a source scan and not go-arch-lint: the transaction is driven via the
// shared.Transactor DOMAIN interface, so an import-level rule can't see the dependency.
// Scanning the run execution path's source for transaction-control call syntax does.
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

	// Transaction-control CALL syntax only (NOT the shared.Transactor type, which the
	// worker legitimately holds to back a handler's shared.WithTx). Covers the wrapper
	// (SqliteManager.BeginTx) AND the raw sqlx/database-sql openers, so a tx driven by
	// bypassing the wrapper is caught too. The trailing "(" keeps doc comments that merely
	// mention "BeginTx" from tripping the gate.
	forbidden := []string{
		".Begin(", ".BeginTx(", ".Beginx(", ".BeginTxx(", ".MustBegin(", ".MustBeginTx(",
		".Commit(", ".Rollback(",
		// WithTx clears the forbid marker for its scope, so it is the one sanctioned
		// bypass of the runtime guard. It is for app HANDLERS only; the run PLUMBING
		// wrapping the handler in it would re-introduce a long-held write lock.
		"shared.WithTx(",
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
