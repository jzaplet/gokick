package console

import (
	"gokick/app/infrastructure/scheduler"
	"gokick/app/presentation/http/server"

	"github.com/spf13/cobra"
)

type ServeCommand struct {
	server    *server.Server
	scheduler *scheduler.Scheduler
}

func NewServeCommand(server *server.Server, scheduler *scheduler.Scheduler) *ServeCommand {
	return &ServeCommand{server: server, scheduler: scheduler}
}

func (c *ServeCommand) Command() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			// Scheduler runs in parallel with the HTTP server; both share
			// the same ctx so a single SIGTERM drains everything in tandem.
			schedulerDone := make(chan struct{})
			go func() {
				defer close(schedulerDone)
				c.scheduler.Run(ctx)
			}()

			serverErr := c.server.Start(ctx)
			<-schedulerDone
			return serverErr
		},
	}
}
