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

// DecodeJSON enforces three guarantees that plain json.Decoder doesn't:
//
//  1. body size capped at MaxBodyBytes (http.MaxBytesReader)
//  2. unknown JSON fields rejected (catches typos in field names that
//     would otherwise silently no-op, plus reduces accidental over-posting)
//  3. exactly one JSON value present (rejects payloads like `{}{}`)
//
// All decode failures collapse into a single sentinel-shaped error so the
// handler can map them to 400 without branching on encoding/json internals.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return fmt.Errorf("request body exceeds %d bytes", MaxBodyBytes)
		}
		return fmt.Errorf("invalid request body: %w", err)
	}

	if dec.More() {
		return errors.New("request body must contain a single JSON object")
	}
	if err := dec.Decode(&struct{}{}); err != nil && !errors.Is(err, io.EOF) {
		return errors.New("request body must contain a single JSON object")
	}

	return nil
}
