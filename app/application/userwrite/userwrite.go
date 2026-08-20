// Package userwrite holds the shared body of the admin and platform user-write
// handlers. Admin (tenant-scoped) and platform (cross-tenant) user management are
// deliberate mirrors — same validation, different reach — and were copy-paste
// duplicated, which drifted (F-023: the admin update handler lost the
// superadmin-target guard the platform one had, yielding a phantom role_changed
// audit for a 0-row no-op). Centralising the body here makes that drift impossible.
// This is the pattern-setter: the delete-user mirror pair belongs here too.
package userwrite

import (
	"context"
	"time"

	"gokick/app/domain/shared"
	"gokick/app/domain/shared/msgkey"
	"gokick/app/domain/user"
)

// Deps is the dependency pair every userwrite body needs — the repository the
// reads/writes go through and the password hasher.
type Deps struct {
	Repo   user.Repository
	Hasher shared.PasswordHasher
}

// Plane carries the two spots where the admin and platform planes legitimately
// diverge (see Update's doc): the plane-specific guard (nil = none) and the
// save closure (tenant-scoped Update vs cross-tenant UpdateAcrossTenants).
type Plane struct {
	Guard func(user.Role) error
	Save  func(context.Context, *user.User) error
}

// CreateSpec is the validated input of Create — value objects only, so an
// unvalidated string can't reach the shared body.
type CreateSpec struct {
	Nickname user.Nickname
	Password user.Password
	Email    user.Email
	Role     user.Role
	TenantID string
}

// Fields carries the mutable attributes an update command sends. An empty
// Password means "unchanged"; an empty Email means "no email".
type Fields struct {
	Nickname string
	Email    string
	Role     string
	Password string
}

// Update validates an edit, applies it to target, persists it, and records the
// role-change audit — the shared body of UpdateUserHandler (admin) and
// UpdatePlatformUserHandler (platform). The two planes diverge in exactly two
// spots, both supplied by the caller:
//
//   - save: the tenant-scoped user.Repository.Update vs the cross-tenant
//     PlatformRepository.UpdateAcrossTenants (the methods differ in name, so an
//     interface can't dispatch them — a closure is the honest seam).
//   - guard: a plane-specific check run once the new role is parsed (admin passes
//     a self-demote guard; platform passes nil).
//
// Reads (FindByNickname) go through user.Repository, which the platform repo also
// satisfies (PlatformRepository embeds it). The caller loads target first
// (FindByID), so a missing user is already a not-found before we get here.
//
// The validation ORDER is load-bearing — it fixes which ValidationError wins, so
// it stays in lockstep with the handlers' original behaviour: superadmin-target →
// nickname → role → superadmin-role → guard → email → nickname-conflict →
// password → save → audit. target.Active is deliberately never touched (the edit
// form carries no active flag, so a missing field must not deactivate the user).
func Update(
	ctx context.Context,
	d Deps,
	target *user.User,
	f Fields,
	plane Plane,
) error {
	// A superadmin (platform) account is managed out-of-band, never via the API.
	// The repo write also excludes superadmin rows; this refuses up front with a
	// clean 403 instead of a 0-row no-op + phantom audit (F-023).
	if user.Role(target.Role).IsSuperAdmin() {
		return &shared.PermissionError{Key: msgkey.PermissionSuperadminImmutable}
	}

	nickname, err := user.NewNickname(f.Nickname)
	if err != nil {
		return err
	}

	role, err := user.NewRole(f.Role)
	if err != nil {
		return err
	}
	// Nobody may promote anyone TO superadmin via the API (self-escalation to the
	// platform plane). Existing superadmins are already refused above.
	if role.IsSuperAdmin() {
		return &shared.ValidationError{
			Field: "role",
			Key:   msgkey.UserSuperadminRoleUnassignable,
		}
	}

	// Plane-specific guard (admin: block self-demote), run once the new role is
	// known and before any further work — preserves the admin handler's order.
	if plane.Guard != nil {
		if err := plane.Guard(role); err != nil {
			return err
		}
	}

	email, err := user.NewEmail(f.Email)
	if err != nil {
		return err
	}

	if err := ensureNicknameFree(ctx, d.Repo, string(nickname), target); err != nil {
		return err
	}

	if f.Password != "" {
		hash, err := user.HashNewPassword(f.Password, d.Hasher)
		if err != nil {
			return err
		}
		target.PasswordHash = hash
	}

	roleChanged := target.Role != string(role)

	target.Nickname = string(nickname)
	target.Email = string(email)
	target.Role = string(role)
	target.UpdatedAt = time.Now()

	if err := plane.Save(ctx, target); err != nil {
		return err
	}

	if roleChanged {
		shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{
			Action:     "user.role_changed",
			TargetType: "user",
			TargetID:   target.ID,
			Metadata:   map[string]any{"new_role": target.Role},
		})
	}

	return nil
}

// Create is the shared body of the API-facing user-creation paths (admin
// CreateUser and platform CreatePlatformUser). Each caller validates the raw
// inputs into value objects in ITS OWN order (so per-handler ValidationError
// precedence is preserved) and decides the role + tenant, then hands them here.
//
// Create REFUSES the superadmin role, and that refusal is a floor, not a
// courtesy. Both handlers also refuse it up front — deliberately, because their
// order fixes which ValidationError wins (a superadmin role beats a bad password)
// and only they know that order. But a per-handler habit is not an invariant: a
// fourth create path (invite-accept, self-signup, an import) would ship with no
// refusal and nothing would fail. Update already refuses in this body for the same
// reason; Create now matches it. The one sanctioned minter names itself —
// CreateSuperAdmin below. This is the same defence-in-depth the repo layer applies
// to the update/delete twins, which re-refuse superadmin rows in the statement.
func Create(
	ctx context.Context,
	d Deps,
	spec CreateSpec,
	save func(context.Context, *user.User) error,
) (*user.User, error) {
	// Before the uniqueness lookup, so the error precedence matches the handlers
	// that already refused up front: role beats a taken nickname, either way in.
	if spec.Role.IsSuperAdmin() {
		return nil, &shared.ValidationError{
			Field: "role",
			Key:   msgkey.UserSuperadminRoleUnassignable,
		}
	}

	return create(ctx, d, spec, save)
}

