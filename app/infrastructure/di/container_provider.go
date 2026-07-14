//go:build wireinject

package di

import (
	"fmt"
	app "gokick/app"
	authcmd "gokick/app/application/auth/command"
	"gokick/app/application/bus"
	busmw "gokick/app/application/bus/middleware"
	dashboardqry "gokick/app/application/dashboard/query"
	platformcmd "gokick/app/application/platform/command"
	platformqry "gokick/app/application/platform/query"
	profilecmd "gokick/app/application/profile/command"
	profileqry "gokick/app/application/profile/query"
	runapp "gokick/app/application/run"
	tenantcmd "gokick/app/application/tenant/command"
	tenantqry "gokick/app/application/tenant/query"
	usercmd "gokick/app/application/user/command"
	userqry "gokick/app/application/user/query"
	"gokick/app/domain/run"
	"gokick/app/domain/shared"
	"gokick/app/domain/tenant"
	"gokick/app/domain/token"
	"gokick/app/domain/user"
	"gokick/app/infrastructure/config"
	"gokick/app/infrastructure/database"
	"gokick/app/infrastructure/scheduler"
	"gokick/app/infrastructure/security"
	sqliteaudit "gokick/app/infrastructure/sqlite/audit"
	sqliterun "gokick/app/infrastructure/sqlite/run"
	sqliteseeder "gokick/app/infrastructure/sqlite/seeder"
	sqlitetenant "gokick/app/infrastructure/sqlite/tenant"
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

	"time"

	"github.com/google/wire"
)

func providePasswordHasher() shared.PasswordHasher {
	return security.NewPasswordHasher()
}

func providePermissionChecker() shared.PermissionChecker {
	return security.NewPermissionChecker()
}

// provideTenantResolver wires the single-tenant default resolver (multitenancy
// off). Swapping this binding is how a multi-tenant deployment turns isolation
// on without touching handlers.
func provideTenantResolver() shared.TenantResolver {
	return security.NewDefaultTenantResolver()
}

// provideCommandBus wires the write-side bus from busmw.CommandChain (the single
// source of the chain order, shared with testfx so they can't drift).
func provideCommandBus(
	logger *slog.Logger,
	db *database.SqliteManager,
	checker shared.PermissionChecker,
	eventBus *bus.EventBus,
	runDispatcher shared.RunDispatcher,
	audit shared.AuditLogger,
	reporter shared.ErrorReporter,
	tenantResolver shared.TenantResolver,
) *bus.CommandBus {
	return bus.NewCommandBus(
		busmw.CommandChain(
			logger,
			checker,
			reporter,
			tenantResolver,
			audit,
			runDispatcher,
			eventBus,
			db,
		)...,
	)
}

// provideSystemCommandBus wires the OPERATOR-TRUSTED write bus for the CLI
// create-* commands. It is the CommandBus chain MINUS Authorize and Tenant (no
// principal, no JWT-resolved tenant). Audit still wraps OUTSIDE Transaction and
// DispatchEvents still wraps it, and RunDispatcher sits between them so an event
// handler on this bus can durably enqueue a follow-up run (F-008). See
// bus.SystemCommandBus.
func provideSystemCommandBus(
	logger *slog.Logger,
	db *database.SqliteManager,
	eventBus *bus.EventBus,
	audit shared.AuditLogger,
	runDispatcher shared.RunDispatcher,
	reporter shared.ErrorReporter,
) *bus.SystemCommandBus {
	return bus.NewSystemCommandBus(
		busmw.SystemChain(logger, db, eventBus, audit, runDispatcher, reporter)...,
	)
}

