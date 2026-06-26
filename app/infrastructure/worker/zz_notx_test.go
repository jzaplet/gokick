package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunPath_NeverOpensTransaction is a STATIC conformance gate: the durable-run
// execution path must never open a transaction. A run handler runs outside-tx — a
// transaction would hold the global SQLite write lock for the run's whole lifetime
// (minutes to hours) and freeze every other write. The runtime guard backs this at
// execution time (shared.ContextForbidTx → SqliteManager.BeginTx fails closed); this
// gate backs it at build time.
//
// Why a source scan and not go-arch-lint: the run worker shares this package with the
// JOB worker, which legitimately uses a transaction (runWithinTx). go-arch-lint works
// at package granularity and the transaction is injected via the shared.Transactor
// DOMAIN interface, so an import-level rule can neither isolate the run worker nor see
// the dependency. Scanning the run execution path's source for transaction-control
// syntax does both.
func TestRunPath_NeverOpensTransaction(t *testing.T) {
	t.Parallel()

	// Files on the durable-run execution path: the run worker (this package) + the run
	// application layer (dispatcher, registry, handler contract).
	files := []string{"run_worker.go"}
	appRun, err := filepath.Glob(filepath.Join("..", "..", "application", "run", "*.go"))
	if err != nil {
		t.Fatalf("glob application/run: %v", err)
	}
	for _, f := range appRun {
		if !strings.HasSuffix(f, "_test.go") {
			files = append(files, f)
		}
	}

	// Transaction-control CALL/TYPE syntax (the trailing "(" / package-qualified type
	// keeps doc comments that merely mention "BeginTx" from tripping the gate).
	forbidden := []string{".BeginTx(", ".Commit(", ".Rollback(", "shared.Transactor"}
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
