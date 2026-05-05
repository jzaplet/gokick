package user

import (
	"strings"

	"gokick/app/domain/shared"
)

type Email string

// NewEmail validates the email. Empty string is allowed (email is optional).
func NewEmail(s string) (Email, error) {
	if s == "" {
		return "", nil
	}
	if len(s) > 254 {
		return "", &shared.ValidationError{
			Field:   "email",
			Message: "email must be at most 254 characters",
		}
	}
	if !strings.Contains(s, "@") {
		return "", &shared.ValidationError{Field: "email", Message: "invalid email format"}
	}
	return Email(s), nil
}
