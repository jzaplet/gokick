//go:build wireinject

package di

import (
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
	"gokick/app/infrastructure/sqlite"
	sqlitejob "gokick/app/infrastructure/sqlite/job"
	sqlitetoken "gokick/app/infrastructure/sqlite/token"
	sqliteuser "gokick/app/infrastructure/sqlite/user"
	"gokick/app/infrastructure/worker"
	"gokick/app/presentation/console"
	"gokick/app/presentation/http/handler"
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

// provideCommandBus wires the write-side bus. DispatchEvents wraps Transaction
// so a failed commit drops events. JobDispatcher sits outside Transaction so
// the dispatcher is injected before tx begin — Enqueue itself uses Conn(ctx),
// joining the transaction when called from a handler.
func provideCommandBus(
	logger *slog.Logger,
	db *database.SqliteManager,
	checker shared.PermissionChecker,
	eventBus *bus.EventBus,
	dispatcher shared.JobDispatcher,
) *bus.CommandBus {
	chain := append(busmw.BaseChain(logger, checker),
		busmw.JobDispatcherMiddleware(dispatcher),
		busmw.DispatchEventsMiddleware(logger, eventBus),
		busmw.TransactionMiddleware(db),
	)
	return bus.NewCommandBus(chain...)
}

func provideQueryBus(logger *slog.Logger, checker shared.PermissionChecker) *bus.QueryBus {
	return bus.NewQueryBus(busmw.BaseChain(logger, checker)...)
}

func providePublicFS() fs.FS {
	return public.FS
}

func provideEventBus(logger *slog.Logger) *bus.EventBus {
	return bus.NewEventBus(
		busmw.RecoveryMiddleware(logger),
		busmw.LoggingMiddleware(logger),
	)
}

func provideCookieSecure(cfg *config.Config) handler.CookieSecure {
	return handler.CookieSecure(cfg.CookieSecure)
}

func provideScheduler(logger *slog.Logger, tokens token.TokenRepository) (*scheduler.Scheduler, error) {
	return scheduler.NewScheduler(logger, []scheduler.Job{
		{
			Name:     "cleanup:expired-refresh-tokens",
			Interval: 1 * time.Hour,
			Fn:       tokens.DeleteExpired,
		},
	})
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
func provideJobDispatcher(repo job.Repository, registry *jobapp.HandlerRegistry) shared.JobDispatcher {
	return jobapp.NewDispatcher(repo, registry)
}

// provideWorker wires the persistent job worker. Concurrency stays at 1 by
// default because SQLite serializes writers (WAL: one writer at a time);
// more goroutines don't increase throughput for DB-bound handlers.
func provideWorker(
	logger *slog.Logger,
	repo job.Repository,
	registry *jobapp.HandlerRegistry,
	db *database.SqliteManager,
	dispatcher shared.JobDispatcher,
) *worker.Worker {
	return worker.NewWorker(logger, repo, registry, db, dispatcher, 1)
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

func CreateApplication(logger *slog.Logger) (*app.Application, error) {
	wire.Build(
		config.LoadConfig,
		database.NewSqliteManager,
		database.NewMigrationManager,
		providePasswordHasher,
		providePermissionChecker,
		provideCommandBus,
		provideQueryBus,
		provideEventBus,
		provideCookieSecure,
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
		wire.Bind(new(shared.Seeder), new(*sqlite.Seeder)),
		sqliteuser.NewRepository,
		sqlitetoken.NewRepository,
		sqlitejob.NewRepository,
		sqlite.NewSeeder,
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
