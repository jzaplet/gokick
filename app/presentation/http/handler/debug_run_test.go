package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gokick/app/internal/testfx"
)

// Exercises the debug run endpoints against a real repo (testfx SQLite): enqueue
// returns an id and persists a pending run, Get projects it, a missing id is 404,
// and Cancel sets the operator signal on the row.
func TestDebugRunHandler_EnqueueGetCancel(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/debugrun.db")
	h := NewDebugRunHandler(testResponder(), fx.Runs)

	// --- Enqueue ---
	req := httptest.NewRequest(
		http.MethodPost,
		"/debug/runs/e2e:checkpoint",
		bytes.NewBufferString(`{"steps":3}`),
	)
	req.SetPathValue("kind", "e2e:checkpoint")
	rec := httptest.NewRecorder()
	h.Enqueue(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("enqueue: got %d want 202; body=%s", rec.Code, rec.Body.String())
	}
	var enq map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &enq); err != nil {
		t.Fatalf("decode enqueue: %v", err)
	}
	id := enq["id"]
	if id == "" {
		t.Fatal("enqueue must return an id")
	}

	// --- Get ---
	greq := httptest.NewRequest(http.MethodGet, "/debug/runs/"+id, nil)
	greq.SetPathValue("id", id)
	grec := httptest.NewRecorder()
	h.Get(grec, greq)
	if grec.Code != http.StatusOK {
		t.Fatalf("get: got %d want 200", grec.Code)
	}
	var view debugRunView
	if err := json.Unmarshal(grec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode view: %v", err)
	}
	if view.ID != id || view.Kind != "e2e:checkpoint" {
		t.Fatalf("view mismatch: %+v", view)
	}
	if view.Status != "pending" {
		t.Fatalf("fresh run status: got %q want pending", view.Status)
	}

	// --- Get missing → 404 ---
	mreq := httptest.NewRequest(http.MethodGet, "/debug/runs/does-not-exist", nil)
	mreq.SetPathValue("id", "does-not-exist")
	mrec := httptest.NewRecorder()
	h.Get(mrec, mreq)
	if mrec.Code != http.StatusNotFound {
		t.Fatalf("missing run: got %d want 404", mrec.Code)
	}

	// --- Cancel ---
	creq := httptest.NewRequest(http.MethodPost, "/debug/runs/"+id+"/cancel", nil)
	creq.SetPathValue("id", id)
	crec := httptest.NewRecorder()
	h.Cancel(crec, creq)
	if crec.Code != http.StatusAccepted {
		t.Fatalf("cancel: got %d want 202", crec.Code)
	}
	rn, err := fx.Runs.FindByID(context.Background(), id)
	if err != nil {
		t.Fatalf("reload run: %v", err)
	}
	if rn == nil || !rn.CancelRequested {
		t.Fatal("cancel must set cancel_requested on the row")
	}
}

// max_retries must be a non-negative integer.
func TestDebugRunHandler_Enqueue_RejectsBadMaxRetries(t *testing.T) {
	fx := testfx.New(t, t.TempDir()+"/debugrun_bad.db")
	h := NewDebugRunHandler(testResponder(), fx.Runs)
	req := httptest.NewRequest(http.MethodPost, "/debug/runs/e2e:succeed?max_retries=-1", nil)
	req.SetPathValue("kind", "e2e:succeed")
	rec := httptest.NewRecorder()
	h.Enqueue(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("negative max_retries: got %d want 400", rec.Code)
	}
}
