//go:build wireinject

package di

import (
	"log/slog"
	app "myapp/app"
	"myapp/app/bus"
	"myapp/app/bus/bus_middleware"
	"myapp/app/config"
	"myapp/app/console"
	"myapp/app/database"
	"myapp/app/domain/shared"
	"myapp/app/http/handler"
	"myapp/app/http/server"

	"github.com/google/wire"
)

func provideEventCollector() *shared.EventCollector {
	return shared.NewEventCollector()
}

func provideCommandBus(logger *slog.Logger, db *database.SqliteManager, collector *shared.EventCollector) *bus.CommandBus {
	return &bus.CommandBus{Bus: bus.New(
		bus_middleware.RecoveryMiddleware(logger),
		bus_middleware.LoggingMiddleware(logger),
		// AuthorizeMiddleware přidáme ve fázi 5 (potřebuje PermissionChecker)
		bus_middleware.TransactionMiddleware(db),
		bus_middleware.DispatchEventsMiddleware(logger, collector),
	)}
}

func provideQueryBus(logger *slog.Logger) *bus.QueryBus {
	return &bus.QueryBus{Bus: bus.New(
		bus_middleware.RecoveryMiddleware(logger),
		bus_middleware.LoggingMiddleware(logger),
		// AuthorizeMiddleware přidáme ve fázi 5
	)}
}

func provideEventBus(logger *slog.Logger) *bus.EventBus {
	return &bus.EventBus{Bus: bus.New(
		bus_middleware.RecoveryMiddleware(logger),
		bus_middleware.LoggingMiddleware(logger),
	)}
}

func CreateApplication(logger *slog.Logger) (*app.Application, error) {
	wire.Build(
		config.LoadConfig,
		database.NewSqliteManager,
		database.NewMigrationManager,
		// Bus providers (aktivují se ve fázi 6 až budou handlery):
		// provideEventCollector,
		// provideCommandBus,
		// provideQueryBus,
		// provideEventBus,
		handler.NewHealthHandler,
		server.NewServer,
		console.NewServeCommand,
		console.NewRootCommand,
		app.NewApplication,
	)
	return nil, nil
}
