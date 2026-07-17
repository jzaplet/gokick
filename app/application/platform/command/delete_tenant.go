package command

import (
	"context"

	"gokick/app/domain/shared"
	"gokick/app/domain/tenant"
)

// DeletePlatformTenantCommand deletes one tenant (superadmin plane). A tenant is
// deletable only once it owns no users — see the handler for why that rule lives
// in three places at once.
type DeletePlatformTenantCommand struct {
	ID string
}

func (DeletePlatformTenantCommand) RequiredPermission() string { return "platform:tenants:delete" }

type DeletePlatformTenantHandler struct {
	tenants tenant.PlatformRepository
}

func NewDeletePlatformTenantHandler(
	tenants tenant.PlatformRepository,
) *DeletePlatformTenantHandler {
	return &DeletePlatformTenantHandler{tenants: tenants}
}

// Handle refuses the default tenant, then deletes iff the tenant owns no users.
//
// The emptiness rule is enforced at three depths on purpose, and none of them is
// redundant: the grid disables the button (a hint — it acts on a count that was
// already stale when it rendered), DeleteIfEmptyAcrossTenants makes the test and
// the delete one statement (the real gate), and users.tenant_id REFERENCES
// tenants(id) means the DB itself refuses a widowed row (the backstop, and the
// reason the count must include superadmins even though the platform user grid
// hides them — an FK does not care which role holds the reference).
func (h *DeletePlatformTenantHandler) Handle(
	ctx context.Context,
	cmd DeletePlatformTenantCommand,
) error {
	// The default tenant is created by migration, not the factory: in
	// single-tenant mode every user belongs to it and the runs table DEFAULTs to
	// its id. Deleting it would leave rows pointing at a tenant that no longer
	// exists. It is normally non-empty (superadmins live there) and so already
	// unreachable — this refuses it by identity rather than by luck.
	if cmd.ID == shared.DefaultTenantID {
		return &shared.ValidationError{Message: "the default tenant cannot be deleted"}
	}

	target, err := h.tenants.FindByID(ctx, cmd.ID)
	if err != nil {
		return err
	}
	if target == nil {
		//gkerrf:exempt path-param lookup - the grid reloads on failure, no form field maps id
		return &shared.ValidationError{Field: "id", Message: "tenant not found"}
	}

	deleted, err := h.tenants.DeleteIfEmptyAcrossTenants(ctx, cmd.ID)
	if err != nil {
		return err
	}
	// It exists (we just loaded it), so the only way it survived is users.
	if !deleted {
		return &shared.ValidationError{
			Message: "tenant still has users — remove them before deleting it",
		}
	}

	shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{
		Action:     "tenant.deleted",
		TargetType: "tenant",
		TargetID:   cmd.ID,
		Metadata:   map[string]any{"name": target.Name},
	})

	return nil
}
