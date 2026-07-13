package handler

import (
	"gokick/app/presentation/http/response"
	"net/http"
)

type HealthHandler struct{}

func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

// healthResponse is intentionally NOT under tsgen (no //gkts): the /health
// endpoint is infra-only (liveness/readiness probes), never called by the Vue
// app, so a generated FE type would be dead code (knip flags it). Kept as a
// named struct regardless — a typed response beats an inline map.
type healthResponse struct {
	Status string `json:"status"`
}

func (h *HealthHandler) Check(w http.ResponseWriter, _ *http.Request) {
	response.JSON(w, http.StatusOK, healthResponse{Status: "ok"})
}
