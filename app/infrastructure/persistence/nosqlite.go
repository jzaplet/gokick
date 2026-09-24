//go:build nosqlite

package persistence

import (
	"log/slog"

	"gokick/app/infrastructure/config"
	"gokick/app/infrastructure/database"
)

// openSQLite in a -tags nosqlite build: the SQLite adapter is not linked, so the
// driver is refused at startup rather than silently replaced.
func openSQLite(*config.Config, *slog.Logger) (*Store, func() error, error) {
	return nil, nil, notBuilt(database.DriverSQLite)
}
