package app

import (
	"gokick/app/application/bus"
	"gokick/app/domain/token"
	"gokick/app/infrastructure/database"
	"gokick/app/infrastructure/security"
	"gokick/app/presentation/console"
)

type Application struct {
	rootCmd    *console.RootCommand
	migrations *database.MigrationManager
	// Dead code – Wire vyžaduje consumer pro každý provider, jinak hlásí chybu.
	// Ve fázi 6 se tyto závislosti přesunou do command/query/HTTP handlerů
	// a z Application se odeberou.
	CommandBus *bus.CommandBus
	QueryBus   *bus.QueryBus
	EventBus   *bus.EventBus
	Jwt        *security.JwtService
	Tokens     token.TokenRepository
}

func NewApplication(
	rootCmd *console.RootCommand,
	migrations *database.MigrationManager,
	commandBus *bus.CommandBus,
	queryBus *bus.QueryBus,
	eventBus *bus.EventBus,
	jwt *security.JwtService,
	tokens token.TokenRepository,
) *Application {
	return &Application{
		rootCmd:    rootCmd,
		migrations: migrations,
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
	return a.rootCmd.Execute()
}
