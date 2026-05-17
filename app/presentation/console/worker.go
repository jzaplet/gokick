package console

import (
	"gokick/app/infrastructure/worker"

	"github.com/spf13/cobra"
)

// WorkerCommand runs only the persistent job worker — no HTTP server, no
// scheduler. Use this to scale the worker independently of the HTTP layer
// (one serve replica + N worker replicas) or to take inflight work off the
// serve process during high traffic.
type WorkerCommand struct {
	worker *worker.Worker
}

func NewWorkerCommand(w *worker.Worker) *WorkerCommand {
	return &WorkerCommand{worker: w}
}

func (c *WorkerCommand) Command() *cobra.Command {
	return &cobra.Command{
		Use:   "worker",
		Short: "Run the persistent job worker (no HTTP server)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c.worker.Run(cmd.Context())
			return nil
		},
	}
}
