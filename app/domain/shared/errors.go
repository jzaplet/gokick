package shared

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"gokick/app/domain/shared/msgkey"
)

// The three domain error kinds carry a translation key (msgkey constant)
// plus optional template params instead of prose. The Responder renders the
// key in the request's language on the way out; Error() returns
// the raw key so logs, CLI output and tests stay locale-free.
//
// Two accessors, two audiences, on purpose: MessageKey() feeds the WIRE (the
// key the frontend looks up — it must stay the bare key forever), Error()
// feeds OPERATORS (CLI stderr, slog), where the params are half the message.

// errorText renders a keyed error for operator surfaces: the raw key plus its
// params, both locale-free. The params are what the message existed to convey
// — "user.password_too_short {count:8}" tells an operator the limit, the bare
// key does not. Names are sorted so the text is stable across runs. Never use
// this for the wire; that is MessageKey()'s job.
func errorText(key msgkey.Key, params map[string]any) string {
	if len(params) == 0 {
		return string(key)
	}
	pairs := make([]string, 0, len(params))
	for _, name := range slices.Sorted(maps.Keys(params)) {
		pairs = append(pairs, fmt.Sprintf("%s:%v", name, params[name]))
	}

	return fmt.Sprintf("%s {%s}", key, strings.Join(pairs, " "))
}

// ValidationError indicates user-supplied data failed validation. Maps to
// HTTP 400; a non-empty Field routes the message to that form field.
type ValidationError struct {
	Field  string
	Key    msgkey.Key
	Params map[string]any
}

func (e *ValidationError) Error() string                 { return errorText(e.Key, e.Params) }
func (e *ValidationError) HTTPStatus() int               { return 400 }
func (e *ValidationError) ErrorField() string            { return e.Field }
func (e *ValidationError) MessageKey() msgkey.Key        { return e.Key }
func (e *ValidationError) MessageParams() map[string]any { return e.Params }

// AuthError indicates the caller is not authenticated (no/invalid/expired
// credentials). Maps to HTTP 401.
type AuthError struct {
	Key msgkey.Key
}

func (e *AuthError) Error() string                 { return string(e.Key) }
func (e *AuthError) HTTPStatus() int               { return 401 }
func (e *AuthError) MessageKey() msgkey.Key        { return e.Key }
func (e *AuthError) MessageParams() map[string]any { return nil }

// PermissionError indicates the caller is authenticated but not permitted to
// perform the requested operation. Maps to HTTP 403.
type PermissionError struct {
	Key msgkey.Key
}

func (e *PermissionError) Error() string                 { return string(e.Key) }
func (e *PermissionError) HTTPStatus() int               { return 403 }
func (e *PermissionError) MessageKey() msgkey.Key        { return e.Key }
func (e *PermissionError) MessageParams() map[string]any { return nil }

// MessageError is the generic keyed error for presentation-level failures
// that are none of the three kinds above (rate limit 429, stray-API 404,
// the internal-error 500 body). Status is explicit because the key alone
// does not imply one.
type MessageError struct {
	Key    msgkey.Key
	Params map[string]any
	Status int
}

func (e *MessageError) Error() string                 { return errorText(e.Key, e.Params) }
func (e *MessageError) MessageKey() msgkey.Key        { return e.Key }
func (e *MessageError) MessageParams() map[string]any { return e.Params }

// HTTPStatus floors a forgotten Status at 500. Unlike its three siblings, which
// hardcode 400/401/403 and cannot be got wrong, this type takes the status from
// the caller — and a zero value would reach w.WriteHeader(0), which panics
// inside net/http BEFORE the header is written. RecoveryMiddleware's recorder
// has already flagged the response as started by then, so it declines its clean
// 500 and the client receives a silent 200 with an empty body.
func (e *MessageError) HTTPStatus() int {
	if e.Status == 0 {
		return 500
	}

	return e.Status
}
