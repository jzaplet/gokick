package console

import (
	"context"
	"fmt"

	"gokick/app/application/bus"
	platformcmd "gokick/app/application/platform/command"

	"github.com/spf13/cobra"
)

// CreateSuperAdminCommand wraps the platform CreateSuperAdminHandler as a CLI
// command. Like the other create-* commands it dispatches through the
// SystemCommandBus (operator-trusted: no Authorize/Tenant), which wraps the write
// in a transaction + audit + panic→Sentry. It is the sanctioned way to mint a
// superadmin on a server, since the HTTP/admin path deliberately refuses the role.
type CreateSuperAdminCommand struct {
	handler *platformcmd.CreateSuperAdminHandler
	sysBus  *bus.SystemCommandBus
}

func NewCreateSuperAdminCommand(
	handler *platformcmd.CreateSuperAdminHandler,
	sysBus *bus.SystemCommandBus,
) *CreateSuperAdminCommand {
	return &CreateSuperAdminCommand{handler: handler, sysBus: sysBus}
}

func (c *CreateSuperAdminCommand) Command() *cobra.Command {
	var nickname, password, email string

	cmd := &cobra.Command{
		Use:   "create-superadmin",
		Short: "Create a platform superadmin account (cross-tenant access)",
		Example: "  app create-superadmin -n root -p secret12\n" +
			"  app create-superadmin -n ops -p secret12 -e ops@example.com",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.run(cmd.Context(), nickname, password, email)
		},
	}

	cmd.Flags().StringVarP(&nickname, "nickname", "n", "", "nickname (required)")
	cmd.Flags().StringVarP(&password, "password", "p", "", "password (required)")
	cmd.Flags().StringVarP(&email, "email", "e", "", "email (optional)")

	_ = cmd.MarkFlagRequired("nickname")
	_ = cmd.MarkFlagRequired("password")

	return cmd
}

func (c *CreateSuperAdminCommand) run(ctx context.Context, nickname, password, email string) error {
	cmd := platformcmd.CreateSuperAdminCommand{
		Nickname: nickname,
		Password: password,
		Email:    email,
	}

	err := bus.SystemDispatchVoid(
		ctx,
		c.sysBus,
		"CreateSuperAdmin",
		cmd,
		func(ctx context.Context) error {
			return c.handler.Handle(ctx, cmd)
		},
	)
	if err != nil {
		return err
	}

	fmt.Printf("superadmin %q created\n", nickname)

	return nil
}
