package handler

import (
	"io/fs"
	"net/http"
	"strings"
)

type SPAHandler struct {
	fs    http.Handler
	index []byte
}

func NewSPAHandler(publicFS fs.FS) *SPAHandler {
	index, err := fs.ReadFile(publicFS, "index.html")
	if err != nil {
		index = []byte(
			"<!doctype html><html><body>Frontend not built. Run: yarn build</body></html>",
		)
	}

	return &SPAHandler{
		fs:    http.FileServerFS(publicFS),
		index: index,
	}
}

func (h *SPAHandler) Serve(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Try serving static file first (JS, CSS, assets)
	if strings.Contains(path, ".") {
		h.fs.ServeHTTP(w, r)
		return
	}

	// SPA fallback — serve index.html for all other routes
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(h.index)
}
