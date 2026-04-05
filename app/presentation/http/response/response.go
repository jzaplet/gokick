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

func JSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

func Error(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error": err.Error(),
	})
}

func HandleError(w http.ResponseWriter, err error) {
	var httpErr HTTPError
	if errors.As(err, &httpErr) {
		Error(w, httpErr.HTTPStatus(), err)
	} else {
		Error(w, http.StatusInternalServerError, err)
	}
}
