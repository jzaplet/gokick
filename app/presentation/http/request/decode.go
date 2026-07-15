// Package request houses helpers shared by HTTP handlers — body decoding,
// validation glue, etc. Handlers depend on this package instead of going
// straight to encoding/json so every endpoint inherits the same caps and
// strictness.
package request

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// MaxBodyBytes caps the request body at 1 MiB. JSON payloads in this app
// are nicknames / passwords / IDs — well under 1 KiB. The cap exists to
// reject malicious oversize bodies that would otherwise pin a goroutine
// and burn memory.
const MaxBodyBytes int64 = 1 << 20

// DecodeError is a request-body decode failure carrying the HTTP status it maps
// to. It satisfies response.HTTPError (HTTPStatus), so a handler hands it straight
// to response.HandleError — no hardcoded status, and no risk of the untyped-error
// → 500 funnel trap. 413 for an oversize body, 400 for every other decode failure.
type DecodeError struct {
	status int
	msg    string
}

func (e *DecodeError) Error() string   { return e.msg }
func (e *DecodeError) HTTPStatus() int { return e.status }

// DecodeJSON enforces three guarantees that plain json.Decoder doesn't:
//
//  1. body size capped at MaxBodyBytes (http.MaxBytesReader)
//  2. unknown JSON fields rejected (catches typos in field names that
//     would otherwise silently no-op, plus reduces accidental over-posting)
//  3. exactly one JSON value present (rejects payloads like `{}{}`)
//
// Every failure is a typed *DecodeError (an HTTPError) with the right status, so a
// handler just calls response.HandleError without branching on encoding/json.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return &DecodeError{
				status: http.StatusRequestEntityTooLarge,
				msg:    fmt.Sprintf("request body exceeds %d bytes", MaxBodyBytes),
			}
		}
		return &DecodeError{
			status: http.StatusBadRequest,
			msg:    fmt.Sprintf("invalid request body: %v", err),
		}
	}

	if dec.More() {
		return singleObjectError()
	}
	if err := dec.Decode(&struct{}{}); err != nil && !errors.Is(err, io.EOF) {
		return singleObjectError()
	}

	return nil
}

func singleObjectError() *DecodeError {
	return &DecodeError{
		status: http.StatusBadRequest,
		msg:    "request body must contain a single JSON object",
	}
}
