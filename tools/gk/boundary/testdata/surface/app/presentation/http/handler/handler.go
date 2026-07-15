// Package handler calls the drifted JSON — the arity mismatch must be a
// violation, and with zero matching sites the per-kind guards have to fire.
package handler

import (
	"context"
	"net/http"

	"gokick/app/presentation/http/response"
)

//gkts:assets/x/SoloDTO.ts SoloDTO
type soloDTO struct {
	A string `json:"a"`
}

func solo(resp *response.Responder, w http.ResponseWriter) {
	resp.JSON(context.Background(), w, http.StatusOK, soloDTO{}, "extra")
}
