package response

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"gokick/app/domain/shared"
	"gokick/app/domain/shared/msgkey"
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

// KeyedError is satisfied by errors that carry a translation key + params
// instead of prose (ValidationError, AuthError, PermissionError,
// MessageError). MessageKey() is deliberately separate from Error(): the wire
// needs the BARE key the frontend looks up, while Error() is free to add the
// params for operators. Anything that does not implement this interface never
// reaches the wire as itself — Error() substitutes the generic internal-error
// key rather than shipping prose in the key slot.
type KeyedError interface {
	error
	MessageKey() msgkey.Key
	MessageParams() map[string]any
}

// ApiMessage is the wire shape of one error message: a translation key into
// the locale/ catalogs plus the params that fill its {placeholders} (a
// "count" param additionally selects the CLDR plural form). The API never
// ships prose — the FRONTEND renders the text in the user's language.
//
//gkts:assets/app-ui/Fetch/types/ApiMessage.ts ApiMessage noguard
type ApiMessage struct {
	Key    string         `json:"key"`
	Params map[string]any `json:"params,omitempty"`
}

// Responder writes JSON HTTP responses through the injected logger, so a failed
// response encode leaves a server-side signal instead of vanishing (F-067). It
// holds no per-request state — one instance is shared by every handler and
// middleware (injected via DI); ctx carries the per-request bits (trace
// correlation).
type Responder struct {
	logger *slog.Logger
}

// msgUnkeyedError is logged when a handler hands Error() something that is not
// a KeyedError: the client gets the generic internal-error key, so the real
// text must not vanish with it.
const msgUnkeyedError = "response: unkeyed error replaced with the generic key"

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

// Error writes status + a {field|general: {key, params}} body derived from
// err. A KeyedError contributes its key + params; anything else contributes
// the generic internal-error key and logs its real text server-side.
//
// The fallback is what keeps "the API ships keys, the frontend renders" true
// for every caller, not just the disciplined ones: err.Error() in the key slot
// would put an English sentence where the frontend expects a catalog key, so
// tm() would report it as an unknown key and render the raw prose at a Czech
// user. No linter can see what reaches this function, so the invariant has to
// hold here.
func (rp *Responder) Error(ctx context.Context, w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	message := ApiMessage{Key: string(msgkey.CommonInternalError)}
	var ke KeyedError
	if errors.As(err, &ke) {
		message = ApiMessage{Key: string(ke.MessageKey()), Params: ke.MessageParams()}
	} else {
		rp.logger.LogAttrs(ctx, slog.LevelWarn, msgUnkeyedError,
			append(shared.LogAttrs(ctx), slog.Any(shared.LogKeyError, err))...)
	}

	body := map[string]ApiMessage{}
	var fe FieldError
	if errors.As(err, &fe) && fe.ErrorField() != "" {
		body[fe.ErrorField()] = message
	} else {
		body["general"] = message
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
// leak raw repo errors, panic messages, or other internals. Keyed so the 500
// body renders like every other message; operators correlate the real
// error via the trace_id surfaced in logs.
var errInternal = &shared.MessageError{
	Key:    msgkey.CommonInternalError,
	Status: http.StatusInternalServerError,
}

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
