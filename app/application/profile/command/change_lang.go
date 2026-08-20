package command

import (
	"context"
	"time"

	"gokick/app/domain/shared"
	"gokick/app/domain/user"
)

type ChangeLangCommand struct {
	Lang string
}

func (ChangeLangCommand) RequiredPermission() string { return "profile:update" }

type ChangeLangHandler struct {
	users user.Repository
}

func NewChangeLangHandler(users user.Repository) *ChangeLangHandler {
	return &ChangeLangHandler{users: users}
}

// Handle persists the caller's own UI-language preference (users.lang). The
// next refresh mints the new value into the JWT claims; the SPA switches its
// locale state immediately on its side.
func (h *ChangeLangHandler) Handle(ctx context.Context, cmd ChangeLangCommand) error {
	claims, err := shared.RequireClaims(ctx)
	if err != nil {
		return err
	}

	lang, err := user.NewLang(cmd.Lang)
	if err != nil {
		return err
	}

	if err := h.users.UpdateLang(ctx, claims.UserID, lang, time.Now()); err != nil {
		return err
	}

	shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{
		Action:     "user.lang_changed",
		TargetType: "user",
		TargetID:   claims.UserID,
		Metadata:   map[string]any{"new_lang": string(lang)},
	})

	return nil
}
