package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gokick/app/domain/shared"
)

// decodeBody unmarshals the recorder body into a string-keyed map so tests can
// assert exact keys/values and the absence of unexpected keys.
func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not valid JSON object: %v (raw=%q)", err, rec.Body.String())
	}
	return body
}

// Closes domain-55 (the 400/401/403 legs), guide-forms-fe-01/02/07/08/09,
// presentation-50, presentation-51: real shared.* error types routed through
// HandleError must map to the documented status and JSON shape. Using the real
// types (not a stub) means this fails if ValidationError.HTTPStatus() drifts off
// 400, AuthError off 401, or PermissionError off 403, or if the field/general
// keying logic breaks.
func TestHandleError_RealDomainErrorMapping(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode int
		wantKey  string
		wantMsg  string
	}{
		{
			name:     "ValidationError routes to its field key with 400",
			err:      &shared.ValidationError{Field: "nickname", Message: "nickname is required"},
			wantCode: http.StatusBadRequest,
			wantKey:  "nickname",
			wantMsg:  "nickname is required",
		},
		{
			name:     "AuthError routes to general with 401",
			err:      &shared.AuthError{Message: "invalid credentials"},
			wantCode: http.StatusUnauthorized,
			wantKey:  "general",
			wantMsg:  "invalid credentials",
		},
		{
			name:     "PermissionError routes to general with 403",
			err:      &shared.PermissionError{Message: "forbidden"},
			wantCode: http.StatusForbidden,
			wantKey:  "general",
			wantMsg:  "forbidden",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			HandleError(rec, tc.err)

			if rec.Code != tc.wantCode {
				t.Fatalf("status: got %d want %d", rec.Code, tc.wantCode)
			}

			body := decodeBody(t, rec)

			// Exactly one key — the handler returns the first error it hits,
			// so the body never carries both a field key and "general".
			if len(body) != 1 {
				t.Fatalf("body must have exactly one key, got %d: %v", len(body), body)
			}
			got, ok := body[tc.wantKey]
			if !ok {
				t.Fatalf("missing key %q in body %v", tc.wantKey, body)
			}
			if got != tc.wantMsg {
				t.Fatalf("body[%q]: got %q want %q", tc.wantKey, got, tc.wantMsg)
			}
			// A field error must NOT also populate "general", and vice versa.
			if tc.wantKey != "general" {
				if _, leaked := body["general"]; leaked {
					t.Fatalf("field error also populated \"general\": %v", body)
				}
			}
		})
	}
}

// Closes domain-55 (the "all other errors -> 500" leg is already covered by
// TestHandleError_GenericErrorIsSanitizedTo500; here we additionally pin that a
// *shared.ValidationError, AuthError and PermissionError each carry their OWN
// status rather than collapsing to 500). This is the complement that guarantees
// the real types are recognized as HTTPErrors at all.
func TestHandleError_RealDomainErrorsAreNotCollapsedTo500(t *testing.T) {
	cases := []error{
		&shared.ValidationError{Field: "x", Message: "m"},
		&shared.AuthError{Message: "m"},
		&shared.PermissionError{Message: "m"},
	}
	for _, err := range cases {
		rec := httptest.NewRecorder()
		HandleError(rec, err)
		if rec.Code == http.StatusInternalServerError {
			t.Fatalf("%T was collapsed to 500; expected its own status", err)
		}
	}
}

// Closes guide-forms-fe-02, guide-forms-fe-07, presentation-48, presentation-50:
// Error() called directly (not via HandleError) with a field-bearing
// ValidationError writes status 400 and a body keyed by the field name. Matches
// the claims that name Error() explicitly.
func TestError_FieldErrorKeyedByFieldName(t *testing.T) {
	rec := httptest.NewRecorder()
	verr := &shared.ValidationError{Field: "nickname", Message: "nickname is required"}

	Error(rec, verr.HTTPStatus(), verr)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want 400", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type: got %q want application/json", ct)
	}

	body := decodeBody(t, rec)
	if len(body) != 1 {
		t.Fatalf("expected single-key body, got %v", body)
	}
	if body["nickname"] != "nickname is required" {
		t.Fatalf("body[nickname]: got %q want %q", body["nickname"], "nickname is required")
	}
	if _, ok := body["general"]; ok {
		t.Fatalf("field error must not populate \"general\": %v", body)
	}
}

// Closes guide-forms-fe-08, presentation-48, presentation-51: Error() with an
// error that has no field (AuthError) lands under "general". Drives the else
// branch of Error() directly via the real AuthError type.
func TestError_NonFieldErrorKeyedAsGeneral(t *testing.T) {
	rec := httptest.NewRecorder()
	aerr := &shared.AuthError{Message: "invalid credentials"}

	Error(rec, aerr.HTTPStatus(), aerr)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want 401", rec.Code)
	}

	body := decodeBody(t, rec)
	if len(body) != 1 {
		t.Fatalf("expected single-key body, got %v", body)
	}
	if body["general"] != "invalid credentials" {
		t.Fatalf("body[general]: got %q want %q", body["general"], "invalid credentials")
	}
}

// Closes the FieldError-with-empty-field edge of presentation-48 and the
// general-fallback half of guide-forms-fe-08: a ValidationError whose Field is
// empty must fall through to "general" (Error() guards on ErrorField() != "").
func TestError_FieldErrorWithEmptyFieldFallsToGeneral(t *testing.T) {
	rec := httptest.NewRecorder()
	// Field is empty on purpose.
	verr := &shared.ValidationError{Message: "something is wrong"}

	Error(rec, http.StatusBadRequest, verr)

	body := decodeBody(t, rec)
	if len(body) != 1 {
		t.Fatalf("expected single-key body, got %v", body)
	}
	if body["general"] != "something is wrong" {
		t.Fatalf("empty-field ValidationError should land under general, got %v", body)
	}
}

// Closes presentation-47: JSON() serializes data, sets Content-Type
// application/json, and writes the given status code.
func TestJSON_SetsContentTypeStatusAndEncodesBody(t *testing.T) {
	rec := httptest.NewRecorder()
	payload := struct {
		Foo string `json:"foo"`
	}{Foo: "bar"}

	JSON(rec, http.StatusCreated, payload)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type: got %q want application/json", ct)
	}

	var decoded struct {
		Foo string `json:"foo"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("body is not valid JSON: %v (raw=%q)", err, rec.Body.String())
	}
	if decoded.Foo != "bar" {
		t.Fatalf("decoded body: got %+v want foo=bar", decoded)
	}
}

// Reinforces presentation-47: JSON() with nil data still sets the header and
// status and writes no body (the data != nil guard).
func TestJSON_NilDataWritesHeaderAndStatusOnly(t *testing.T) {
	rec := httptest.NewRecorder()

	JSON(rec, http.StatusNoContent, nil)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type: got %q want application/json", ct)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("nil data should write no body, got %q", rec.Body.String())
	}
}
