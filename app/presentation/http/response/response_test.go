package response

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeHTTPErr struct {
	msg    string
	status int
}

func (e *fakeHTTPErr) Error() string   { return e.msg }
func (e *fakeHTTPErr) HTTPStatus() int { return e.status }

// discardResponder is a Responder whose encode-failure log goes nowhere — for
// tests that assert on the HTTP response, not on the log line.
func discardResponder() *Responder {
	return NewResponder(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// capturingHandler records every slog.Record it handles so a test can assert the
// level and message of the encode-failure log.
type capturingHandler struct{ records []slog.Record }

func (h *capturingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *capturingHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r)
	return nil
}
func (h *capturingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *capturingHandler) WithGroup(string) slog.Handler      { return h }

func TestHandleError_HTTPErrorPropagatesStatusAndMessage(t *testing.T) {
	rec := httptest.NewRecorder()
	discardResponder().HandleError(
		context.Background(), rec,
		&fakeHTTPErr{msg: "validation failed", status: http.StatusBadRequest},
	)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "validation failed") {
		t.Fatalf("body should carry HTTPError message, got %q", rec.Body.String())
	}
}

// Non-HTTPError must collapse to a generic 500 — DB errors, panic
// messages, and the like must NOT reach the client.
func TestHandleError_GenericErrorIsSanitizedTo500(t *testing.T) {
	rec := httptest.NewRecorder()
	discardResponder().HandleError(context.Background(), rec, errors.New("sqlite: no such table 'users'"))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "sqlite") {
		t.Fatalf("response leaked internal error: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "internal server error") {
		t.Fatalf("expected generic message, got %s", rec.Body.String())
	}
}

// F-067: a payload that cannot be JSON-encoded (a handler bug, e.g. a channel
// field) must NOT vanish — with a live ctx it logs at Error, the meaningful
// server-side signal the finding asks for. A plain struct never reaches here.
func TestJSON_UnencodablePayloadLogsAtError(t *testing.T) {
	cap := &capturingHandler{}
	resp := NewResponder(slog.New(cap))
	rec := httptest.NewRecorder()

	resp.JSON(context.Background(), rec, http.StatusOK, map[string]any{"x": make(chan int)})

	if len(cap.records) != 1 {
		t.Fatalf("expected exactly one log record, got %d", len(cap.records))
	}
	if cap.records[0].Level != slog.LevelError {
		t.Fatalf(
			"unencodable payload with a live ctx must log at Error, got %v",
			cap.records[0].Level,
		)
	}
	if cap.records[0].Message != "response: encode failed" {
		t.Fatalf("unexpected log message: %q", cap.records[0].Message)
	}
}

// The level split: when the ctx is already cancelled (the client hung up), a
// failed write is not our bug — it logs at Debug so the tracker isn't flooded
// with disconnect noise. Triggered here with an unencodable payload + a cancelled
// ctx, which exercises the exact ctx.Err() branch.
func TestJSON_EncodeFailureOnCancelledCtxLogsAtDebug(t *testing.T) {
	cap := &capturingHandler{}
	resp := NewResponder(slog.New(cap))
	rec := httptest.NewRecorder()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resp.JSON(ctx, rec, http.StatusOK, map[string]any{"x": make(chan int)})

	if len(cap.records) != 1 {
		t.Fatalf("expected exactly one log record, got %d", len(cap.records))
	}
	if cap.records[0].Level != slog.LevelDebug {
		t.Fatalf(
			"encode failure on a cancelled ctx must log at Debug, got %v",
			cap.records[0].Level,
		)
	}
}

// The happy path must log nothing: a normal struct always encodes, so no
// encode-failure line should ever fire.
func TestJSON_SuccessLogsNothing(t *testing.T) {
	cap := &capturingHandler{}
	resp := NewResponder(slog.New(cap))
	rec := httptest.NewRecorder()

	resp.JSON(context.Background(), rec, http.StatusOK, map[string]string{"ok": "yes"})

	if len(cap.records) != 0 {
		t.Fatalf("a successful encode must log nothing, got %d records", len(cap.records))
	}
}
