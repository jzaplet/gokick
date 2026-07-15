package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"gokick/app/domain/run"
	"gokick/app/presentation/http/response"
)

// DebugRunHandler exposes enqueue / observe / cancel for durable runs behind
// APP_RUN_DEBUG, so a live or local deploy can drive the e2e:* run kinds and assert
// the engine's process-lifecycle behavior (crash → resume, at-least-once, drain).
// Like the /debug/sentry trigger it is a test-only affordance that talks to the run
// Repository DIRECTLY (bypassing the bus); it is registered only when config.RunDebug
// is on (see server.registerRoutes) and must never be enabled in production.
type DebugRunHandler struct {
	resp *response.Responder
	runs run.Repository
}

func NewDebugRunHandler(resp *response.Responder, runs run.Repository) *DebugRunHandler {
	return &DebugRunHandler{resp: resp, runs: runs}
}

const debugRunMaxPayload = 4 << 10 // 4 KiB — e2e payloads are tiny (steps/sleep config)

var (
	errBadMaxRetries = errors.New("max_retries must be an integer >= 0")
	errRunNotFound   = errors.New("run not found")
)

// Enqueue inserts a run of the {kind} path segment. The request body (if any)
// becomes the run payload verbatim — e.g. {"steps":10,"sleep_ms":1000} for
// e2e:checkpoint. max_retries is an optional query param (default 0). Returns the
// new run id; the worker picks it up on its next poll.
func (h *DebugRunHandler) Enqueue(w http.ResponseWriter, r *http.Request) {
	// Reject an oversize body (400) rather than silently truncating it — matches
	// the request.DecodeJSON idiom instead of io.LimitReader's quiet cut, so a
	// too-big payload can't enqueue a corrupt run that looks like success.
	r.Body = http.MaxBytesReader(w, r.Body, debugRunMaxPayload)
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		h.resp.Error(r.Context(), w, http.StatusBadRequest, err)
		return
	}

	maxRetries := 0
	if v := r.URL.Query().Get("max_retries"); v != "" {
		if maxRetries, err = strconv.Atoi(v); err != nil || maxRetries < 0 {
			h.resp.Error(r.Context(), w, http.StatusBadRequest, errBadMaxRetries)
			return
		}
	}

	rn, err := run.NewRun(r.PathValue("kind"), payload, maxRetries)
	if err != nil {
		h.resp.Error(r.Context(), w, http.StatusBadRequest, err)
		return
	}
	if err := h.runs.Enqueue(r.Context(), rn); err != nil {
		h.resp.HandleError(r.Context(), w, err)
		return
	}
	//gkts:ignore APP_RUN_DEBUG e2e-only endpoint, consumed by the shell harness, not the SPA
	h.resp.JSON(
		r.Context(),
		w,
		http.StatusAccepted,
		map[string]string{"id": rn.ID, "kind": rn.Kind},
	)
}

// Get returns the run's observable state (status / checkpoint / counters) so a
// harness can poll for completion or verify a resume (reclaims > 0, state advanced).
func (h *DebugRunHandler) Get(w http.ResponseWriter, r *http.Request) {
	rn, err := h.runs.FindByID(r.Context(), r.PathValue("id"))
	if err != nil {
		h.resp.HandleError(r.Context(), w, err)
		return
	}
	if rn == nil {
		h.resp.Error(r.Context(), w, http.StatusNotFound, errRunNotFound)
		return
	}
	//gkts:ignore APP_RUN_DEBUG e2e-only endpoint, consumed by the shell harness, not the SPA
	h.resp.JSON(r.Context(), w, http.StatusOK, viewRun(rn))
}

// Cancel sets the operator cancel signal (idempotent; a no-op on a terminal run).
func (h *DebugRunHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	if err := h.runs.RequestCancel(r.Context(), r.PathValue("id")); err != nil {
		h.resp.HandleError(r.Context(), w, err)
		return
	}
	//gkts:ignore APP_RUN_DEBUG e2e-only endpoint, consumed by the shell harness, not the SPA
	h.resp.JSON(
		r.Context(),
		w,
		http.StatusAccepted,
		map[string]string{"status": "cancel requested"},
	)
}

// debugRunView is the JSON projection of a run — the fields a lifecycle test needs.
type debugRunView struct {
	ID              string          `json:"id"`
	Kind            string          `json:"kind"`
	Status          string          `json:"status"` // pending | running | completed | failed | cancelled
	State           json.RawMessage `json:"state,omitempty"`
	Attempts        int             `json:"attempts"`
	Reclaims        int             `json:"reclaims"`
	Parks           int             `json:"parks"`
	CancelRequested bool            `json:"cancel_requested"`
	LastError       *string         `json:"last_error,omitempty"`
}

func viewRun(rn *run.Run) debugRunView {
	v := debugRunView{
		ID:   rn.ID,
		Kind: rn.Kind,
		// Derive the label from the domain method — the ONE terminal/lease-state
		// derivation — instead of re-expressing the switch here.
		Status:          rn.Status(time.Now()),
		Attempts:        rn.Attempts,
		Reclaims:        rn.Reclaims,
		Parks:           rn.Parks,
		CancelRequested: rn.CancelRequested,
		LastError:       rn.LastError,
	}
	if len(rn.State) > 0 {
		v.State = json.RawMessage(rn.State)
	}
	return v
}
