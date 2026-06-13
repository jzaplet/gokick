//go:build wireinject

package di

import (
	"fmt"
	app "gokick/app"
	authcmd "gokick/app/application/auth/command"
	"gokick/app/application/bus"
	busmw "gokick/app/application/bus/middleware"
	dashboardqry "gokick/app/application/dashboard/query"
	jobapp "gokick/app/application/job"
	profilecmd "gokick/app/application/profile/command"
	profileqry "gokick/app/application/profile/query"
	usercmd "gokick/app/application/user/command"
	userqry "gokick/app/application/user/query"
	"gokick/app/domain/job"
	"gokick/app/domain/shared"
	"gokick/app/domain/token"
	"gokick/app/domain/user"
	"gokick/app/infrastructure/config"
	"gokick/app/infrastructure/database"
	"gokick/app/infrastructure/scheduler"
	"gokick/app/infrastructure/security"
	sqliteaudit "gokick/app/infrastructure/sqlite/audit"
	sqlitejob "gokick/app/infrastructure/sqlite/job"
	sqliteseeder "gokick/app/infrastructure/sqlite/seeder"
	sqlitetoken "gokick/app/infrastructure/sqlite/token"
	sqliteuser "gokick/app/infrastructure/sqlite/user"
	"gokick/app/infrastructure/worker"
	"gokick/app/presentation/console"
	"gokick/app/presentation/http/handler"
	httpmw "gokick/app/presentation/http/middleware"
	"gokick/app/presentation/http/server"
	"gokick/public"
	"io/fs"
	"log/slog"

	"github.com/google/wire"
	"time"
)

func providePasswordHasher() shared.PasswordHasher {
	return security.NewPasswordHasher()
}

func providePermissionChecker() shared.PermissionChecker {
	return security.NewPermissionChecker()
}

// provideCommandBus wires the write-side bus. Audit wraps OUTSIDE both
// DispatchEvents and Transaction so security-relevant events persist even
// when business work rolls back. JobDispatcher sits outside Transaction so
// the dispatcher is injected before tx begin — Enqueue itself uses Conn(ctx),
// joining the transaction when called from a handler.
func provideCommandBus(
	logger *slog.Logger,
	db *database.SqliteManager,
	checker shared.PermissionChecker,
	eventBus *bus.EventBus,
	dispatcher shared.JobDispatcher,
	audit shared.AuditLogger,
	reporter shared.ErrorReporter,
) *bus.CommandBus {
	chain := append(busmw.BaseChain(logger, checker, reporter),
		busmw.AuditMiddleware(logger, audit),
		busmw.JobDispatcherMiddleware(dispatcher),
		busmw.DispatchEventsMiddleware(logger, eventBus),
		busmw.TransactionMiddleware(db),
	)
	return bus.NewCommandBus(chain...)
}

func provideQueryBus(
	logger *slog.Logger,
	checker shared.PermissionChecker,
	reporter shared.ErrorReporter,
) *bus.QueryBus {
	return bus.NewQueryBus(busmw.BaseChain(logger, checker, reporter)...)
}

func providePublicFS() fs.FS {
	return public.FS
}

// provideSPAConfig narrows *config.Config down to the deployment-specific
// frontend values the SPA handler injects into index.html, keeping the handler
// layer free of an infrastructure/config import.
func provideSPAConfig(cfg *config.Config) handler.SPAConfig {
	return handler.SPAConfig{
		SentryDSN:         cfg.FrontendSentryDSN,
		SentryEnvironment: cfg.SentryEnvironment,
		SentryDebug:       cfg.SentryDebug,
	}
}

// provideEventHandlers is the single source of truth for event subscriptions
// — mirrors providePermissionsRegistry / provideSchedulerJobs. Add a new
// entry here; provideEventBus wires them up during construction.
func provideEventHandlers() []bus.EventHandlerEntry {
	return []bus.EventHandlerEntry{
		// {Event: "user.created", Handler: welcomeMailer.Handle},
	}
}

func provideEventBus(
	logger *slog.Logger,
	handlers []bus.EventHandlerEntry,
	reporter shared.ErrorReporter,
) *bus.EventBus {
	eb := bus.NewEventBus(
		busmw.RecoveryMiddleware(logger, reporter),
		busmw.LoggingMiddleware(logger),
	)
	for _, h := range handlers {
		eb.Register(h.Event, h.Handler)
	}
	return eb
}

func provideCookieSecure(cfg *config.Config) handler.CookieSecure {
	return handler.CookieSecure(cfg.CookieSecure)
}

// provideIPExtractor is the single source of truth for how a request's
// client IP is resolved — rate limiters and the audit IP middleware both
// pull from it so flipping APP_TRUST_PROXY_HEADERS keeps them in sync.
func provideIPExtractor(cfg *config.Config) httpmw.IPExtractor {
	return httpmw.NewIPExtractor(cfg.TrustProxyHeaders)
}

// provideRateLimiters parses the configured per-IP buckets at startup so a
// malformed APP_RATE_LIMIT_* fails fast instead of silently disabling
// protection.
func provideRateLimiters(
	cfg *config.Config,
	extract httpmw.IPExtractor,
	logger *slog.Logger,
) (*server.RateLimiters, error) {
	loginRule, err := httpmw.ParseRateRule(cfg.RateLimitLogin)
	if err != nil {
		return nil, fmt.Errorf("APP_RATE_LIMIT_LOGIN: %w", err)
	}
	refreshRule, err := httpmw.ParseRateRule(cfg.RateLimitRefresh)
	if err != nil {
		return nil, fmt.Errorf("APP_RATE_LIMIT_REFRESH: %w", err)
	}
	return &server.RateLimiters{
		Login:   httpmw.NewRateLimiter(loginRule, extract, logger),
		Refresh: httpmw.NewRateLimiter(refreshRule, extract, logger),
	}, nil
}

