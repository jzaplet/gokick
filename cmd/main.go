package main

import (
	"context"
	"gokick/app/infrastructure/config"
	"gokick/app/infrastructure/di"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const sentryFlushTimeout = 2 * time.Second

// main is deliberately just the exit-code adapter: all real work (and every
// defer) lives in run(). os.Exit skips defers, so calling it here — after run()
// has fully unwound — is the one place it is safe. This is the fix for the
// exitAfterDefer footgun where an in-body os.Exit skipped the Sentry flush and
// the signal-stop cleanup on the startup error paths.
func main() {
	os.Exit(run())
}

func run() int {
	// The logger and error reporter are built before the full Config loads, so a
	// failure inside LoadConfig itself can still be logged and reported. They read
	// their env through config.LoadStartup (which loads .env) so cmd/ never calls
	// os.Getenv directly.
	startup := config.LoadStartup()
	sentryEnabled := startup.SentryDSN != ""
	version := releaseVersion(startup.SentryRelease)

	logger := newLogger(
		startup.LogFormat,
		parseLogLevel(startup.LogLevel),
		sentryEnabled,
	)
	slog.SetDefault(logger)
	logger.Info("starting gokick", "version", version)

	reporter, err := newErrorReporter(startup.SentryDSN, startup.SentryEnvironment, version)
	if err != nil {
		logger.Error("failed to initialize error reporter", "error", err)
		return 1
	}
	// Surface a common footgun: with a DSN but no environment, both backend
	// (default) and frontend (meta-tag fallback) tag events "development" — so
	// production errors would hide under the dev environment unless flagged.
	if sentryEnabled && startup.SentryEnvironment == "" {
		logger.Warn(
			"APP_SENTRY_ENVIRONMENT is empty — Sentry events will be tagged 'development'; set it explicitly in production",
		)
	}
	// Flush on every return path and during panic unwinding — now that exit goes
	// through run()'s return, this defer always fires (no os.Exit skips it). Flush
	// reports whether it drained within the timeout; a false means buffered panic/
	// terminal reports were dropped, so log it rather than swallow the loss.
	defer func() {
		if !reporter.Flush(sentryFlushTimeout) {
			logger.Warn("sentry flush timed out before exit; some error reports may have been dropped")
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	application, err := di.CreateApplication(logger, reporter)
	if err != nil {
		logger.Error("failed to create application", "error", err)
		return 1
	}

	if err := application.Run(ctx); err != nil {
		logger.Error("application error", "error", err)
		return 1
	}

	return 0
}
