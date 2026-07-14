package console

import (
	"context"
	"fmt"

	"gokick/app/application/bus"
	tenantcmd "gokick/app/application/tenant/command"
	"gokick/app/domain/tenant"

	"github.com/spf13/cobra"
)

// CreateTenantCommand is the CLI to create a tenant and print its id, so the
// operator can then pass that id to `create-user --tenant-id`. Dispatches through
// the SystemCommandBus (operator-trusted: no Authorize/Tenant) like the other
// create-* commands, so the write is transactional and audited.
type CreateTenantCommand struct {
	handler *tenantcmd.CreateTenantHandler
	sysBus  *bus.SystemCommandBus
}

func NewCreateTenantCommand(
	handler *tenantcmd.CreateTenantHandler,
	sysBus *bus.SystemCommandBus,
) *CreateTenantCommand {
	return &CreateTenantCommand{handler: handler, sysBus: sysBus}
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
	cmd := tenantcmd.CreateTenantCommand{Name: name}

	t, err := bus.SystemDispatch(ctx, c.sysBus, "CreateTenant", cmd,
		func(ctx context.Context) (*tenant.Tenant, error) {
			return c.handler.Handle(ctx, cmd)
		})
	if err != nil {
		return err
	}

	fmt.Printf("tenant %q created (id %s)\n", t.Name, t.ID)

	return nil
}
