package console

import (
	"context"

	"gokick/app/infrastructure/scheduler"
	"gokick/app/infrastructure/worker"
	"gokick/app/presentation/http/server"

	"github.com/spf13/cobra"
)

type ServeCommand struct {
	server    *server.Server
	scheduler *scheduler.Scheduler
	runWorker *worker.RunWorker
}

func NewServeCommand(
	server *server.Server,
	scheduler *scheduler.Scheduler,
	runWorker *worker.RunWorker,
) *ServeCommand {
	return &ServeCommand{
		server:    server,
		scheduler: scheduler,
		runWorker: runWorker,
	}
}

func (c *ServeCommand) Command() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP server with in-process scheduler and durable-task worker",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Wrap the signal-handler ctx so we can also cancel on a non-SIGTERM
			// server.Start failure (e.g. port bind) — otherwise scheduler/worker
			// would hang on a healthy ctx and the process would never exit.
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			schedulerDone := make(chan struct{})
			go func() {
				defer close(schedulerDone)
				c.scheduler.Run(ctx)
			}()

			runWorkerDone := make(chan struct{})
			go func() {
				defer close(runWorkerDone)
				c.runWorker.Run(ctx)
			}()

			serverErr := c.server.Start(ctx)
			cancel()
			<-schedulerDone
			<-runWorkerDone
			return serverErr
		},
	}
}
