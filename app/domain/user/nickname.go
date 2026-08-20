package user

import (
	"unicode/utf8"

	"gokick/app/domain/shared"
	"gokick/app/domain/shared/msgkey"
)

type Nickname string

const maxNicknameChars = 50

func NewNickname(s string) (Nickname, error) {
	if s == "" {
		return "", &shared.ValidationError{Field: "nickname", Key: msgkey.UserNicknameRequired}
	}
	// Count characters (runes), not bytes, to match the "characters" limit the
	// message promises — a 50-rune accented nickname must not be rejected early.
	if utf8.RuneCountInString(s) > maxNicknameChars {
		return "", &shared.ValidationError{
			Field:  "nickname",
			Key:    msgkey.UserNicknameTooLong,
			Params: map[string]any{"count": maxNicknameChars},
		}
	}
	return Nickname(s), nil
}
