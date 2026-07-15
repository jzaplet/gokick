// Package handler exercises every boundary rule: OK shapes (named/slice/
// pointer/alias/spaced directive), payload violations, escape-marker
// semantics (standalone, reasonless, trailing, unused) and the bypass rules.
package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"gokick/app/presentation/http/request"
	"gokick/app/presentation/http/response"
)

//gkts:assets/x/OkDTO.ts OkDTO
type okDTO struct {
	A string `json:"a"`
}

// aliasDTO aliases the annotated DTO — payloadOK must unalias (Go 1.26
// materializes aliases as *types.Alias).
type aliasDTO = okDTO

type plainDTO struct {
	B string `json:"b"`
}

// gkts:assets/x/SpacedDTO.ts SpacedDTO
type spacedDTO struct {
	C string `json:"c"`
}

func ok(resp *response.Responder, w http.ResponseWriter, r *http.Request) {
	resp.JSON(context.Background(), w, http.StatusOK, okDTO{A: "x"})
	resp.JSON(context.Background(), w, http.StatusOK, []okDTO{})
	resp.JSON(context.Background(), w, http.StatusOK, &okDTO{})
	resp.JSON(context.Background(), w, http.StatusOK, aliasDTO{A: "y"})
	resp.JSON(context.Background(), w, http.StatusOK, spacedDTO{C: "z"})

	var dst okDTO
	request.DecodeJSON(w, r, &dst)
}

func badPayloads(resp *response.Responder, w http.ResponseWriter) {
	resp.JSON(context.Background(), w, http.StatusOK, map[string]string{"x": "y"})
	resp.JSON(context.Background(), w, http.StatusOK, plainDTO{B: "b"})
}

func escapes(resp *response.Responder, w http.ResponseWriter) {
	//gkts:ignore fixture: exempted non-SPA endpoint
	resp.JSON(context.Background(), w, http.StatusOK, map[string]int{"n": 1})

	//gkts:ignore
	resp.JSON(context.Background(), w, http.StatusOK, map[string]int{"n": 2})

	resp.JSON(
		context.Background(),
		w,
		http.StatusOK,
		map[string]int{"n": 3},
	) //gkts:ignore trailing marker
	resp.JSON(context.Background(), w, http.StatusOK, map[string]int{"n": 4})

	//gkts:ignore fixture: stale marker with nothing to shield
	resp.JSON(context.Background(), w, http.StatusOK, okDTO{A: "fine"})
}

func bypasses(resp *response.Responder, w http.ResponseWriter) {
	_ = resp
	_ = response.NewResponder()
	response.Legacy(w, okDTO{})
	b, _ := json.Marshal(okDTO{A: "x"})
	_ = b

	//gkts:ignore fixture: an ignore must NOT silence a bypass
	_ = json.Valid([]byte("{}"))
}
