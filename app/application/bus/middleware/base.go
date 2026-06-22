package middleware

import (
	"gokick/app/application/bus"
	"gokick/app/domain/shared"
	"log/slog"
)

// BaseChain returns the recovery + logging + authorize + tenant quartet shared
// by CommandBus, QueryBus and any bus that runs user-driven commands. Tenant
// resolution sits right after authorization so every handler and read runs with
// the active tenant in ctx. Bus-specific extras (Transaction, DispatchEvents)
// are appended by the caller.
func BaseChain(
	logger *slog.Logger,
	checker shared.PermissionChecker,
	reporter shared.ErrorReporter,
	tenantResolver shared.TenantResolver,
) []bus.Middleware {
	return []bus.Middleware{
		RecoveryMiddleware(logger, reporter),
		LoggingMiddleware(logger),
		AuthorizeMiddleware(checker),
		TenantMiddleware(tenantResolver),
	}
}
