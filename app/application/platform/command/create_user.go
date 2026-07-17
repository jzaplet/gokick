package command

import (
	"context"

	"gokick/app/application/userwrite"
	"gokick/app/domain/shared"
	"gokick/app/domain/tenant"
	"gokick/app/domain/user"
)

// CreatePlatformUserCommand creates a user in ANY tenant (superadmin plane).
//
// The one real difference from the admin twin: the tenant is CHOSEN, not
// inherited. An admin creates users in their own tenant, so their handler reads
// the tenant the bus resolved into ctx; a superadmin spans every tenant, so
// theirs arrives as a form field and ctx's tenant is irrelevant.
type CreatePlatformUserCommand struct {
	Nickname string
	Password string
	Email    string
	Role     string
	TenantID string
}

func (CreatePlatformUserCommand) RequiredPermission() string { return "platform:users:create" }

type CreatePlatformUserHandler struct {
	users       user.PlatformRepository
	tenants     tenant.Repository
	password    shared.PasswordHasher
	multitenant shared.Multitenancy
}

func NewCreatePlatformUserHandler(
	users user.PlatformRepository,
	tenants tenant.Repository,
	password shared.PasswordHasher,
	multitenant shared.Multitenancy,
) *CreatePlatformUserHandler {
	return &CreatePlatformUserHandler{
		users:       users,
		tenants:     tenants,
		password:    password,
		multitenant: multitenant,
	}
}

// Handle validates in the SAME order as the admin twin (nickname → role →
// superadmin-role → password → email → tenant) so the two planes agree on which
// ValidationError wins, then delegates the body to userwrite.Create — the shared
// create path that single-sources uniqueness, hashing, persistence and the
// announce (UserCreated event + user.created audit).
func (h *CreatePlatformUserHandler) Handle(
	ctx context.Context,
	cmd CreatePlatformUserCommand,
) error {
	nickname, err := user.NewNickname(cmd.Nickname)
	if err != nil {
		return err
	}

	role, err := user.NewRole(cmd.Role)
	if err != nil {
		return err
	}
	// Nobody mints a superadmin through the API — not even a superadmin. The only
	// paths are the CLI (create-superadmin) and the seeder, both out-of-band by
	// design. Mirrors the admin create and userwrite.Update's twin refusal.
	if role.IsSuperAdmin() {
		return &shared.ValidationError{
			Field:   "role",
			Message: "cannot assign the superadmin role",
		}
	}

	password, err := user.NewPassword(cmd.Password)
	if err != nil {
		return err
	}

	email, err := user.NewEmail(cmd.Email)
	if err != nil {
		return err
	}

	tenantID, err := h.resolveTenant(ctx, cmd.TenantID)
	if err != nil {
		return err
	}

	// SaveAcrossTenants, not Save: the superadmin's own tenant is the default one,
	// so writing into the CHOSEN tenant is a cross-tenant write and Save's scope
	// guard rejects it — correctly, since that guard is what stops an admin from
	// planting a row outside their own tenant. This plane is the sanctioned
	// exception.
	_, err = userwrite.Create(
		ctx,
		userwrite.Deps{Repo: h.users, Hasher: h.password},
		userwrite.CreateSpec{
			Nickname: nickname,
			Password: password,
			Email:    email,
			Role:     role,
			TenantID: tenantID,
		},
		h.users.SaveAcrossTenants,
	)

	return err
}

// resolveTenant turns the form's tenant_id into a tenant that provably exists.
//
// Deliberately NOT shared.RequireTenant: that helper's empty-in-multitenant case
// returns a plain error because there it means "a non-bus path forgot to resolve
// the tenant" — a bug, correctly a 500. Here the value is a form field, so an
// empty or unknown one is the operator's mistake and owes them a 400 against the
// field. Single-tenant mode keeps RequireTenant's leniency: there is exactly one
// tenant to mean.
//
// The existence check is what buys the clean 400: users.tenant_id REFERENCES
// tenants(id), so an unknown id would otherwise surface as an FK violation — a
// 500 that tells the operator nothing.
func (h *CreatePlatformUserHandler) resolveTenant(
	ctx context.Context,
	tenantID string,
) (string, error) {
	if tenantID == "" {
		if h.multitenant {
			return "", &shared.ValidationError{
				Field:   "tenant_id",
				Message: "tenant is required",
			}
		}

		return shared.DefaultTenantID, nil
	}

	t, err := h.tenants.FindByID(ctx, tenantID)
	if err != nil {
		return "", err
	}
	if t == nil {
		return "", &shared.ValidationError{Field: "tenant_id", Message: "tenant not found"}
	}

	return tenantID, nil
}
