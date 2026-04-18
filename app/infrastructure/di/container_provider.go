//go:build wireinject

package di

import (
	app "gokick/app"
	authcmd "gokick/app/application/auth/command"
	"gokick/app/application/bus"
	busmw "gokick/app/application/bus/middleware"
	profilecmd "gokick/app/application/profile/command"
	profileqry "gokick/app/application/profile/query"
	"gokick/app/domain/shared"
	"gokick/app/domain/token"
	"gokick/app/domain/user"
	"gokick/app/infrastructure/config"
	"gokick/app/infrastructure/database"
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
)

func provideEventCollector() *shared.EventCollector {
	return shared.NewEventCollector()
}

func providePasswordHasher() shared.PasswordHasher {
	return security.NewPasswordHasher()
}

func providePermissionChecker() shared.PermissionChecker {
	return security.NewPermissionChecker()
}

func provideCommandBus(
	logger *slog.Logger,
	db *database.SqliteManager,
	collector *shared.EventCollector,
	checker shared.PermissionChecker,
	eventBus *bus.EventBus,
) *bus.CommandBus {
	return bus.NewCommandBus(
		busmw.RecoveryMiddleware(logger),
		busmw.LoggingMiddleware(logger),
		busmw.AuthorizeMiddleware(checker),
		busmw.TransactionMiddleware(db),
		busmw.DispatchEventsMiddleware(logger, collector, eventBus),
	)
}

func provideQueryBus(logger *slog.Logger, checker shared.PermissionChecker) *bus.QueryBus {
	return bus.NewQueryBus(
		busmw.RecoveryMiddleware(logger),
		busmw.LoggingMiddleware(logger),
		busmw.AuthorizeMiddleware(checker),
	)
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

// providePermissionsRegistry collects RequiredPermission() values from every
// command/query handler that is Permissioned. Adding a new handler requires
// adding it here too — there is no other permission list in the codebase.
func providePermissionsRegistry() *shared.PermissionsRegistry {
	return shared.NewPermissionsRegistry([]shared.Permissioned{
		authcmd.LogoutCommand{},
		profilecmd.ChangePasswordCommand{},
		profileqry.GetProfileQuery{},
	})
}

func CreateApplication(logger *slog.Logger) (*app.Application, error) {
	wire.Build(
		config.LoadConfig,
		database.NewSqliteManager,
		database.NewMigrationManager,
		provideEventCollector,
		providePasswordHasher,
		providePermissionChecker,
		provideCommandBus,
		provideQueryBus,
		provideEventBus,
		provideCookieSecure,
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
		providePublicFS,
		handler.NewSPAHandler,
		handler.NewHealthHandler,
		handler.NewAuthHandler,
		handler.NewProfileHandler,
		server.NewServer,
		console.NewServeCommand,
		console.NewSeedCommand,
		console.NewRootCommand,
		app.NewApplication,
	)
	return nil, nil
}
