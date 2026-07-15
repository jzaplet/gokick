// Package response mirrors the real wire plumbing for the boundary fixture.
package response

import (
	"context"
	"net/http"
)

type Responder struct{}

func NewResponder() *Responder { return &Responder{} }

func (rp *Responder) JSON(ctx context.Context, w http.ResponseWriter, status int, data any) {
	_, _, _, _ = ctx, w, status, data
}

func (rp *Responder) Error(ctx context.Context, w http.ResponseWriter, err error) {
	_, _, _ = ctx, w, err
}

// Legacy is a package-level writer — the free-function bypass rule's target.
func Legacy(w http.ResponseWriter, v any) { _, _ = w, v }
