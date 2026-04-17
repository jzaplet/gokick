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

	return h.tokens.DeleteByUserID(ctx, claims.UserID)
}
