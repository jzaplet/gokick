package request

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gokick/app/domain/shared"
	"gokick/app/domain/shared/msgkey"
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
	var de *shared.MessageError
	if !errors.As(err, &de) {
		t.Fatalf("expected *shared.MessageError, got %T", err)
	}
	if de.MessageKey() != msgkey.RequestBodyTooLarge {
		t.Fatalf("key: got %q want %q", de.MessageKey(), msgkey.RequestBodyTooLarge)
	}
	// Error() is the OPERATOR rendering, so it carries the params the wire key
	// deliberately leaves out — the byte cap is the whole point of the message.
	if want := "request.body_too_large {count:1048576}"; de.Error() != want {
		t.Fatalf("Error(): got %q want %q", de.Error(), want)
	}
	if de.MessageParams()["count"] != MaxBodyBytes {
		t.Fatalf("params: got %v, want Count=%d", de.MessageParams(), MaxBodyBytes)
	}
}

// F-066: decode failures are typed *shared.MessageError (an HTTPError) so a
// handler maps them via response.HandleError without hardcoding a status —
// oversize → 413, every other decode failure → 400.
func TestDecodeJSON_ErrorsCarryHTTPStatus(t *testing.T) {
	assertStatus := func(t *testing.T, err error, want int) {
		t.Helper()
		var de *shared.MessageError
		if !errors.As(err, &de) {
			t.Fatalf("expected *shared.MessageError, got %T: %v", err, err)
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
