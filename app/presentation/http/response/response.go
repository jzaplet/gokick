package response

import (
	"encoding/json"
	"errors"
	"net/http"
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

func JSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

func Error(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	body := map[string]string{}

	var fe FieldError
	if errors.As(err, &fe) && fe.ErrorField() != "" {
		body[fe.ErrorField()] = err.Error()
	} else {
		body["general"] = err.Error()
	}

	_ = json.NewEncoder(w).Encode(body)
}

// errInternal is returned to the client on any non-HTTPError so we don't
// leak raw repo errors, panic messages, or other internals. Operators can
// correlate the real error via the trace_id surfaced in logs.
var errInternal = errors.New("internal server error")

func HandleError(w http.ResponseWriter, err error) {
	var httpErr HTTPError
	if errors.As(err, &httpErr) {
		Error(w, httpErr.HTTPStatus(), err)
		return
	}
	Error(w, http.StatusInternalServerError, errInternal)
}
