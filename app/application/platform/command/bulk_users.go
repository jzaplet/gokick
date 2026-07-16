package command

import (
	"context"

	"gokick/app/domain/shared"
	"gokick/app/domain/user"
)

// Platform bulk commands — the cross-tenant twins of the admin bulk pair.
// Same dual-mode selection (ids, or all_filtered + the platform filter set
// incl. tenant name), same actor exclusion; superadmin rows are spared by the
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
	actorID string,
) (user.PlatformBulkSelection, error) {
	sel := user.PlatformBulkSelection{
		IDs:         ids,
		AllFiltered: allFiltered,
		Filters:     filters,
		ExcludeID:   actorID,
	}
	if sel.IsEmpty() {
		return user.PlatformBulkSelection{}, &shared.ValidationError{Message: "nothing selected"}
	}
	return sel, nil
}

func (h *BulkDeletePlatformUsersHandler) Handle(
	ctx context.Context,
	cmd BulkDeletePlatformUsersCommand,
) error {
	claims, err := shared.RequireClaims(ctx)
	if err != nil {
		return err
	}

	filters := user.PlatformListFilters{
		ListFilters: user.ListFilters{
			Nickname: cmd.Nickname,
			Email:    cmd.Email,
			Role:     cmd.Role,
			Active:   cmd.Active,
		},
		Tenant: cmd.Tenant,
	}
	sel, err := platformBulkSelection(cmd.IDs, cmd.AllFiltered, filters, claims.UserID)
	if err != nil {
		return err
	}

	affected, err := h.users.BulkDeleteAcrossTenants(ctx, sel)
	if err != nil {
		return err
	}

	shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{
		Action:     "user.bulk_deleted",
		TargetType: "user",
		Metadata:   map[string]any{"affected": affected, "all_filtered": sel.AllFiltered},
	})

	return nil
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
) error {
	claims, err := shared.RequireClaims(ctx)
	if err != nil {
		return err
	}

	filters := user.PlatformListFilters{
		ListFilters: user.ListFilters{
			Nickname: cmd.Nickname,
			Email:    cmd.Email,
			Role:     cmd.Role,
			Active:   cmd.Active,
		},
		Tenant: cmd.Tenant,
	}
	sel, err := platformBulkSelection(cmd.IDs, cmd.AllFiltered, filters, claims.UserID)
	if err != nil {
		return err
	}

	affected, err := h.users.BulkSetActiveAcrossTenants(ctx, sel, cmd.SetActive)
	if err != nil {
		return err
	}

	action := "user.bulk_deactivated"
	if cmd.SetActive {
		action = "user.bulk_activated"
	}
	shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{
		Action:     action,
		TargetType: "user",
		Metadata:   map[string]any{"affected": affected, "all_filtered": sel.AllFiltered},
	})

	return nil
}
