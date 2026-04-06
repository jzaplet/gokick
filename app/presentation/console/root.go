package console

import (
	"github.com/spf13/cobra"
)

type RootCommand struct {
	cmd      *cobra.Command
	serveCmd *ServeCommand
	seedCmd  *SeedCommand
}

func NewRootCommand(serveCmd *ServeCommand, seedCmd *SeedCommand) *RootCommand {
	root := &RootCommand{
		serveCmd: serveCmd,
		seedCmd:  seedCmd,
	}

	root.cmd = &cobra.Command{
		Use:     "app",
		Short:   "Golang skeleton application",
		Version: "0.1.0",
	}

	root.cmd.AddCommand(serveCmd.Command())
	root.cmd.AddCommand(seedCmd.Command())

	return root
}

func (r *RootCommand) Execute() error {
	return r.cmd.Execute()
}
