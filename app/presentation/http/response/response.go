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

func HandleError(w http.ResponseWriter, err error) {
	var httpErr HTTPError
	if errors.As(err, &httpErr) {
		Error(w, httpErr.HTTPStatus(), err)
	} else {
		Error(w, http.StatusInternalServerError, err)
	}
}
