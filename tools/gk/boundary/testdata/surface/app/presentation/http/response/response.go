// Package response drifts from the anchored surface on purpose: JSON grew a
// 5th parameter and Push is a new any-payload method the gate doesn't know.
package response

import (
	"context"
	"net/http"
)

type Responder struct{}

func (rp *Responder) JSON(
	ctx context.Context,
	w http.ResponseWriter,
	status int,
	data any,
	extra string,
) {
	_, _, _, _, _ = ctx, w, status, data, extra
}

func (rp *Responder) Push(ctx context.Context, w http.ResponseWriter, data any) {
	_, _, _ = ctx, w, data
}
