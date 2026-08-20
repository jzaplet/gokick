// Package request houses helpers shared by HTTP handlers — body decoding,
// validation glue, etc. Handlers depend on this package instead of going
// straight to encoding/json so every endpoint inherits the same caps and
// strictness.
package request

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"gokick/app/domain/shared"
	"gokick/app/domain/shared/msgkey"
)

// MaxBodyBytes caps the request body at 1 MiB. JSON payloads in this app
// are nicknames / passwords / IDs — well under 1 KiB. The cap exists to
// reject malicious oversize bodies that would otherwise pin a goroutine
// and burn memory.
const MaxBodyBytes int64 = 1 << 20

// DecodeJSON enforces three guarantees that plain json.Decoder doesn't:
//
//  1. body size capped at MaxBodyBytes (http.MaxBytesReader)
//  2. unknown JSON fields rejected (catches typos in field names that
//     would otherwise silently no-op, plus reduces accidental over-posting)
//  3. exactly one JSON value present (rejects payloads like `{}{}`)
//
// Every failure is a typed *shared.MessageError carrying its own HTTP status
// (413 for an oversize body, 400 for every other decode failure) and a
// translation key, so a handler just calls response.HandleError without
// branching on encoding/json and the message renders in the request language.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return &shared.MessageError{
				Key:    msgkey.RequestBodyTooLarge,
				Params: map[string]any{"count": MaxBodyBytes},
				Status: http.StatusRequestEntityTooLarge,
			}
		}
		// The decoder's prose (unknown field, type mismatch) rides along as the
		// technical {detail} param — untranslated on purpose, it names Go types
		// and JSON fields and exists for the developer reading the response,
		// not for end users.
		return &shared.MessageError{
			Key:    msgkey.RequestBodyInvalid,
			Params: map[string]any{"detail": err.Error()},
			Status: http.StatusBadRequest,
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

func singleObjectError() *shared.MessageError {
	return &shared.MessageError{
		Key:    msgkey.RequestSingleJSONObjectRequired,
		Status: http.StatusBadRequest,
	}
}
