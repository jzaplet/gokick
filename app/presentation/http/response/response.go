package response

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"gokick/app/domain/shared"
)

type HTTPError interface {
	error
	HTTPStatus() int
}

// FieldError is satisfied by domain errors that know which form field caused
// them (e.g. ValidationError{Field:"nickname"}). When present, Error() uses
// the field name as the JSON key so the frontend can route the message to the
// specific input; otherwise the message goes to the "general" key.
type FieldError interface {
	error
	ErrorField() string
}

// Responder writes JSON HTTP responses through the injected logger, so a failed
// response encode leaves a server-side signal instead of vanishing (F-067). It
// holds no per-request state — one instance is shared by every handler and
// middleware (injected via DI), the ctx is passed per call for trace correlation.
type Responder struct {
	logger *slog.Logger
}

func NewResponder(logger *slog.Logger) *Responder {
	return &Responder{logger: logger}
}

// JSON writes status + the JSON encoding of data (nil data = header only).
func (rp *Responder) JSON(ctx context.Context, w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		rp.encode(ctx, w, data)
	}
}

// Error writes status + a {field|general: message} body derived from err.
func (rp *Responder) Error(ctx context.Context, w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	body := map[string]string{}

	var fe FieldError
	if errors.As(err, &fe) && fe.ErrorField() != "" {
		body[fe.ErrorField()] = err.Error()
	} else {
		body["general"] = err.Error()
	}

	rp.encode(ctx, w, body)
}

// encode writes v as JSON and logs a failure. The status line is already sent, so
// the write can't be salvaged — but a swallowed error is a silent gap. Genuine
// unencodable payloads (a handler bug) log at Error; a write that fails because the
// client hung up mid-body (ctx cancelled) is not our bug and logs at Debug, so the
// tracker isn't flooded with disconnect noise.
func (rp *Responder) encode(ctx context.Context, w http.ResponseWriter, v any) {
	err := json.NewEncoder(w).Encode(v)
	if err == nil {
		return
	}
	level := slog.LevelError
	if ctx.Err() != nil {
		level = slog.LevelDebug
	}
	rp.logger.LogAttrs(ctx, level, "response: encode failed",
		append(shared.LogAttrs(ctx), slog.Any(shared.LogKeyError, err))...)
}

// errInternal is returned to the client on any non-HTTPError so we don't
// leak raw repo errors, panic messages, or other internals. Operators can
// correlate the real error via the trace_id surfaced in logs.
var errInternal = errors.New("internal server error")

// HandleError maps err to its HTTP status: an HTTPError uses its declared status,
// anything else funnels to a generic 500 (no internal leak).
func (rp *Responder) HandleError(ctx context.Context, w http.ResponseWriter, err error) {
	var httpErr HTTPError
	if errors.As(err, &httpErr) {
		rp.Error(ctx, w, httpErr.HTTPStatus(), err)
		return
	}
	rp.Error(ctx, w, http.StatusInternalServerError, errInternal)
}
