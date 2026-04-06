package server

import (
	"log/slog"
	"myapp/app/infrastructure/config"
	"myapp/app/presentation/http/handler"
	"myapp/app/presentation/http/middleware"
	"net/http"
)

type Server struct {
	config *config.Config
	logger *slog.Logger
	health *handler.HealthHandler
	spa    *handler.SPAHandler
}

func NewServer(
	config *config.Config,
	logger *slog.Logger,
	health *handler.HealthHandler,
	spa *handler.SPAHandler,
) *Server {
	return &Server{
		config: config,
		logger: logger,
		health: health,
		spa:    spa,
	}
}

func (s *Server) Start() error {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", s.health.Handle)

	// SPA catch-all — must be last
	mux.HandleFunc("GET /{path...}", s.spa.Handle)

	chain := s.buildMiddlewareChain(mux)

	addr := ":" + s.config.HTTPPort
	s.logger.Info("server: starting", "addr", addr)
	return http.ListenAndServe(addr, chain)
}

func (s *Server) buildMiddlewareChain(handler http.Handler) http.Handler {
	csrf := &http.CrossOriginProtection{}

	// Pořadí: Trace → CORS → CSRF → Logging (→ handler)
	// Aplikuje se od posledního, takže definice je obrácená:
	middlewares := []func(http.Handler) http.Handler{
		middleware.TraceMiddleware(),
		middleware.CORSMiddleware(s.config.CORSOrigin),
		csrf.Handler,
		middleware.LoggingMiddleware(s.logger),
	}

	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}
