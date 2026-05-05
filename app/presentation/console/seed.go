package console

import (
	"gokick/app/domain/shared"

	"github.com/spf13/cobra"
)

type SeedCommand struct {
	seeder shared.Seeder
}

func NewSeedCommand(seeder shared.Seeder) *SeedCommand {
	return &SeedCommand{seeder: seeder}
}

func (c *SeedCommand) Command() *cobra.Command {
	return &cobra.Command{
		Use:   "seed",
		Short: "Seed the database with default data",
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.seeder.Seed(cmd.Context())
		},
	}
}
