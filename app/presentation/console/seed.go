package console

import (
	"context"

	"gokick/app/application/bus"
	"gokick/app/domain/shared"

	"github.com/spf13/cobra"
)

type SeedCommand struct {
	seeder shared.Seeder
	sysBus *bus.SystemCommandBus
}

func NewSeedCommand(seeder shared.Seeder, sysBus *bus.SystemCommandBus) *SeedCommand {
	return &SeedCommand{seeder: seeder, sysBus: sysBus}
}

// seedCommand is the bus dispatch marker — the seeder orchestrates several writes
// and has no command struct of its own.
type seedCommand struct{}

func (c *SeedCommand) Command() *cobra.Command {
	return &cobra.Command{
		Use:   "seed",
		Short: "Seed the database with default data",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Dispatch through the system bus so the whole bootstrap (admin tenant +
			// admin + optional superadmin) runs in ONE transaction — atomic, no
			// half-seeded DB — with the audit trail and panic→Sentry.
			return bus.ExecVoid(cmd.Context(), c.sysBus.Bus, "Seed", seedCommand{},
				func(ctx context.Context) error { return c.seeder.Seed(ctx) })
		},
	}
}
