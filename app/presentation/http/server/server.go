package server

import (
	"log/slog"
	"net/http"

	"gokick/app/domain/shared"
	"gokick/app/infrastructure/config"
	"gokick/app/presentation/http/handler"
	"gokick/app/presentation/http/middleware"
)

type Server struct {
	config  *config.Config
	logger  *slog.Logger
	jwt     shared.JwtService
	health  *handler.HealthHandler
	spa     *handler.SPAHandler
	auth    *handler.AuthHandler
	profile *handler.ProfileHandler
}

func NewServer(
	config *config.Config,
	logger *slog.Logger,
	jwt shared.JwtService,
	health *handler.HealthHandler,
	spa *handler.SPAHandler,
	auth *handler.AuthHandler,
	profile *handler.ProfileHandler,
) *Server {
	return &Server{
		config:  config,
		logger:  logger,
		jwt:     jwt,
		health:  health,
		spa:     spa,
		auth:    auth,
		profile: profile,
	}
}

func (s *Server) Start() error {
	mux := s.registerRoutes()
	chain := s.buildMiddlewareChain(mux)

	addr := ":" + s.config.HTTPPort
	s.logger.Info("server: starting", "addr", addr)

	return http.ListenAndServe(addr, chain)
}

func (s *Server) registerRoutes() *http.ServeMux {
	mux := http.NewServeMux()

	// Public — no authentication required.
	mux.HandleFunc("GET /health", s.health.Check)
	mux.HandleFunc("POST /api/v1/auth/login", s.auth.Login)
	mux.HandleFunc("POST /api/v1/auth/refresh", s.auth.Refresh)

	// Protected — JWT Bearer required (AuthMiddleware populates claims,
	// bus AuthorizeMiddleware then enforces the per-command permission).
	authed := middleware.AuthMiddleware(s.jwt)
	mux.Handle("POST /api/v1/auth/logout", authed(http.HandlerFunc(s.auth.Logout)))
	mux.Handle("GET /api/v1/profile", authed(http.HandlerFunc(s.profile.Get)))
	mux.Handle("PUT /api/v1/profile/password", authed(http.HandlerFunc(s.profile.ChangePassword)))

	// SPA catch-all — must be last so explicit routes win.
	mux.HandleFunc("GET /{path...}", s.spa.Serve)

	return mux
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
