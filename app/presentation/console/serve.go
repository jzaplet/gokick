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
	worker    *worker.Worker
	runWorker *worker.RunWorker
}

func NewServeCommand(
	server *server.Server,
	scheduler *scheduler.Scheduler,
	worker *worker.Worker,
	runWorker *worker.RunWorker,
) *ServeCommand {
	return &ServeCommand{
		server:    server,
		scheduler: scheduler,
		worker:    worker,
		runWorker: runWorker,
	}
}

func (c *ServeCommand) Command() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP server with in-process scheduler, job worker and run worker",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Wrap the signal-handler ctx so we can also cancel on a non-SIGTERM
			// server.Start failure (e.g. port bind) — otherwise scheduler/workers
			// would hang on a healthy ctx and the process would never exit.
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			schedulerDone := make(chan struct{})
			go func() {
				defer close(schedulerDone)
				c.scheduler.Run(ctx)
			}()

			workerDone := make(chan struct{})
			go func() {
				defer close(workerDone)
				c.worker.Run(ctx)
			}()

			runWorkerDone := make(chan struct{})
			go func() {
				defer close(runWorkerDone)
				c.runWorker.Run(ctx)
			}()

			serverErr := c.server.Start(ctx)
			cancel()
			<-schedulerDone
			<-workerDone
			<-runWorkerDone
			return serverErr
		},
	}
}
