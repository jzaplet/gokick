package console

import (
	"context"
	"fmt"

	platformcmd "gokick/app/application/platform/command"

	"github.com/spf13/cobra"
)

// CreateSuperAdminCommand wraps the platform CreateSuperAdminHandler as a CLI
// command. Like create-user it bypasses the bus (operator-trusted, no auth
// context). It is the sanctioned way to mint a superadmin on a server, since the
// HTTP/admin path deliberately refuses to assign that role.
type CreateSuperAdminCommand struct {
	handler *platformcmd.CreateSuperAdminHandler
}

func NewCreateSuperAdminCommand(
	handler *platformcmd.CreateSuperAdminHandler,
) *CreateSuperAdminCommand {
	return &CreateSuperAdminCommand{handler: handler}
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
	if err := c.handler.Handle(ctx, platformcmd.CreateSuperAdminCommand{
		Nickname: nickname,
		Password: password,
		Email:    email,
	}); err != nil {
		return err
	}

	fmt.Printf("superadmin %q created\n", nickname)

	return nil
}