// provideSeedAdminPassword surfaces the seed admin password as a distinct
// Wire-bound type so the seeder's constructor can take it without colliding
// with other strings in the DI graph.
func provideSeedAdminPassword(cfg *config.Config) sqliteseeder.SeedAdminPassword {
	return sqliteseeder.SeedAdminPassword(cfg.SeedAdminPassword)
}

// provideSchedulerJobs is the single source of truth for periodic in-process
// jobs — mirrors providePermissionsRegistry / provideJobHandlerRegistry. Add
// a new Job here; provideScheduler stays decoupled from job business.
func provideSchedulerJobs(tokens token.TokenRepository) []scheduler.Job {
	return []scheduler.Job{
		{
			Name:     "cleanup:expired-refresh-tokens",
			Interval: 1 * time.Hour,
			Fn:       tokens.DeleteExpired,
		},
	}
}

func provideScheduler(logger *slog.Logger, jobs []scheduler.Job) (*scheduler.Scheduler, error) {
	return scheduler.NewScheduler(logger, jobs)
}

// provideJobHandlerRegistry collects every kind → handler the binary can
// process. Empty for now — handlers will be added in subsequent phases as
// real background work appears.
func provideJobHandlerRegistry() (*jobapp.HandlerRegistry, error) {
	return jobapp.NewHandlerRegistry(map[string]jobapp.HandlerFunc{})
}

// provideJobDispatcher returns the dispatcher as a domain interface so command
// handlers and event handlers depend on shared.JobDispatcher, not on the
// concrete application-layer type.
func provideJobDispatcher(
	repo job.Repository,
	registry *jobapp.HandlerRegistry,
) shared.JobDispatcher {
	return jobapp.NewDispatcher(repo, registry)
}

// provideWorker wires the persistent job worker. Concurrency stays at 1 by
// default because SQLite serializes writers (WAL: one writer at a time);
// more goroutines don't increase throughput for DB-bound handlers.
func provideWorker(
	logger *slog.Logger,
	reporter shared.ErrorReporter,
	repo job.Repository,
	registry *jobapp.HandlerRegistry,
	db *database.SqliteManager,
	dispatcher shared.JobDispatcher,
) *worker.Worker {
	return worker.NewWorker(logger, reporter, repo, registry, db, dispatcher, 1)
}

func providePermissionsRegistry() *shared.PermissionsRegistry {
	return shared.NewPermissionsRegistry([]shared.Permissioned{
		authcmd.LogoutCommand{},
		profilecmd.ChangePasswordCommand{},
		profileqry.GetProfileQuery{},
		usercmd.CreateUserCommand{},
		usercmd.UpdateUserCommand{},
		usercmd.DeleteUserCommand{},
		userqry.ListUsersQuery{},
		dashboardqry.GetUserDashboardQuery{},
		dashboardqry.GetAdminDashboardQuery{},
	})
}

func CreateApplication(
	logger *slog.Logger,
	reporter shared.ErrorReporter,
) (*app.Application, error) {
	wire.Build(
		config.LoadConfig,
		database.NewSqliteManager,
		database.NewMigrationManager,
		providePasswordHasher,
		providePermissionChecker,
		provideCommandBus,
		provideQueryBus,
		provideEventHandlers,
		provideEventBus,
		provideCookieSecure,
		provideIPExtractor,
		provideRateLimiters,
		provideSeedAdminPassword,
		provideSchedulerJobs,
		provideScheduler,
		provideJobHandlerRegistry,
		provideJobDispatcher,
		provideWorker,
		providePermissionsRegistry,
		security.NewJwtService,
		wire.Bind(new(shared.JwtService), new(*security.JwtService)),
		wire.Bind(new(user.Repository), new(*sqliteuser.Repository)),
		wire.Bind(new(token.TokenRepository), new(*sqlitetoken.Repository)),
		wire.Bind(new(job.Repository), new(*sqlitejob.Repository)),
		wire.Bind(new(shared.Seeder), new(*sqliteseeder.Seeder)),
		wire.Bind(new(shared.AuditLogger), new(*sqliteaudit.Repository)),
		sqliteuser.NewRepository,
		sqlitetoken.NewRepository,
		sqlitejob.NewRepository,
		sqliteseeder.NewSeeder,
		sqliteaudit.NewRepository,
		authcmd.NewLoginHandler,
		authcmd.NewRefreshTokenHandler,
		authcmd.NewLogoutHandler,
		profilecmd.NewChangePasswordHandler,
		profileqry.NewGetProfileHandler,
		usercmd.NewCreateUserHandler,
		usercmd.NewUpdateUserHandler,
		usercmd.NewDeleteUserHandler,
		userqry.NewListUsersHandler,
		dashboardqry.NewGetUserDashboardHandler,
		dashboardqry.NewGetAdminDashboardHandler,
		providePublicFS,
		provideSPAConfig,
		handler.NewSPAHandler,
		handler.NewHealthHandler,
		handler.NewAuthHandler,
		handler.NewProfileHandler,
		handler.NewAdminUsersHandler,
		handler.NewDashboardHandler,
		server.NewServer,
		console.NewServeCommand,
		console.NewSeedCommand,
		console.NewCreateUserCommand,
		console.NewWorkerCommand,
		console.NewRootCommand,
		app.NewApplication,
	)
	return nil, nil
}
