package command

import (
	"context"

	"gokick/app/application/userwrite"
	"gokick/app/domain/shared"
	"gokick/app/domain/shared/msgkey"
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

// No shared.Multitenancy dependency, unlike the admin twin: that one needs the
// flag to decide whether an absent tenant is fatal, because it resolves the tenant
// from ctx and the CLI can legitimately arrive without one. Here the tenant is
// always an explicit field, so the answer is the same in either mode — see
// resolveTenant.
type CreatePlatformUserHandler struct {
	users    user.PlatformRepository
	tenants  tenant.Repository
	password shared.PasswordHasher
}

func NewCreatePlatformUserHandler(
	users user.PlatformRepository,
	tenants tenant.Repository,
	password shared.PasswordHasher,
) *CreatePlatformUserHandler {
	return &CreatePlatformUserHandler{
		users:    users,
		tenants:  tenants,
		password: password,
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
			Field: "role",
			Key:   msgkey.UserSuperadminRoleUnassignable,
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
// tenant_id is required UNCONDITIONALLY — no single-tenant leniency, no default.
// It used to fall back to the default tenant when multitenancy was off, mirroring
// shared.RequireTenant. That was wrong here: the form marks the field required,
// so a silent fallback made the UI lie — you could leave the picker blank and the
// user would quietly land in the default tenant anyway. A form and its endpoint
// must give the same answer to "is this required?".
//
// Nothing is lost by dropping it. RequireTenant's leniency exists for the CLI,
// which has no picker to fill in; this endpoint always does, and the picker always
// has at least one option (the default tenant is created by migration). So an
// empty tenant_id here is never "there was nothing to choose" — it is a caller
// that skipped a field it could see, and it earns a 400 against that field.
//
// Deliberately NOT shared.RequireTenant for the same reason it returns a
// ValidationError: that helper's error is a plain one (a 500), because there an
// absent tenant means a non-bus path forgot to resolve it — a bug. Here it is a
// form field, so it is the operator's mistake and owes them a field error.
//
// The existence check is what buys the clean 400: users.tenant_id REFERENCES
// tenants(id), so an unknown id would otherwise surface as an FK violation — a
// 500 that tells the operator nothing.
func (h *CreatePlatformUserHandler) resolveTenant(
	ctx context.Context,
	tenantID string,
) (string, error) {
	if tenantID == "" {
		return "", &shared.ValidationError{
			Field: "tenant_id",
			Key:   msgkey.TenantRequired,
		}
	}

	t, err := h.tenants.FindByID(ctx, tenantID)
	if err != nil {
		return "", err
	}
	if t == nil {
		return "", &shared.ValidationError{Field: "tenant_id", Key: msgkey.TenantNotFound}
	}

	return tenantID, nil
}
