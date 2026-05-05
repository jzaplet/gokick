package console

import (
	"context"
	"fmt"

	usercmd "gokick/app/application/user/command"

	"github.com/spf13/cobra"
)

// CreateUserCommand wraps the application-layer CreateUserHandler as a CLI
// command. It bypasses the bus (no auth context, no transaction wrapping is
// needed for a single-statement insert) and reuses the same validation +
// hashing path the HTTP API uses.
type CreateUserCommand struct {
	handler *usercmd.CreateUserHandler
}

func NewCreateUserCommand(handler *usercmd.CreateUserHandler) *CreateUserCommand {
	return &CreateUserCommand{handler: handler}
}

func (c *CreateUserCommand) Command() *cobra.Command {
	var nickname, password, email, role string

	cmd := &cobra.Command{
		Use:   "create-user",
		Short: "Create a user (any role — defaults to admin)",
		Example: "  app create-user -n alice -p secret12\n" +
			"  app create-user -n bob -p secret12 -e bob@example.com -r user",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.run(cmd.Context(), nickname, password, email, role)
		},
	}

	cmd.Flags().StringVarP(&nickname, "nickname", "n", "", "nickname (required)")
	cmd.Flags().StringVarP(&password, "password", "p", "", "password (required)")
	cmd.Flags().StringVarP(&email, "email", "e", "", "email (optional)")
	cmd.Flags().StringVarP(&role, "role", "r", "admin", "role (admin or user)")

	_ = cmd.MarkFlagRequired("nickname")
	_ = cmd.MarkFlagRequired("password")

	return cmd
}

func (c *CreateUserCommand) run(ctx context.Context, nickname, password, email, role string) error {
	if err := c.handler.Handle(ctx, usercmd.CreateUserCommand{
		Nickname: nickname,
		Password: password,
		Email:    email,
		Role:     role,
	}); err != nil {
		return err
	}

	fmt.Printf("user %q (%s) created\n", nickname, role)

	return nil
}
