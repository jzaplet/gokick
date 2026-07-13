package tenant

import (
	"strings"

	"gokick/app/domain/shared"
)

// Name is a validated tenant name: required, trimmed, non-blank. NewTenant takes
// this value object (not a raw string) so no construction path — the CreateTenant
// handler, the seeder, testfx — can mint an empty-named tenant. Length/format
// constraints are deliberately out of scope (the audit scoped this to
// required/non-blank/trim only); the future in-app tenant signup can add them.
type Name string

func NewName(s string) (Name, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return "", &shared.ValidationError{Field: "name", Message: "tenant name is required"}
	}

	return Name(trimmed), nil
}