// CreateSuperAdmin mints a superadmin: the ONE sanctioned exception to Create's
// refusal, and the reason that refusal can be a default instead of a habit.
// Reachable only out-of-band — the CLI's create-superadmin over the
// SystemCommandBus. (The seeder's superadmin never comes through here: it writes
// via the repository, so it is untouched by Create's refusal.)
//
// It stamps the role itself so the ENTRY POINT decides, not the call site — but it
// refuses a spec that asked for a different one rather than silently overwriting
// it. Silence would matter because this shares Create's exact signature: the two
// are interchangeable wherever one is held in a variable, so a spec that says
// RoleUser flowing into the wrong branch would come out a superadmin with nothing
// objecting. An unset Role is the ordinary case (the caller has nothing to say);
// an explicit superadmin is fine; anything else is a caller that believes it is
// creating something this function does not create.
func CreateSuperAdmin(
	ctx context.Context,
	d Deps,
	spec CreateSpec,
	save func(context.Context, *user.User) error,
) (*user.User, error) {
	if spec.Role != "" && !spec.Role.IsSuperAdmin() {
		return nil, &shared.ValidationError{
			Field:  "role",
			Key:    msgkey.UserSuperadminCreateWrongRole,
			Params: map[string]any{"role": string(spec.Role)},
		}
	}
	spec.Role = user.RoleSuperAdmin

	return create(ctx, d, spec, save)
}

// create is the body both entry points share: it enforces nickname uniqueness,
// hashes the password (AFTER the uniqueness check, so a taken nickname skips the
// bcrypt cost), persists the user, and announces it ONCE — a user.UserCreated
// event + a user.created audit record. Single-sourcing the announcement is the
// point (F-031): superadmin creation previously skipped the event, and a
// copy-paste body is exactly what let that drift.
//
// save is the same seam Update's Plane.Save is, and for the same reason: the
// tenant-scoped Save and the cross-tenant SaveAcrossTenants differ in NAME, so no
// interface can dispatch between them — a closure is the honest seam. The admin
// and CLI paths pass Save (a row must be born in the active tenant); the platform
// path passes SaveAcrossTenants, because a superadmin's own tenant is the default
// one and the chosen tenant is the entire point.
//
// The uniqueness check deliberately stays on d.Repo: FindByNickname is a global
// identity lookup (nickname is UNIQUE across the whole table), so it must reach
// across tenants on every plane — a nickname taken in tenant A has to collide with
// a platform create into tenant B.
func create(
	ctx context.Context,
	d Deps,
	spec CreateSpec,
	save func(context.Context, *user.User) error,
) (*user.User, error) {
	existing, err := d.Repo.FindByNickname(ctx, string(spec.Nickname))
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, &shared.ValidationError{
			Field: "nickname",
			Key:   msgkey.UserNicknameTaken,
		}
	}

	hash, err := d.Hasher.Hash(string(spec.Password))
	if err != nil {
		return nil, err
	}

	u := user.NewUser(spec.Nickname, hash, spec.Email, spec.Role, spec.TenantID)
	if err := save(ctx, u); err != nil {
		return nil, err
	}

	// TenantID comes off the ROW, never off ctx: the platform create runs as a
	// superadmin whose active tenant is the default one, not the tenant the new
	// user lands in. See the UserCreated doc.
	shared.EventCollectorFromContext(ctx).Collect(user.UserCreated{
		UserID:    u.ID,
		Nickname:  u.Nickname,
		Email:     u.Email,
		Role:      u.Role,
		TenantID:  u.TenantID,
		Timestamp: time.Now(),
	})
	// tenant_id rides in the metadata for the same reason UserCreated carries it:
	// on the platform plane the actor's tenant and the target's tenant are
	// different, and this is the one write that can plant a user in ANY tenant.
	// The middleware enriches the record with the actor and their IP, and
	// AuditRecord has no tenant column — so without this the trail says "superadmin
	// S created user U with role=admin" and nothing, anywhere, says which tenant U
	// was born in. Off the ROW, never off ctx (see the UserCreated doc).
	shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{
		Action:     "user.created",
		TargetType: "user",
		TargetID:   u.ID,
		Metadata:   map[string]any{"role": u.Role, "tenant_id": u.TenantID},
	})

	return u, nil
}

// ensureNicknameFree rejects a rename that collides with another user's nickname
// (nickname is globally unique). An unchanged nickname is a no-op.
func ensureNicknameFree(
	ctx context.Context,
	repo user.Repository,
	nickname string,
	target *user.User,
) error {
	if nickname == target.Nickname {
		return nil
	}
	conflict, err := repo.FindByNickname(ctx, nickname)
	if err != nil {
		return err
	}
	if conflict != nil && conflict.ID != target.ID {
		return &shared.ValidationError{
			Field: "nickname",
			Key:   msgkey.UserNicknameTaken,
		}
	}
	return nil
}
