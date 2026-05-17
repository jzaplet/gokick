package response

import (
	"errors"
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

func TestHandleError_HTTPErrorPropagatesStatusAndMessage(t *testing.T) {
	rec := httptest.NewRecorder()
	HandleError(rec, &fakeHTTPErr{msg: "validation failed", status: http.StatusBadRequest})

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
	HandleError(rec, errors.New("sqlite: no such table 'users'"))

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
