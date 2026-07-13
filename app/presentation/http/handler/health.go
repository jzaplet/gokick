package handler

import (
	"gokick/app/presentation/http/response"
	"net/http"
)

type HealthHandler struct{}

func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

//gkts:HealthResponse assets/app/Home/types/HealthResponse.ts
type healthResponse struct {
	Status string `json:"status"`
}

func (h *HealthHandler) Check(w http.ResponseWriter, _ *http.Request) {
	response.JSON(w, http.StatusOK, healthResponse{Status: "ok"})
}
