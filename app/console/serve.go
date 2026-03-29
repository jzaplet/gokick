package console

import (
	"myapp/app/http/server"

	"github.com/spf13/cobra"
)

type ServeCommand struct {
	server *server.Server
}

func NewServeCommand(server *server.Server) *ServeCommand {
	return &ServeCommand{server: server}
}

func (c *ServeCommand) Command() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Spustí HTTP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.server.Start()
		},
	}
}
