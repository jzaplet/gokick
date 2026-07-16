package command

import (
	"context"

	"gokick/app/domain/shared"
	"gokick/app/domain/user"
)

// Platform bulk commands — the cross-tenant twins of the admin bulk pair.
// Same dual-mode selection (ids, or all_filtered + the platform filter set
// incl. tenant name), same actor exclusion for delete/deactivate (not
// activate), same affected-count return; superadmin rows are spared by the
// repository statements themselves.

type BulkDeletePlatformUsersCommand struct {
	IDs         []string
	AllFiltered bool
	Tenant      string
	Nickname    string
	Email       string
	Role        string
	Active      string
}

func (BulkDeletePlatformUsersCommand) RequiredPermission() string { return "platform:users:delete" }

type BulkDeletePlatformUsersHandler struct {
	users user.PlatformRepository
}

func NewBulkDeletePlatformUsersHandler(
	users user.PlatformRepository,
) *BulkDeletePlatformUsersHandler {
	return &BulkDeletePlatformUsersHandler{users: users}
}

func platformBulkSelection(
	ids []string,
	allFiltered bool,
	filters user.PlatformListFilters,
	excludeID string,
) (user.PlatformBulkSelection, error) {
	if !user.ValidActiveFilter(filters.Active) {
		return user.PlatformBulkSelection{}, &shared.ValidationError{
			Message: "invalid active filter value",
		}
	}
	sel := user.PlatformBulkSelection{
		IDs:         ids,
		AllFiltered: allFiltered,
		Filters:     filters,
		ExcludeID:   excludeID,
	}
	if sel.IsEmpty() {
		return user.PlatformBulkSelection{}, &shared.ValidationError{Message: "nothing selected"}
	}
	return sel, nil
}

func platformBulkAuditMeta(affected int64, sel user.PlatformBulkSelection) map[string]any {
	m := map[string]any{"affected": affected, "all_filtered": sel.AllFiltered}
	if sel.AllFiltered {
		m["filters"] = map[string]any{
			"tenant":   sel.Filters.Tenant,
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

func platformFilters(tenant, nickname, email, role, active string) user.PlatformListFilters {
	return user.PlatformListFilters{
		ListFilters: user.ListFilters{
			Nickname: nickname,
			Email:    email,
			Role:     role,
			Active:   active,
		},
		Tenant: tenant,
	}
}

func (h *BulkDeletePlatformUsersHandler) Handle(
	ctx context.Context,
	cmd BulkDeletePlatformUsersCommand,
) (int64, error) {
	claims, err := shared.RequireClaims(ctx)
	if err != nil {
		return 0, err
	}

	filters := platformFilters(cmd.Tenant, cmd.Nickname, cmd.Email, cmd.Role, cmd.Active)
	sel, err := platformBulkSelection(cmd.IDs, cmd.AllFiltered, filters, claims.UserID)
	if err != nil {
		return 0, err
	}

	affected, err := h.users.BulkDeleteAcrossTenants(ctx, sel)
	if err != nil {
		return 0, err
	}

	shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{
		Action:     "user.bulk_deleted",
		TargetType: "user",
		Metadata:   platformBulkAuditMeta(affected, sel),
	})

	return affected, nil
}

type BulkSetPlatformUsersActiveCommand struct {
	IDs         []string
	AllFiltered bool
	Tenant      string
	Nickname    string
	Email       string
	Role        string
	Active      string
	SetActive   bool
}

func (BulkSetPlatformUsersActiveCommand) RequiredPermission() string {
	return "platform:users:update"
}

type BulkSetPlatformUsersActiveHandler struct {
	users user.PlatformRepository
}

func NewBulkSetPlatformUsersActiveHandler(
	users user.PlatformRepository,
) *BulkSetPlatformUsersActiveHandler {
	return &BulkSetPlatformUsersActiveHandler{users: users}
}

func (h *BulkSetPlatformUsersActiveHandler) Handle(
	ctx context.Context,
	cmd BulkSetPlatformUsersActiveCommand,
) (int64, error) {
	claims, err := shared.RequireClaims(ctx)
	if err != nil {
		return 0, err
	}

	excludeID := claims.UserID
	if cmd.SetActive {
		excludeID = ""
	}

	filters := platformFilters(cmd.Tenant, cmd.Nickname, cmd.Email, cmd.Role, cmd.Active)
	sel, err := platformBulkSelection(cmd.IDs, cmd.AllFiltered, filters, excludeID)
	if err != nil {
		return 0, err
	}

	affected, err := h.users.BulkSetActiveAcrossTenants(ctx, sel, cmd.SetActive)
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
		Metadata:   platformBulkAuditMeta(affected, sel),
	})

	return affected, nil
}
