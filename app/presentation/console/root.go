package console

import (
	"github.com/spf13/cobra"
)

type RootCommand struct {
	cmd      *cobra.Command
	serveCmd *ServeCommand
}

func NewRootCommand(serveCmd *ServeCommand) *RootCommand {
	root := &RootCommand{
		serveCmd: serveCmd,
	}

	root.cmd = &cobra.Command{
		Use:     "app",
		Short:   "Go skeleton application",
		Version: "0.1.0",
	}

	root.cmd.AddCommand(serveCmd.Command())

	return root
}

func (r *RootCommand) Execute() error {
	return r.cmd.Execute()
}
