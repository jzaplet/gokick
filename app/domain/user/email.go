package user

import (
	"strings"

	"gokick/app/domain/shared"
	"gokick/app/domain/shared/msgkey"
)

type Email string

const maxEmailChars = 254

// NewEmail validates the email. Empty string is allowed (email is optional).
func NewEmail(s string) (Email, error) {
	if s == "" {
		return "", nil
	}
	if len(s) > maxEmailChars {
		return "", &shared.ValidationError{
			Field:  "email",
			Key:    msgkey.UserEmailTooLong,
			Params: map[string]any{"count": maxEmailChars},
		}
	}
	if !strings.Contains(s, "@") {
		return "", &shared.ValidationError{Field: "email", Key: msgkey.UserEmailInvalid}
	}
	return Email(s), nil
}
