package command

import (
	"context"

	"gokick/app/domain/shared"
	"gokick/app/domain/tenant"
)

// BulkDeleteTenantsCommand deletes every SELECTED tenant that owns no
// users. Same dual-mode selection as the user bulk pair (explicit ids, or
// all_filtered + the tenants grid's filter set).
//
// Unlike the user twins there is no actor exclusion — a tenant has no "self".
// The two tenants that must survive are refused by the statement rather than
// filtered out of the selection: the default tenant by id, and any tenant still
// holding users by the same emptiness condition the single-row delete uses.
type BulkDeleteTenantsCommand struct {
	IDs         []string
	AllFiltered bool
	Name        string
	Plan        string
}

func (BulkDeleteTenantsCommand) RequiredPermission() string {
	return "platform:tenants:delete"
}

type BulkDeleteTenantsHandler struct {
	tenants tenant.PlatformRepository
}

func NewBulkDeleteTenantsHandler(
	tenants tenant.PlatformRepository,
) *BulkDeleteTenantsHandler {
	return &BulkDeleteTenantsHandler{tenants: tenants}
}

// Handle returns how many tenants were actually deleted — NOT how many were
// selected. A selection of five where three still have users deletes two and
// says two; the caller turns that into an honest toast. Skipping is the designed
// outcome (the superadmin selects freely), so a non-empty tenant in the
// selection is not an error.
func (h *BulkDeleteTenantsHandler) Handle(
	ctx context.Context,
	cmd BulkDeleteTenantsCommand,
) (int64, error) {
	sel := tenant.BulkSelection{
		IDs:         cmd.IDs,
		AllFiltered: cmd.AllFiltered,
		Filters:     tenant.ListFilters{Name: cmd.Name, Plan: cmd.Plan},
	}
	if sel.IsEmpty() {
		return 0, &shared.ValidationError{Message: "nothing selected"}
	}

	affected, err := h.tenants.BulkDeleteEmptyAcrossTenants(ctx, sel)
	if err != nil {
		return 0, err
	}

	shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{
		Action:     "tenant.bulk_deleted",
		TargetType: "tenant",
		Metadata:   bulkTenantAuditMeta(affected, sel),
	})

	return affected, nil
}

// bulkTenantAuditMeta mirrors platformBulkAuditMeta: record the filters in
// all-filtered mode (the ids are not knowable from the request) and the explicit
// ids otherwise. Note the ids recorded are the SELECTED ones — `affected` is what
// says how many of them actually went.
func bulkTenantAuditMeta(affected int64, sel tenant.BulkSelection) map[string]any {
	m := map[string]any{"affected": affected, "all_filtered": sel.AllFiltered}
	if sel.AllFiltered {
		m["filters"] = map[string]any{"name": sel.Filters.Name, "plan": sel.Filters.Plan}
	} else {
		m["ids"] = sel.IDs
	}

	return m
}
