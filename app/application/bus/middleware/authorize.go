package middleware

import (
	"context"
	"fmt"
	"gokick/app/application/bus"
	"gokick/app/domain/shared"
)

func AuthorizeMiddleware(checker shared.PermissionChecker) bus.Middleware {
	return func(ctx context.Context, name string, cmd any, next func(ctx context.Context) (any, error)) (any, error) {
		switch c := cmd.(type) {
		case shared.Permissioned:
			if err := checker.Check(ctx, c.RequiredPermission()); err != nil {
				return nil, err
			}
		case shared.SkipPermission:
			// explicitly skipped
		default:
			return nil, fmt.Errorf("bus: command %q must implement Permissioned or SkipPermission", name)
		}
		return next(ctx)
	}
}
