package console

import (
	"gokick/app/infrastructure/worker"

	"github.com/spf13/cobra"
)

// WorkerCommand runs the persistent background workers (the short-job worker and
// the durable-run worker) — no HTTP server, no scheduler. Use this to scale
// workers independently of the HTTP layer (one serve replica + N worker replicas)
// or to take inflight work off the serve process during high traffic.
type WorkerCommand struct {
	worker    *worker.Worker
	runWorker *worker.RunWorker
}

func NewWorkerCommand(w *worker.Worker, rw *worker.RunWorker) *WorkerCommand {
	return &WorkerCommand{worker: w, runWorker: rw}
}

func (c *WorkerCommand) Command() *cobra.Command {
	return &cobra.Command{
		Use:   "worker",
		Short: "Run the persistent job + durable-run workers (no HTTP server)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			runDone := make(chan struct{})
			go func() { defer close(runDone); c.runWorker.Run(ctx) }()
			c.worker.Run(ctx)
			<-runDone
			return nil
		},
	}
}
