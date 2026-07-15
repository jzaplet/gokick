package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"gokick/app/presentation/http/request"
	"gokick/app/presentation/http/response"
)

// methodValues copies the boundary functions into variables — payloads sent
// through the copies dodge the payload check, so the copy itself must be a
// violation.
func methodValues(resp *response.Responder, w http.ResponseWriter) {
	send := resp.JSON
	send(context.Background(), w, http.StatusOK, okDTO{})
	decode := request.DecodeJSON
	_ = decode
}

// rawWrites hand-builds payloads on the ResponseWriter — invisible to the
// payload check unless flagged.
func rawWrites(w http.ResponseWriter) {
	_, _ = w.Write([]byte("{}"))
	_, _ = io.WriteString(w, "{}")
	fmt.Fprintf(w, "{}")
	//gkts:ignore fixture: static bytes, not JSON
	_, _ = w.Write([]byte("<html>"))
}
