package user

import (
	"strings"

	"gokick/app/domain/shared"
)

type Email string

// NewEmail validuje email. Prázdný řetězec je povolený (email je nepovinný).
func NewEmail(s string) (Email, error) {
	if s == "" {
		return "", nil
	}
	if len(s) > 254 {
		return "", &shared.ValidationError{Field: "email", Message: "email max 254 znaků"}
	}
	if !strings.Contains(s, "@") {
		return "", &shared.ValidationError{Field: "email", Message: "neplatný formát emailu"}
	}
	return Email(s), nil
}
