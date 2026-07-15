// Package request mirrors the real decode plumbing for the boundary fixture.
package request

import "net/http"

func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	_, _, _ = w, r, dst
	return true
}
