package app

import (
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

func (a *Application) Run() error {
	if err := a.migrations.RunUp(); err != nil {
		return err
	}
	return a.rootCmd.Execute()
}
