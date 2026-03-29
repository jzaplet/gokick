package main

import (
	"log/slog"
	"myapp/app/di_container"
	"os"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	application, err := di_container.CreateApplication(logger)
	if err != nil {
		logger.Error("failed to create application", "error", err)
		os.Exit(1)
	}

	if err := application.Run(); err != nil {
		logger.Error("application error", "error", err)
		os.Exit(1)
	}
}
