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
	config     *config.Config
	logger     *slog.Logger
	jwt        shared.JwtService
	health     *handler.HealthHandler
	spa        *handler.SPAHandler
	auth       *handler.AuthHandler
	profile    *handler.ProfileHandler
	adminUsers *handler.AdminUsersHandler
	dashboard  *handler.DashboardHandler
}

func NewServer(
	config *config.Config,
	logger *slog.Logger,
	jwt shared.JwtService,
	health *handler.HealthHandler,
	spa *handler.SPAHandler,
	auth *handler.AuthHandler,
	profile *handler.ProfileHandler,
	adminUsers *handler.AdminUsersHandler,
	dashboard *handler.DashboardHandler,
) *Server {
	return &Server{
		config:     config,
		logger:     logger,
		jwt:        jwt,
		health:     health,
		spa:        spa,
		auth:       auth,
		profile:    profile,
		adminUsers: adminUsers,
		dashboard:  dashboard,
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
	mux.Handle("GET /api/v1/dashboard/user", authed(http.HandlerFunc(s.dashboard.User)))

	// Admin — bus AuthorizeMiddleware enforces admin:* permission per command/query.
	mux.Handle("GET /api/v1/dashboard/admin", authed(http.HandlerFunc(s.dashboard.Admin)))
	mux.Handle("GET /api/v1/admin/users", authed(http.HandlerFunc(s.adminUsers.List)))
	mux.Handle("POST /api/v1/admin/users", authed(http.HandlerFunc(s.adminUsers.Create)))
	mux.Handle("PUT /api/v1/admin/users/{id}", authed(http.HandlerFunc(s.adminUsers.Update)))
	mux.Handle("DELETE /api/v1/admin/users/{id}", authed(http.HandlerFunc(s.adminUsers.Delete)))

	// SPA catch-all — must be last so explicit routes win.
	mux.HandleFunc("GET /{path...}", s.spa.Serve)

	return mux
}

func (s *Server) buildMiddlewareChain(handler http.Handler) http.Handler {
	csrf := &http.CrossOriginProtection{}

	// Pořadí: Trace → Security headers → CORS → CSRF → Logging (→ handler)
	// HSTS posíláme jen v produkci (gateováno na CookieSecure flag, který už
	// rozlišuje HTTPS provoz).
	middlewares := []func(http.Handler) http.Handler{
		middleware.TraceMiddleware(),
		middleware.SecurityHeadersMiddleware(s.config.CookieSecure),
		middleware.CORSMiddleware(s.config.CORSOrigin),
		csrf.Handler,
		middleware.LoggingMiddleware(s.logger),
	}

	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}

	return handler
}
