package job_test

import (
	"context"
	"path/filepath"
	"testing"

	"gokick/app/domain/job"
	"gokick/app/internal/testfx"
)

// Mirror of the run repo: enqueuing a tenant-less job is fail-closed in multitenant
// mode (never silently the default tenant) and defaults in single-tenant mode.
func TestEnqueue_TenantFailClosed(t *testing.T) {
	mt := testfx.NewMultitenant(t, filepath.Join(t.TempDir(), "job_enq_mt.db"))
	j := job.NewJob("welcome:send", []byte(`{}`), 0)
	if j.TenantID != "" {
		t.Fatalf("precondition: NewJob leaves TenantID empty, got %q", j.TenantID)
	}
	if err := mt.Jobs.Enqueue(context.Background(), j); err == nil {
		t.Fatal("multitenant enqueue with no tenant must fail closed")
	}

	st := testfx.New(t, filepath.Join(t.TempDir(), "job_enq_st.db"))
	if err := st.Jobs.Enqueue(context.Background(), job.NewJob("welcome:send", []byte(`{}`), 0)); err != nil {
		t.Fatalf("single-tenant enqueue with no tenant must default (not error), got %v", err)
	}
}
