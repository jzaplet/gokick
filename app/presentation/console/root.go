package console

import (
	"context"

	"github.com/spf13/cobra"
)

type RootCommand struct {
	cmd           *cobra.Command
	serveCmd      *ServeCommand
	seedCmd       *SeedCommand
	createUserCmd *CreateUserCommand
	workerCmd     *WorkerCommand
}

func NewRootCommand(
	serveCmd *ServeCommand,
	seedCmd *SeedCommand,
	createUserCmd *CreateUserCommand,
	workerCmd *WorkerCommand,
) *RootCommand {
	root := &RootCommand{
		serveCmd:      serveCmd,
		seedCmd:       seedCmd,
		createUserCmd: createUserCmd,
		workerCmd:     workerCmd,
	}

	root.cmd = &cobra.Command{
		Use:     "app",
		Short:   "Golang skeleton application",
		Version: "0.1.0",
	}

	root.cmd.AddCommand(serveCmd.Command())
	root.cmd.AddCommand(seedCmd.Command())
	root.cmd.AddCommand(createUserCmd.Command())
	root.cmd.AddCommand(workerCmd.Command())

	return root
}

func (r *RootCommand) Execute(ctx context.Context) error {
	return r.cmd.ExecuteContext(ctx)
}
