package user

import (
	"unicode/utf8"

	"gokick/app/domain/shared"
)

type Password string

func NewPassword(s string) (Password, error) {
	if s == "" {
		return "", &shared.ValidationError{Field: "password", Message: "password is required"}
	}
	// Minimum counts CHARACTERS (runes) to match the "characters" the message
	// promises — an 8-rune accented password is 8 chars, not its byte length.
	if utf8.RuneCountInString(s) < 8 {
		return "", &shared.ValidationError{
			Field:   "password",
			Message: "password must be at least 8 characters",
		}
	}
	// Maximum is a BYTE cap on purpose: an anti-DoS bound on the hasher input
	// (SHA-256 prehash), so the message says bytes, not characters.
	if len(s) > 128 {
		return "", &shared.ValidationError{
			Field:   "password",
			Message: "password must be at most 128 bytes",
		}
	}
	return Password(s), nil
}
