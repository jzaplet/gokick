package app

import (
	"context"
	"gokick/app/infrastructure/database"
	"gokick/app/presentation/console"
)

type Application struct {
	rootCmd    *console.RootCommand
	migrations *database.MigrationManager
}

func NewApplication(
	rootCmd *console.RootCommand,
	migrations *database.MigrationManager,
) *Application {
	return &Application{
		rootCmd:    rootCmd,
		migrations: migrations,
	}
}

func (a *Application) Run(ctx context.Context) error {
	if err := a.migrations.RunUp(); err != nil {
		return err
	}
	return a.rootCmd.Execute(ctx)
}
