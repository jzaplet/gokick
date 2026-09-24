package database

// Migrator applies the adapter's pending schema migrations (Up only — nothing is
// ever rolled back automatically). Application.Run calls it before every CLI
// command, so it must be idempotent: an up-to-date database is a no-op.
type Migrator interface {
	RunUp() error
}
