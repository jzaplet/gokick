package command

import (
	"context"

	"gokick/app/domain/shared"
	"gokick/app/domain/user"
)

// Bulk commands mirror the grid's dual-mode selection: explicit ids, or "all
// filtered" carrying the filter set instead of an id enumeration. Both write
// paths inherit the repository's tenant scoping + superadmin exclusion, and
// always exclude the ACTOR — a bulk delete must not saw off the branch the
// admin sits on (the single-delete self-protection, generalized).

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

func bulkSelection(
	ids []string,
	allFiltered bool,
	filters user.ListFilters,
	actorID string,
) (user.BulkSelection, error) {
	sel := user.BulkSelection{
		IDs:         ids,
		AllFiltered: allFiltered,
		Filters:     filters,
		ExcludeID:   actorID,
	}
	if sel.IsEmpty() {
		return user.BulkSelection{}, &shared.ValidationError{Message: "nothing selected"}
	}
	return sel, nil
}

func (h *BulkDeleteUsersHandler) Handle(ctx context.Context, cmd BulkDeleteUsersCommand) error {
	claims, err := shared.RequireClaims(ctx)
	if err != nil {
		return err
	}

	filters := user.ListFilters{
		Nickname: cmd.Nickname,
		Email:    cmd.Email,
		Role:     cmd.Role,
		Active:   cmd.Active,
	}
	sel, err := bulkSelection(cmd.IDs, cmd.AllFiltered, filters, claims.UserID)
	if err != nil {
		return err
	}

	affected, err := h.users.BulkDelete(ctx, sel)
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
) error {
	claims, err := shared.RequireClaims(ctx)
	if err != nil {
		return err
	}

	filters := user.ListFilters{
		Nickname: cmd.Nickname,
		Email:    cmd.Email,
		Role:     cmd.Role,
		Active:   cmd.Active,
	}
	sel, err := bulkSelection(cmd.IDs, cmd.AllFiltered, filters, claims.UserID)
	if err != nil {
		return err
	}

	affected, err := h.users.BulkSetActive(ctx, sel, cmd.SetActive)
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
