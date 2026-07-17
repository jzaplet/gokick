package console

import (
	"context"
	"errors"
	"fmt"

	"gokick/app/application/bus"
	platformcmd "gokick/app/application/platform/command"
	platformqry "gokick/app/application/platform/query"
	usercmd "gokick/app/application/user/command"
	"gokick/app/domain/shared"
	"gokick/app/domain/user"
	"gokick/app/infrastructure/config"

	"github.com/spf13/cobra"
)

// CreateUserCommand wraps the application-layer CreateUserHandler as a CLI
// command. It dispatches through the SystemCommandBus (operator-trusted: no
// Authorize/Tenant middleware), so — unlike the HTTP path where TenantMiddleware
// resolves the caller's tenant — the tenant must be given explicitly when
// multitenancy is on (--tenant-id / --tenant-name), otherwise the user would
// silently land in the default tenant. The bus supplies the transaction (atomic
// tenant+user create), audit and panic→Sentry that a bare handler call would not.
type CreateUserCommand struct {
	createUser   *usercmd.CreateUserHandler
	createTenant *platformcmd.CreateTenantHandler
	getTenant    *platformqry.GetTenantHandler
	config       *config.Config
	sysBus       *bus.SystemCommandBus
}

func NewCreateUserCommand(
	createUser *usercmd.CreateUserHandler,
	createTenant *platformcmd.CreateTenantHandler,
	getTenant *platformqry.GetTenantHandler,
	cfg *config.Config,
	sysBus *bus.SystemCommandBus,
) *CreateUserCommand {
	return &CreateUserCommand{
		createUser:   createUser,
		createTenant: createTenant,
		getTenant:    getTenant,
		config:       cfg,
		sysBus:       sysBus,
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

	// Fail fast on bad input (and the superadmin role create-user refuses) before
	// touching the database — cheaper than creating a tenant and rolling it back.
	if err := validateUserInput(a.nickname, a.password, a.email, a.role); err != nil {
		return err
	}

	cmd := usercmd.CreateUserCommand{
		Nickname: a.nickname,
		Password: a.password,
		Email:    a.email,
		Role:     a.role,
	}

	// Dispatch through the system bus: its TransactionMiddleware wraps tenant
	// resolution + user creation in ONE transaction, so a failed create rolls back
	// a just-created --tenant-name tenant (no orphan) without a hand-rolled tx;
	// AuditMiddleware persists the user.created record; RecoveryMiddleware reports
	// a panic to Sentry. The chain has no Authorize/Tenant — the operator is
	// trusted and the tenant is injected explicitly inside the unit of work.
	err := bus.SystemDispatchVoid(
		ctx,
		c.sysBus,
		"CreateUser",
		cmd,
		func(ctx context.Context) error {
			tenantID, err := c.resolveTenant(ctx, a.tenantID, a.tenantName)
			if err != nil {
				return err
			}
			if tenantID != "" {
				ctx = shared.ContextWithTenantID(ctx, tenantID)
			}
			return c.createUser.Handle(ctx, cmd)
		},
	)
	if err != nil {
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
		t, err := c.getTenant.Handle(ctx, platformqry.GetTenantQuery{ID: tenantID})
		if err != nil {
			return "", err
		}
		if t == nil {
			return "", fmt.Errorf("tenant %q not found", tenantID)
		}
		return t.ID, nil
	}
	if tenantName != "" {
		t, err := c.createTenant.Handle(ctx, platformcmd.CreateTenantCommand{Name: tenantName})
		if err != nil {
			return "", err
		}
		return t.ID, nil
	}
	return "", nil
}

// validateUserInput is a DELIBERATE lightweight mirror of CreateUserHandler's
// value-object validation, run as a CLI pre-flight so bad input fails before the
// bus creates a --tenant-name tenant. It is an OPTIMISATION, not the safety net:
// the handler re-validates authoritatively, and the bus transaction — not this —
// is what guarantees no orphan tenant on failure (run's dispatch comment). So it
// need not track the handler's field ORDER (the CLI prints one error, no FE field
// routing); if a new rule is added to the handler and missed here, the only cost
// is a tenant create+rollback, never a correctness gap.
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
