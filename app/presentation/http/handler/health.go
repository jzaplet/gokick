package handler

import (
	"gokick/app/presentation/http/response"
	"net/http"
)

type HealthHandler struct {
	resp *response.Responder
}

func NewHealthHandler(resp *response.Responder) *HealthHandler {
	return &HealthHandler{resp: resp}
}

// healthResponse is intentionally NOT under tsgen (no //gkts): the /health
// endpoint is infra-only (liveness/readiness probes), never called by the Vue
// app, so a generated FE type would be dead code (knip flags it). Kept as a
// named struct regardless — a typed response beats an inline map.
type healthResponse struct {
	Status string `json:"status"`
}

func (h *HealthHandler) Check(w http.ResponseWriter, r *http.Request) {
	h.resp.JSON(r.Context(), w, http.StatusOK, healthResponse{Status: "ok"})
}
