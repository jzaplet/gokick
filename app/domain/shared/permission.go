package shared

import "context"

type Permissioned interface {
	RequiredPermission() string
}

type SkipPermission interface {
	SkipPermissionCheck()
}

type PermissionChecker interface {
	Check(ctx context.Context, permission string) error
}
