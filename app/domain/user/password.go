package user

import (
	"unicode/utf8"

	"gokick/app/domain/shared"
)

type Password string

// HashNewPassword validates a raw password into the Password value object and
// hashes it in one call — the single rules+hashing seam shared by every write
// path that hashes a fresh password inline (change-password, the admin/platform
// update body). Returns a *shared.ValidationError (field "password") on an invalid
// password, the hasher's error otherwise. Handlers that validate the password
// EARLY but hash LATE (create-user/superadmin, to skip the bcrypt cost when the
// nickname is already taken) keep NewPassword and Hash separate on purpose.
func HashNewPassword(raw string, hasher shared.PasswordHasher) (string, error) {
	pw, err := NewPassword(raw)
	if err != nil {
		return "", err
	}
	return hasher.Hash(string(pw))
}

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
