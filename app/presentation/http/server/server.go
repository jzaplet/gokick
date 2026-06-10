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

const (
	shutdownGracePeriod = 30 * time.Second

	// Conservative defaults that protect against Slowloris and oversized
	// header attacks without being so tight that legitimate slow clients
	// (mobile networks, large form posts) get cut off. Tune via constants
	// if your traffic profile differs — they're not environment-driven on
	// purpose, since the values should not vary per deployment.
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 60 * time.Second
	maxHeaderBytes    = 1 << 16 // 64 KiB
)

// RateLimiters bundles the per-endpoint limiters so Wire can hand a single
// value to the server. Order matches application registration (login,
// refresh) — add a field here when a new endpoint needs a dedicated bucket.
type RateLimiters struct {
	Login   *middleware.RateLimiter
	Refresh *middleware.RateLimiter
}

// janitorSweepInterval / janitorDropAfter pin the rate-limiter background
// sweeper. The interval is well below dropAfter so a quiet bucket actually
// gets dropped within one sweep instead of lingering forever.
const (
	janitorSweepInterval = time.Minute
	janitorDropAfter     = 5 * time.Minute
)

// Server-local structured-log keys (cross-cutting ones live in shared.LogKey*).
// sloglint's no-raw-keys forbids bare string keys.
const (
	logKeyAddr    = "addr"
	logKeyTimeout = "timeout"
)

type Server struct {
	config     *config.Config
	logger     *slog.Logger
	reporter   shared.ErrorReporter
	jwt        shared.JwtService
	limiters   *RateLimiters
	ipExtract  middleware.IPExtractor
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
	reporter shared.ErrorReporter,
	jwt shared.JwtService,
	limiters *RateLimiters,
	ipExtract middleware.IPExtractor,
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
		reporter:   reporter,
		jwt:        jwt,
		limiters:   limiters,
		ipExtract:  ipExtract,
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

	// Spawn the rate-limiter janitors so idle buckets don't accumulate.
	// They stop when ctx is cancelled — same lifecycle as the listener.
	go s.limiters.Login.Run(ctx, janitorSweepInterval, janitorDropAfter)
	go s.limiters.Refresh.Run(ctx, janitorSweepInterval, janitorDropAfter)

	addr := ":" + s.config.HTTPPort
	s.logger.Info("server: starting", logKeyAddr, addr)
	return runWithShutdown(
		ctx,
		&http.Server{
			Addr:              addr,
			Handler:           chain,
			ReadHeaderTimeout: readHeaderTimeout,
			ReadTimeout:       readTimeout,
			WriteTimeout:      writeTimeout,
			IdleTimeout:       idleTimeout,
			MaxHeaderBytes:    maxHeaderBytes,
		},
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
		logger.Info("server: shutdown signal received, draining", logKeyTimeout, grace)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), grace)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("server: graceful shutdown failed", shared.LogKeyError, err)
			return err
		}
		if err := <-serverErr; err != nil {
			logger.Error(
				"server: listener exited with error during shutdown",
				shared.LogKeyError,
				err,
			)
		}
		logger.Info("server: stopped")
		return nil
	}
}

func (s *Server) registerRoutes() *http.ServeMux {
	mux := http.NewServeMux()

	// Public — no authentication required. Auth endpoints carry per-IP
	// token-bucket limits; everything else relies on the global chain.
	loginLimit := s.limiters.Login.Middleware()
	refreshLimit := s.limiters.Refresh.Middleware()
	mux.HandleFunc("GET /health", s.health.Check)
	mux.Handle("POST /api/v1/auth/login", loginLimit(http.HandlerFunc(s.auth.Login)))
	mux.Handle("POST /api/v1/auth/refresh", refreshLimit(http.HandlerFunc(s.auth.Refresh)))

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

	// Order: Trace → Recovery → IP → Security headers → CORS → CSRF →
	// Logging (→ handler). HSTS is only emitted in production (gated on the
	// CookieSecure flag, which already distinguishes HTTPS traffic).
	// Recovery sits just inside Trace so trace_id is in ctx while it still
	// wraps every other middleware. IPMiddleware runs early so every
	// downstream consumer (audit, logging) sees the same resolved IP.
	middlewares := []func(http.Handler) http.Handler{
		middleware.TraceMiddleware(),
		middleware.RecoveryMiddleware(s.logger, s.reporter),
		middleware.IPMiddleware(s.ipExtract),
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
