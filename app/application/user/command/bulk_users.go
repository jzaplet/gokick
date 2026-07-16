package command

import (
	"context"

	"gokick/app/domain/shared"
	"gokick/app/domain/user"
)

// Bulk commands mirror the grid's dual-mode selection: explicit ids, or "all
// filtered" carrying the filter set instead of an id enumeration. Both write
// paths inherit the repository's tenant scoping + superadmin exclusion. Delete
// and deactivate also exclude the ACTOR — a bulk op must not saw off the branch
// the admin sits on — while activate does not (activating yourself is safe, and
// excluding the actor would make a self-activate a silent no-op). Both return
// the affected-row count so the caller can report the truth (0 rows ≠ success).

type BulkDeleteUsersCommand struct {
	IDs         []string
	AllFiltered bool
	Nickname    string
	Email       string
	Role        string
	Active      string
}

func (BulkDeleteUsersCommand) RequiredPermission() string { return "admin:users:delete" }

type BulkDeleteUsersHandler struct {
	users user.Repository
}

func NewBulkDeleteUsersHandler(users user.Repository) *BulkDeleteUsersHandler {
	return &BulkDeleteUsersHandler{users: users}
}

// bulkSelection builds the validated selection. excludeID is the actor to spare
// ("" = spare no one, used by activate). An unrecognized active filter is a
// hard error: on the destructive bulk path a silently-dropped condition would
// widen "delete inactive" into "delete everyone".
func bulkSelection(
	ids []string,
	allFiltered bool,
	filters user.ListFilters,
	excludeID string,
) (user.BulkSelection, error) {
	if !user.ValidActiveFilter(filters.Active) {
		return user.BulkSelection{}, &shared.ValidationError{Message: "invalid active filter value"}
	}
	sel := user.BulkSelection{
		IDs:         ids,
		AllFiltered: allFiltered,
		Filters:     filters,
		ExcludeID:   excludeID,
	}
	if sel.IsEmpty() {
		return user.BulkSelection{}, &shared.ValidationError{Message: "nothing selected"}
	}
	return sel, nil
}

// bulkAuditMeta records enough to attribute a destructive bulk op: the affected
// count, the mode, and either the id list (id mode) or the filter set
// (all-filtered mode). Without this the compliance log cannot answer "who
// removed user X".
func bulkAuditMeta(affected int64, sel user.BulkSelection) map[string]any {
	m := map[string]any{"affected": affected, "all_filtered": sel.AllFiltered}
	if sel.AllFiltered {
		m["filters"] = map[string]any{
			"nickname": sel.Filters.Nickname,
			"email":    sel.Filters.Email,
			"role":     sel.Filters.Role,
			"active":   sel.Filters.Active,
		}
	} else {
		m["ids"] = sel.IDs
	}
	return m
}

func (h *BulkDeleteUsersHandler) Handle(
	ctx context.Context,
	cmd BulkDeleteUsersCommand,
) (int64, error) {
	claims, err := shared.RequireClaims(ctx)
	if err != nil {
		return 0, err
	}

	filters := user.ListFilters{
		Nickname: cmd.Nickname,
		Email:    cmd.Email,
		Role:     cmd.Role,
		Active:   cmd.Active,
	}
	sel, err := bulkSelection(cmd.IDs, cmd.AllFiltered, filters, claims.UserID)
	if err != nil {
		return 0, err
	}

	affected, err := h.users.BulkDelete(ctx, sel)
	if err != nil {
		return 0, err
	}

	shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{
		Action:     "user.bulk_deleted",
		TargetType: "user",
		Metadata:   bulkAuditMeta(affected, sel),
	})

	return affected, nil
}

type BulkSetUsersActiveCommand struct {
	IDs         []string
	AllFiltered bool
	Nickname    string
	Email       string
	Role        string
	Active      string
	SetActive   bool
}

func (BulkSetUsersActiveCommand) RequiredPermission() string { return "admin:users:update" }

type BulkSetUsersActiveHandler struct {
	users user.Repository
}

func NewBulkSetUsersActiveHandler(users user.Repository) *BulkSetUsersActiveHandler {
	return &BulkSetUsersActiveHandler{users: users}
}

func (h *BulkSetUsersActiveHandler) Handle(
	ctx context.Context,
	cmd BulkSetUsersActiveCommand,
) (int64, error) {
	claims, err := shared.RequireClaims(ctx)
	if err != nil {
		return 0, err
	}

	// Activate spares no one (activating yourself is safe); deactivate excludes
	// the actor so an admin cannot lock themselves out in a bulk op.
	excludeID := claims.UserID
	if cmd.SetActive {
		excludeID = ""
	}

	filters := user.ListFilters{
		Nickname: cmd.Nickname,
		Email:    cmd.Email,
		Role:     cmd.Role,
		Active:   cmd.Active,
	}
	sel, err := bulkSelection(cmd.IDs, cmd.AllFiltered, filters, excludeID)
	if err != nil {
		return 0, err
	}

	affected, err := h.users.BulkSetActive(ctx, sel, cmd.SetActive)
	if err != nil {
		return 0, err
	}

	action := "user.bulk_deactivated"
	if cmd.SetActive {
		action = "user.bulk_activated"
	}
	shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{
		Action:     action,
		TargetType: "user",
		Metadata:   bulkAuditMeta(affected, sel),
	})

	return affected, nil
}
