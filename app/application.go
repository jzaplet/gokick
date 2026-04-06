package app

import (
	"context"

	"myapp/app/application/bus"
	"myapp/app/domain/token"
	"myapp/app/infrastructure/database"
	"myapp/app/infrastructure/security"
	"myapp/app/infrastructure/sqlite"
	"myapp/app/presentation/console"
)

type Application struct {
	rootCmd    *console.RootCommand
	migrations *database.MigrationManager
	seeder     *sqlite.Seeder

	// Wired but used by command/query handlers in Phase 6.
	CommandBus *bus.CommandBus
	QueryBus   *bus.QueryBus
	EventBus   *bus.EventBus
	Jwt        *security.JwtService
	Tokens     token.TokenRepository
}

func NewApplication(
	rootCmd *console.RootCommand,
	migrations *database.MigrationManager,
	seeder *sqlite.Seeder,
	commandBus *bus.CommandBus,
	queryBus *bus.QueryBus,
	eventBus *bus.EventBus,
	jwt *security.JwtService,
	tokens token.TokenRepository,
) *Application {
	return &Application{
		rootCmd:    rootCmd,
		migrations: migrations,
		seeder:     seeder,
		CommandBus: commandBus,
		QueryBus:   queryBus,
		EventBus:   eventBus,
		Jwt:        jwt,
		Tokens:     tokens,
	}
}

func (a *Application) Run() error {
	if err := a.migrations.RunUp(); err != nil {
		return err
	}
	if err := a.seeder.Seed(context.Background()); err != nil {
		return err
	}
	return a.rootCmd.Execute()
}
