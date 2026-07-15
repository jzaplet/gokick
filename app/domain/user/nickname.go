package user

import (
	"unicode/utf8"

	"gokick/app/domain/shared"
)

type Nickname string

func NewNickname(s string) (Nickname, error) {
	if s == "" {
		return "", &shared.ValidationError{Field: "nickname", Message: "nickname is required"}
	}
	// Count characters (runes), not bytes, to match the "characters" limit the
	// message promises — a 50-rune accented nickname must not be rejected early.
	if utf8.RuneCountInString(s) > 50 {
		return "", &shared.ValidationError{
			Field:   "nickname",
			Message: "nickname must be at most 50 characters",
		}
	}
	return Nickname(s), nil
}
