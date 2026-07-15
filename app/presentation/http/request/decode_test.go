package request

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type payload struct {
	Name string `json:"name"`
}

func decode(t *testing.T, body string) error {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()
	var got payload
	return DecodeJSON(rec, req, &got)
}

func TestDecodeJSON_AcceptsValidObject(t *testing.T) {
	if err := decode(t, `{"name":"alice"}`); err != nil {
		t.Fatalf("valid JSON should decode: %v", err)
	}
}

func TestDecodeJSON_RejectsUnknownFields(t *testing.T) {
	err := decode(t, `{"name":"alice","extra":"surprise"}`)
	if err == nil {
		t.Fatal("unknown field must be rejected (typos / over-posting)")
	}
}

func TestDecodeJSON_RejectsTrailingObject(t *testing.T) {
	err := decode(t, `{"name":"alice"}{"name":"mallory"}`)
	if err == nil {
		t.Fatal("trailing JSON value must be rejected")
	}
}

func TestDecodeJSON_RejectsOversizedBody(t *testing.T) {
	// MaxBytesReader counts every byte over the cap, including JSON
	// padding. We push past the cap with whitespace + a single key.
	big := append([]byte(`{"name":"`), bytes.Repeat([]byte("x"), int(MaxBodyBytes)+1024)...)
	big = append(big, []byte(`"}`)...)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(big))
	rec := httptest.NewRecorder()
	var got payload
	err := DecodeJSON(rec, req, &got)
	if err == nil {
		t.Fatal("body over MaxBodyBytes must be rejected")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error should mention size limit, got %q", err)
	}
}

// F-066: decode failures are typed *DecodeError (HTTPError) so a handler maps them
// via response.HandleError without hardcoding a status — oversize → 413, every
// other decode failure → 400.
func TestDecodeJSON_ErrorsCarryHTTPStatus(t *testing.T) {
	assertStatus := func(t *testing.T, err error, want int) {
		t.Helper()
		var de *DecodeError
		if !errors.As(err, &de) {
			t.Fatalf("expected *DecodeError, got %T: %v", err, err)
		}
		if de.HTTPStatus() != want {
			t.Fatalf("HTTPStatus: got %d want %d", de.HTTPStatus(), want)
		}
	}

	t.Run("unknown field is 400", func(t *testing.T) {
		assertStatus(t, decode(t, `{"extra":1}`), http.StatusBadRequest)
	})
	t.Run("trailing object is 400", func(t *testing.T) {
		assertStatus(t, decode(t, `{"name":"a"}{}`), http.StatusBadRequest)
	})
	t.Run("oversize is 413", func(t *testing.T) {
		big := append([]byte(`{"name":"`), bytes.Repeat([]byte("x"), int(MaxBodyBytes)+1024)...)
		big = append(big, []byte(`"}`)...)
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(big))
		rec := httptest.NewRecorder()
		var got payload
		assertStatus(t, DecodeJSON(rec, req, &got), http.StatusRequestEntityTooLarge)
	})
}
