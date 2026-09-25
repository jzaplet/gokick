package database

import (
	"context"
	"errors"
	"log/slog"

	"gokick/app/domain/shared"

	"github.com/pressly/goose/v3"
)

// Migrator applies the adapter's pending schema migrations (Up only — nothing is
// ever rolled back automatically). Application.Run calls it before every CLI
// command, so it must be idempotent: an up-to-date database is a no-op.
type Migrator interface {
	RunUp() error
}

// Migration-local structured-log keys. sloglint's no-raw-keys forbids bare
// string keys.
const (
	logKeyFrom    = "from"
	logKeyTo      = "to"
	logKeyVersion = "version"
)

// MigrateUp applies every pending migration of provider and logs what changed —
// the part of RunUp both adapters share (each builds its own goose Provider:
// dialect, migration set, locking).
func MigrateUp(ctx context.Context, provider *goose.Provider, logger *slog.Logger) error {
	before, errBefore := provider.GetDBVersion(ctx)

	if _, err := provider.Up(ctx); err != nil {
		return err
	}

	after, errAfter := provider.GetDBVersion(ctx)

	switch {
	case errBefore != nil || errAfter != nil:
		// A version read failed. The migrations themselves succeeded (Up
		// returned nil), so this is a reporting-only degradation — but don't
		// fabricate an applied-range from a swallowed 0 (that would log a phantom
		// "0 -> N" or "up to date version 0"). Surface the read failure instead.
		logger.Warn("migrations: applied, but version read failed (applied-range log skipped)",
			shared.LogKeyError, errors.Join(errBefore, errAfter))
	case after > before:
		logger.Info("migrations: applied", logKeyFrom, before, logKeyTo, after)
	default:
		logger.Info("migrations: up to date", logKeyVersion, after)
	}
	return nil
}
