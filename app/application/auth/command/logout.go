package command

import (
	"context"

	"gokick/app/domain/shared"
	"gokick/app/domain/token"
)

type LogoutCommand struct{}

func (LogoutCommand) RequiredPermission() string { return "auth:logout" }

type LogoutHandler struct {
	tokens token.Repository
}

func NewLogoutHandler(tokens token.Repository) *LogoutHandler {
	return &LogoutHandler{tokens: tokens}
}

func (h *LogoutHandler) Handle(ctx context.Context, _ LogoutCommand) error {
	claims, err := shared.RequireClaims(ctx)
	if err != nil {
		return err
	}

	// Record-before-revoke (mirrors the refresh theft branch): logout is a
	// security-relevant global session revocation, so the intent is audited even
	// if DeleteByUserID errors. The collector is a throwaway outside the bus.
	shared.AuditCollectorFromContext(ctx).Record(shared.AuditEvent{
		Action:     "auth.logout",
		TargetType: "user",
		TargetID:   claims.UserID,
	})

	return h.tokens.DeleteByUserID(ctx, claims.UserID)
}