func provideQueryBus(
	logger *slog.Logger,
	checker shared.PermissionChecker,
	reporter shared.ErrorReporter,
	tenantResolver shared.TenantResolver,
) *bus.QueryBus {
	return bus.NewQueryBus(busmw.BaseChain(logger, checker, reporter, tenantResolver)...)
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

// provideSeedSuperAdminPassword surfaces the OPTIONAL superadmin seed password
// as its own Wire-bound type. Empty = no superadmin seeded.
func provideSeedSuperAdminPassword(cfg *config.Config) sqliteseeder.SeedSuperAdminPassword {
	return sqliteseeder.SeedSuperAdminPassword(cfg.SeedSuperAdminPassword)
}

// provideSeedAdminTenant / provideMultitenant surface the seeder's tenant inputs
// as Wire-distinct types so it gets the specific values, not the whole config.
func provideSeedAdminTenant(cfg *config.Config) sqliteseeder.SeedAdminTenant {
	return sqliteseeder.SeedAdminTenant(cfg.SeedAdminTenant)
}

func provideMultitenant(cfg *config.Config) sqliteseeder.Multitenant {
	return sqliteseeder.Multitenant(cfg.Multitenancy)
}

// provideMultitenancy surfaces APP_MULTITENANCY to application-layer constructors
// (the create-user handler) that fail-close tenant resolution but cannot import
// infrastructure. shared.Multitenancy is the Wire-distinct domain type.
func provideMultitenancy(cfg *config.Config) shared.Multitenancy {
	return shared.Multitenancy(cfg.Multitenancy)
}

// provideSchedulerJobs is the single source of truth for periodic in-process
// jobs — mirrors providePermissionsRegistry. Add
// a new Job here; provideScheduler stays decoupled from job business.
func provideSchedulerJobs(tokens token.Repository) []scheduler.Job {
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

// provideRunHandlerRegistry collects every kind → durable-run handler the binary
// can process. Real handlers are registered here as they appear. When APP_RUN_DEBUG
// is on it also registers the e2e:* debug kinds (the crash-recovery / drain
// harness) — never in production. The default lease comes from config so an unset
// per-kind lease stays consistent.
func provideRunHandlerRegistry(cfg *config.Config) (*runapp.HandlerRegistry, error) {
	kinds := map[string]runapp.Registration{}
	if cfg.RunDebug {
		for kind, reg := range runapp.E2EDebugRegistrations() {
			kinds[kind] = reg
		}
	}
	return runapp.NewHandlerRegistry(kinds, cfg.RunWorkerLease)
}

// provideRunDispatcher returns the durable-run dispatcher as a domain interface so
// command/event handlers depend on shared.RunDispatcher, not the concrete type.
func provideRunDispatcher(
	repo run.Repository,
	registry *runapp.HandlerRegistry,
) shared.RunDispatcher {
	return runapp.NewDispatcher(repo, registry)
}

// provideRunWorker wires the durable run worker (the one background-work engine) from
// config. It injects the run dispatcher (so a handler can enqueue a child task), the
// SqliteManager as the Transactor backing shared.WithTx (short atomic writes the handler
// scopes itself), and the AuditLogger so a run handler's audit events are drained and
// persisted — the worker bypasses the bus, where these are normally injected.
func provideRunWorker(
	logger *slog.Logger,
	reporter shared.ErrorReporter,
	repo run.Repository,
	registry *runapp.HandlerRegistry,
	runDispatcher shared.RunDispatcher,
	db *database.SqliteManager,
	audit shared.AuditLogger,
	cfg *config.Config,
) *worker.RunWorker {
	return worker.NewRunWorker(
		logger,
		reporter,
		repo,
		registry,
		runDispatcher,
		db,
		audit,
		worker.RunWorkerConfig{
			DefaultLease:      cfg.RunWorkerLease,
			HeartbeatInterval: cfg.RunWorkerHeartbeat,
			PollInterval:      cfg.RunWorkerPoll,
			DrainTimeout:      cfg.RunWorkerDrainTimeout,
			MaxInFlight:       cfg.RunWorkerMaxInFlight,
			MaxReclaims:       cfg.RunWorkerMaxReclaims,
		},
	)
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
		platformqry.ListAllUsersQuery{},
		platformqry.ListTenantsQuery{},
		platformqry.GetStatsQuery{},
		platformcmd.UpdatePlatformUserCommand{},
		platformcmd.DeletePlatformUserCommand{},
		// CLI-only (shared.CLIOnly): dispatched via the SystemCommandBus, no HTTP
		// route. NewPermissionsRegistry filters them out so their permission never
		// reaches the FE-facing list. Listed here so the marker is the single
		// exclusion mechanism (drop the marker → the di outcome test fails).
		platformcmd.CreateSuperAdminCommand{},
		tenantcmd.CreateTenantCommand{},
		tenantqry.GetTenantQuery{},
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
		provideTenantResolver,
		provideCommandBus,
		provideSystemCommandBus,
		provideQueryBus,
		provideEventHandlers,
		provideEventBus,
		provideCookieSecure,
		provideIPExtractor,
		provideRateLimiters,
		provideSeedAdminPassword,
		provideSeedSuperAdminPassword,
		provideSeedAdminTenant,
		provideMultitenant,
		provideMultitenancy,
		provideSchedulerJobs,
		provideScheduler,
		provideRunHandlerRegistry,
		provideRunDispatcher,
		provideRunWorker,
		providePermissionsRegistry,
		security.NewJwtService,
		wire.Bind(new(shared.TokenService), new(*security.JwtService)),
		wire.Bind(new(user.Repository), new(*sqliteuser.Repository)),
		wire.Bind(new(user.PlatformRepository), new(*sqliteuser.Repository)),
		wire.Bind(new(token.Repository), new(*sqlitetoken.Repository)),
		wire.Bind(new(run.Repository), new(*sqliterun.Repository)),
		wire.Bind(new(tenant.Repository), new(*sqlitetenant.Repository)),
		wire.Bind(new(tenant.PlatformRepository), new(*sqlitetenant.Repository)),
		wire.Bind(new(shared.Seeder), new(*sqliteseeder.Seeder)),
		wire.Bind(new(shared.AuditLogger), new(*sqliteaudit.Repository)),
		sqliteuser.NewRepository,
		sqlitetoken.NewRepository,
		sqliterun.NewRepository,
		sqlitetenant.NewRepository,
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
		platformqry.NewListAllUsersHandler,
		platformqry.NewListTenantsHandler,
		platformqry.NewGetStatsHandler,
		platformcmd.NewUpdatePlatformUserHandler,
		platformcmd.NewDeletePlatformUserHandler,
		platformcmd.NewCreateSuperAdminHandler,
		tenantcmd.NewCreateTenantHandler,
		tenantqry.NewGetTenantHandler,
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
		handler.NewPlatformHandler,
		handler.NewDebugRunHandler,
		server.NewServer,
		console.NewServeCommand,
		console.NewSeedCommand,
		console.NewCreateUserCommand,
		console.NewCreateSuperAdminCommand,
		console.NewCreateTenantCommand,
		console.NewWorkerCommand,
		console.NewRootCommand,
		app.NewApplication,
	)
	return nil, nil
}
