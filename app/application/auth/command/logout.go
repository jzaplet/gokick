package command

import (
	"context"

	"gokick/app/domain/shared"
	"gokick/app/domain/token"
)

type LogoutCommand struct{}

func (LogoutCommand) RequiredPermission() string { return "auth:logout" }

type LogoutHandler struct {
	tokens token.TokenRepository
}

func NewLogoutHandler(tokens token.TokenRepository) *LogoutHandler {
	return &LogoutHandler{tokens: tokens}
}

func (h *LogoutHandler) Handle(ctx context.Context, _ LogoutCommand) error {
	claims := shared.ClaimsFromContext(ctx)
	if claims == nil {
		return &shared.AuthError{Message: "authentication required"}
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
