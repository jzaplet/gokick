//go:build wireinject

package di

import (
	app "gokick/app"
	authcmd "gokick/app/application/auth/command"
	"gokick/app/application/bus"
	busmw "gokick/app/application/bus/middleware"
	dashboardqry "gokick/app/application/dashboard/query"
	profilecmd "gokick/app/application/profile/command"
	profileqry "gokick/app/application/profile/query"
	usercmd "gokick/app/application/user/command"
	userqry "gokick/app/application/user/query"
	"gokick/app/domain/shared"
	"gokick/app/domain/token"
	"gokick/app/domain/user"
	"gokick/app/infrastructure/config"
	"gokick/app/infrastructure/database"
	"gokick/app/infrastructure/scheduler"
	"gokick/app/infrastructure/security"
	"gokick/app/infrastructure/sqlite"
	sqlitetoken "gokick/app/infrastructure/sqlite/token"
	sqliteuser "gokick/app/infrastructure/sqlite/user"
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

func provideCommandBus(
	logger *slog.Logger,
	db *database.SqliteManager,
	checker shared.PermissionChecker,
	eventBus *bus.EventBus,
) *bus.CommandBus {
	chain := append(busmw.BaseChain(logger, checker),
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

// provideCookieSecure extracts the boolean flag so handler.NewAuthHandler
// does not need to import the config package.
func provideCookieSecure(cfg *config.Config) handler.CookieSecure {
	return handler.CookieSecure(cfg.CookieSecure)
}

// provideScheduler wires the in-process job scheduler. Add a new Job to the
// slice for every periodic task (cron-like); the scheduler runs each in its
// own goroutine and drains them on ctx cancel.
func provideScheduler(logger *slog.Logger, tokens token.TokenRepository) (*scheduler.Scheduler, error) {
	return scheduler.NewScheduler(logger, []scheduler.Job{
		{
			Name:     "cleanup:expired-refresh-tokens",
			Interval: 1 * time.Hour,
			Fn:       tokens.DeleteExpired,
		},
	})
}

// providePermissionsRegistry collects RequiredPermission() values from every
// command/query handler that is Permissioned. Adding a new handler requires
// adding it here too — there is no other permission list in the codebase.
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
		providePermissionsRegistry,
		security.NewJwtService,
		wire.Bind(new(shared.JwtService), new(*security.JwtService)),
		wire.Bind(new(user.Repository), new(*sqliteuser.Repository)),
		wire.Bind(new(token.TokenRepository), new(*sqlitetoken.Repository)),
		wire.Bind(new(shared.Seeder), new(*sqlite.Seeder)),
		sqliteuser.NewRepository,
		sqlitetoken.NewRepository,
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
		console.NewRootCommand,
		app.NewApplication,
	)
	return nil, nil
}
