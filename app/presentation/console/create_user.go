package console

import (
	"context"
	"errors"
	"fmt"

	tenantcmd "gokick/app/application/tenant/command"
	tenantqry "gokick/app/application/tenant/query"
	usercmd "gokick/app/application/user/command"
	"gokick/app/domain/shared"
	"gokick/app/domain/user"
	"gokick/app/infrastructure/config"

	"github.com/spf13/cobra"
)

// CreateUserCommand wraps the application-layer CreateUserHandler as a CLI
// command. It bypasses the bus (no auth context), so — unlike the HTTP path
// where TenantMiddleware resolves the caller's tenant — the tenant must be given
// explicitly when multitenancy is on (--tenant-id / --tenant-name), otherwise the
// user would silently land in the default tenant.
type CreateUserCommand struct {
	createUser   *usercmd.CreateUserHandler
	createTenant *tenantcmd.CreateTenantHandler
	getTenant    *tenantqry.GetTenantHandler
	config       *config.Config
}

func NewCreateUserCommand(
	createUser *usercmd.CreateUserHandler,
	createTenant *tenantcmd.CreateTenantHandler,
	getTenant *tenantqry.GetTenantHandler,
	cfg *config.Config,
) *CreateUserCommand {
	return &CreateUserCommand{
		createUser:   createUser,
		createTenant: createTenant,
		getTenant:    getTenant,
		config:       cfg,
	}
}

func (c *CreateUserCommand) Command() *cobra.Command {
	var nickname, password, email, role, tenantID, tenantName string

	cmd := &cobra.Command{
		Use:   "create-user",
		Short: "Create a user (admin or user; defaults to admin)",
		Example: "  app create-user -n alice -p secret12\n" +
			"  app create-user -n bob -p secret12 -r user\n" +
			"  app create-user -n carol -p secret12 --tenant-name Acme   # multitenant\n" +
			"  app create-user -n dave -p secret12 --tenant-id <uuid>     # multitenant",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.run(cmd.Context(), createUserArgs{
				nickname:   nickname,
				password:   password,
				email:      email,
				role:       role,
				tenantID:   tenantID,
				tenantName: tenantName,
			})
		},
	}

	cmd.Flags().StringVarP(&nickname, "nickname", "n", "", "nickname (required)")
	cmd.Flags().StringVarP(&password, "password", "p", "", "password (required)")
	cmd.Flags().StringVarP(&email, "email", "e", "", "email (optional)")
	cmd.Flags().StringVarP(&role, "role", "r", "admin", "role (admin or user)")
	cmd.Flags().StringVar(&tenantID, "tenant-id", "", "existing tenant id (multitenant)")
	cmd.Flags().StringVar(&tenantName, "tenant-name", "", "new tenant to create (multitenant)")

	_ = cmd.MarkFlagRequired("nickname")
	_ = cmd.MarkFlagRequired("password")

	return cmd
}

type createUserArgs struct {
	nickname, password, email, role, tenantID, tenantName string
}

func (c *CreateUserCommand) run(ctx context.Context, a createUserArgs) error {
	if err := c.checkTenantFlags(a.tenantID, a.tenantName); err != nil {
		return err
	}

	// Validate the user inputs BEFORE creating a tenant, so a bad input (or the
	// superadmin role, which create-user refuses) can't leave an orphan tenant
	// behind when --tenant-name would create one.
	if err := validateUserInput(a.nickname, a.password, a.email, a.role); err != nil {
		return err
	}

	tenantID, err := c.resolveTenant(ctx, a.tenantID, a.tenantName)
	if err != nil {
		return err
	}
	if tenantID != "" {
		ctx = shared.ContextWithTenantID(ctx, tenantID)
	}

	if err := c.createUser.Handle(ctx, usercmd.CreateUserCommand{
		Nickname: a.nickname,
		Password: a.password,
		Email:    a.email,
		Role:     a.role,
	}); err != nil {
		return err
	}

	fmt.Printf("user %q (%s) created\n", a.nickname, a.role)

	return nil
}

// checkTenantFlags enforces the multitenancy matrix: on → a tenant flag is
// required; off → tenant flags are not applicable; and the two are exclusive.
func (c *CreateUserCommand) checkTenantFlags(tenantID, tenantName string) error {
	hasFlag := tenantID != "" || tenantName != ""

	if tenantID != "" && tenantName != "" {
		return errors.New("specify only one of --tenant-id or --tenant-name")
	}
	if c.config.Multitenancy && !hasFlag {
		return errors.New("multitenancy is on: specify --tenant-id or --tenant-name")
	}
	if !c.config.Multitenancy && hasFlag {
		return errors.New("multitenancy is off: --tenant-id / --tenant-name are not applicable")
	}
	return nil
}

// resolveTenant returns the tenant id the user should land in (or "" for the
// single-tenant default). --tenant-id is verified to exist; --tenant-name creates
// a new tenant.
func (c *CreateUserCommand) resolveTenant(
	ctx context.Context,
	tenantID, tenantName string,
) (string, error) {
	if tenantID != "" {
		t, err := c.getTenant.Handle(ctx, tenantqry.GetTenantQuery{ID: tenantID})
		if err != nil {
			return "", err
		}
		if t == nil {
			return "", fmt.Errorf("tenant %q not found", tenantID)
		}
		return t.ID, nil
	}
	if tenantName != "" {
		t, err := c.createTenant.Handle(ctx, tenantcmd.CreateTenantCommand{Name: tenantName})
		if err != nil {
			return "", err
		}
		return t.ID, nil
	}
	return "", nil
}

func validateUserInput(nickname, password, email, role string) error {
	if _, err := user.NewNickname(nickname); err != nil {
		return err
	}
	if _, err := user.NewPassword(password); err != nil {
		return err
	}
	if _, err := user.NewEmail(email); err != nil {
		return err
	}
	r, err := user.NewRole(role)
	if err != nil {
		return err
	}
	if r.IsSuperAdmin() {
		return errors.New("create-user cannot assign the superadmin role (use create-superadmin)")
	}
	return nil
}
