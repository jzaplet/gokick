package run_test

import (
	"context"
	"path/filepath"
	"testing"

	"gokick/app/domain/run"
	"gokick/app/internal/testfx"
)

// Enqueuing a tenant-less run (a non-bus path that forgot to resolve the tenant) is
// fail-closed in multitenant mode — it must never silently land in the default
// tenant — but still defaults in single-tenant mode (unchanged behavior).
func TestEnqueue_TenantFailClosed(t *testing.T) {
	mt := testfx.NewMultitenant(t, filepath.Join(t.TempDir(), "run_enq_mt.db"))
	r := run.NewRun("agent", []byte(`{}`), 0)
	if r.TenantID != "" {
		t.Fatalf("precondition: NewRun leaves TenantID empty, got %q", r.TenantID)
	}
	if err := mt.Runs.Enqueue(context.Background(), r); err == nil {
		t.Fatal("multitenant enqueue with no tenant must fail closed")
	}

	st := testfx.New(t, filepath.Join(t.TempDir(), "run_enq_st.db"))
	if err := st.Runs.Enqueue(context.Background(), run.NewRun("agent", []byte(`{}`), 0)); err != nil {
		t.Fatalf("single-tenant enqueue with no tenant must default (not error), got %v", err)
	}
}
