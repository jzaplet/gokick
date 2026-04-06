//go:build wireinject

package di

import (
	"log/slog"
	app "myapp/app"
	"myapp/app/application/bus"
	busmw "myapp/app/application/bus/middleware"
	"myapp/app/domain/shared"
	"myapp/app/domain/token"
	"myapp/app/domain/user"
	"myapp/app/infrastructure/config"
	"myapp/app/infrastructure/database"
	"myapp/app/infrastructure/security"
	"myapp/app/infrastructure/sqlite"
	sqlitetoken "myapp/app/infrastructure/sqlite/token"
	sqliteuser "myapp/app/infrastructure/sqlite/user"
	"myapp/app/presentation/console"
	"myapp/app/presentation/http/handler"
	"myapp/app/presentation/http/server"

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

func provideCommandBus(logger *slog.Logger, db *database.SqliteManager, collector *shared.EventCollector, checker shared.PermissionChecker) *bus.CommandBus {
	return &bus.CommandBus{Bus: bus.New(
		busmw.RecoveryMiddleware(logger),
		busmw.LoggingMiddleware(logger),
		busmw.AuthorizeMiddleware(checker),
		busmw.TransactionMiddleware(db),
		busmw.DispatchEventsMiddleware(logger, collector),
	)}
}

func provideQueryBus(logger *slog.Logger, checker shared.PermissionChecker) *bus.QueryBus {
	return &bus.QueryBus{Bus: bus.New(
		busmw.RecoveryMiddleware(logger),
		busmw.LoggingMiddleware(logger),
		busmw.AuthorizeMiddleware(checker),
	)}
}

func provideEventBus(logger *slog.Logger) *bus.EventBus {
	return &bus.EventBus{Bus: bus.New(
		busmw.RecoveryMiddleware(logger),
		busmw.LoggingMiddleware(logger),
	)}
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
		security.NewJwtService,
		wire.Bind(new(user.Repository), new(*sqliteuser.Repository)),
		wire.Bind(new(token.TokenRepository), new(*sqlitetoken.Repository)),
		sqliteuser.NewRepository,
		sqlitetoken.NewRepository,
		sqlite.NewSeeder,
		handler.NewHealthHandler,
		server.NewServer,
		console.NewServeCommand,
		console.NewRootCommand,
		app.NewApplication,
	)
	return nil, nil
}
