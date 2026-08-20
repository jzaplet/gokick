package command

import (
	"context"

	"gokick/app/domain/shared"
	"gokick/app/domain/shared/msgkey"
	"gokick/app/domain/tenant"
)

// DeleteTenantCommand deletes one tenant (superadmin plane). A tenant is
// deletable only once it owns no users — see the handler for why that rule lives
// in three places at once.
type DeleteTenantCommand struct {
	ID string
}

func (DeleteTenantCommand) RequiredPermission() string { return "platform:tenants:delete" }

type DeleteTenantHandler struct {
	tenants tenant.PlatformRepository
}

func NewDeleteTenantHandler(
	tenants tenant.PlatformRepository,
) *DeleteTenantHandler {
	return &DeleteTenantHandler{tenants: tenants}
}

// Handle refuses the default tenant, then deletes iff the tenant owns nothing
// live — no users, and no unfinished runs.
//
// The rule is enforced at three depths on purpose, and none of them is redundant:
// the grid disables the button (a hint — it acts on a count that was already stale
// when it rendered), DeleteIfEmptyAcrossTenants makes the test and the delete one
// statement (the real gate), and users.tenant_id REFERENCES tenants(id) means the
// DB itself refuses a widowed user row (the backstop, and the reason the count must
// include superadmins even though the platform user grid disables their row actions
// — an FK does not care which role holds the reference, and the rows are listed
// either way).
//
// The backstop covers users ONLY. runs.tenant_id carries no FK, so for runs the
// statement-level test is not the middle layer of three — it is the only one there
// is. That asymmetry is why it lives in emptyTenantCond rather than here.
func (h *DeleteTenantHandler) Handle(
	ctx context.Context,
	cmd DeleteTenantCommand,
) error {
	// The default tenant is created by migration, not the factory: in
	// single-tenant mode every user belongs to it and the runs table DEFAULTs to
	// its id. Deleting it would leave rows pointing at a tenant that no longer
	// exists. It is normally non-empty (superadmins live there) and so already
	// unreachable — this refuses it by identity rather than by luck.
	if cmd.ID == shared.DefaultTenantID {
		return &shared.ValidationError{Key: msgkey.TenantDefaultUndeletable}
	}

	target, err := h.tenants.FindByID(ctx, cmd.ID)
	if err != nil {
		return err
	}
	if target == nil {
		// Deliberately FIELDLESS: the id is a path param, not a form field, so
		// there is nowhere on the grid to route an `id` key to — Responder sends a
		// fieldless ValidationError to `general`, which is the one key the FE can
		// actually read and show. Keying it to a field nothing renders meant the
		// message was silently dropped and the operator saw only a generic toast.
		return &shared.ValidationError{Key: msgkey.TenantNotFound}
	}

	deleted, err := h.tenants.DeleteIfEmptyAcrossTenants(ctx, cmd.ID)
	if err != nil {
		return err
	}
	// It exists (we just loaded it) and it is not the default tenant (refused
	// above), so it survived because it still owns something: users, unfinished
	// runs, or both. The message names both rather than picking one — the repo
	// reports a bool, and guessing "users" would send an operator staring at a
	// user_count of 0 off to delete users that are not there. Telling them apart
	// would cost a second query on a path that only runs when the grid's hint was
	// already stale; naming both is honest and free.
	if !deleted {
		return &shared.ValidationError{Key: msgkey.TenantNotEmpty}
	}

	shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{
		Action:     "tenant.deleted",
		TargetType: "tenant",
		TargetID:   cmd.ID,
		Metadata:   map[string]any{"name": target.Name},
	})

	return nil
}
