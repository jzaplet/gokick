//go:build wireinject

package di_container

import (
	"log/slog"
	app "myapp/app"
	"myapp/app/console"
	"myapp/app/env"
	"myapp/app/handler"
	"myapp/app/server"

	"github.com/google/wire"
)

func CreateApplication(logger *slog.Logger) (*app.Application, error) {
	wire.Build(
		env.LoadConfig,
		handler.NewHealthHandler,
		server.NewServer,
		console.NewServeCommand,
		console.NewRootCommand,
		app.NewApplication,
	)
	return nil, nil
}
