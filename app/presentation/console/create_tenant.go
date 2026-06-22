package console

import (
	"context"
	"fmt"

	tenantcmd "gokick/app/application/tenant/command"

	"github.com/spf13/cobra"
)

// CreateTenantCommand is the CLI to create a tenant and print its id, so the
// operator can then pass that id to `create-user --tenant-id`. Bypasses the bus
// (operator-trusted), like the other create-* commands.
type CreateTenantCommand struct {
	handler *tenantcmd.CreateTenantHandler
}

func NewCreateTenantCommand(handler *tenantcmd.CreateTenantHandler) *CreateTenantCommand {
	return &CreateTenantCommand{handler: handler}
}

func (c *CreateTenantCommand) Command() *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:     "create-tenant",
		Short:   "Create a tenant and print its id",
		Example: "  app create-tenant -n Acme",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.run(cmd.Context(), name)
		},
	}

	cmd.Flags().StringVarP(&name, "name", "n", "", "tenant name (required)")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func (c *CreateTenantCommand) run(ctx context.Context, name string) error {
	t, err := c.handler.Handle(ctx, tenantcmd.CreateTenantCommand{Name: name})
	if err != nil {
		return err
	}

	fmt.Printf("tenant %q created (id %s)\n", t.Name, t.ID)

	return nil
}
