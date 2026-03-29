package app

import (
	"myapp/app/console"
)

type Application struct {
	rootCmd *console.RootCommand
}

func NewApplication(rootCmd *console.RootCommand) *Application {
	return &Application{rootCmd: rootCmd}
}

func (a *Application) Run() error {
	return a.rootCmd.Execute()
}
