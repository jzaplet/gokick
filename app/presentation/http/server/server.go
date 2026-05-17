package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"gokick/app/domain/shared"
	"gokick/app/infrastructure/config"
	"gokick/app/presentation/http/handler"
	"gokick/app/presentation/http/middleware"
)

const shutdownGracePeriod = 30 * time.Second

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

func (s *Server) Start(ctx context.Context) error {
	mux := s.registerRoutes()
	chain := s.buildMiddlewareChain(mux)

	addr := ":" + s.config.HTTPPort
	s.logger.Info("server: starting", "addr", addr)
	return runWithShutdown(
		ctx,
		&http.Server{Addr: addr, Handler: chain},
		s.logger,
		shutdownGracePeriod,
	)
}

// runWithShutdown runs srv.ListenAndServe in a goroutine and waits for ctx
// cancellation. On cancel it drains inflight requests via srv.Shutdown with
// the given grace period. Extracted so server_test.go can exercise the same
// goroutine + select wiring against a hand-built http.Server.
func runWithShutdown(
	ctx context.Context,
	srv *http.Server,
	logger *slog.Logger,
	grace time.Duration,
) error {
	serverErr := make(chan error, 1)
	go func() {
		err := srv.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serverErr <- err
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		logger.Info("server: shutdown signal received, draining", "timeout", grace)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), grace)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("server: graceful shutdown failed", "error", err)
			return err
		}
		if err := <-serverErr; err != nil {
			logger.Error("server: listener exited with error during shutdown", "error", err)
		}
		logger.Info("server: stopped")
		return nil
	}
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

	// Order: Trace → Security headers → CORS → CSRF → Logging (→ handler).
	// HSTS is only emitted in production (gated on the CookieSecure flag,
	// which already distinguishes HTTPS traffic).
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
