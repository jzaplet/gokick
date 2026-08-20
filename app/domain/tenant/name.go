package tenant

import (
	"strings"

	"gokick/app/domain/shared"
	"gokick/app/domain/shared/msgkey"
)

// Name is a validated tenant name: required, trimmed, non-blank. NewTenant takes
// this value object (not a raw string) so no construction path — the CreateTenant
// handler, the seeder, testfx — can mint an empty-named tenant. Length/format
// constraints are deliberately out of scope (the audit scoped this to
// required/non-blank/trim only); the future in-app tenant signup can add them.
//
// The Field below reaches a real form now: the superadmin plane's tenant create
// (PlatformTenantForm) routes it to its `name` input. It used to carry an errfields
// exemption saying no FE tenant form existed — that stopped being true.
type Name string

func NewName(s string) (Name, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return "", &shared.ValidationError{Field: "name", Key: msgkey.TenantNameRequired}
	}

	return Name(trimmed), nil
}
