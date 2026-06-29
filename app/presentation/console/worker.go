package console

import (
	"gokick/app/infrastructure/worker"

	"github.com/spf13/cobra"
)

// WorkerCommand runs the persistent durable-task worker — no HTTP server, no
// scheduler. Use this to scale the worker independently of the HTTP layer (one
// serve replica + N worker replicas) or to take inflight work off the serve
// process during high traffic.
type WorkerCommand struct {
	runWorker *worker.RunWorker
}

func NewWorkerCommand(rw *worker.RunWorker) *WorkerCommand {
	return &WorkerCommand{runWorker: rw}
}

func (c *WorkerCommand) Command() *cobra.Command {
	return &cobra.Command{
		Use:   "worker",
		Short: "Run the persistent durable-task worker (no HTTP server)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c.runWorker.Run(cmd.Context())
			return nil
		},
	}
}
