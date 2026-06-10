package middleware

import (
	"gokick/app/application/bus"
	"gokick/app/domain/shared"
	"log/slog"
)

// BaseChain returns the recovery + logging + authorize triplet shared by
// CommandBus, QueryBus and any bus that runs user-driven commands.
// Bus-specific extras (Transaction, DispatchEvents) are appended by the caller.
func BaseChain(
	logger *slog.Logger,
	checker shared.PermissionChecker,
	reporter shared.ErrorReporter,
) []bus.Middleware {
	return []bus.Middleware{
		RecoveryMiddleware(logger, reporter),
		LoggingMiddleware(logger),
		AuthorizeMiddleware(checker),
	}
}
