package user

import "gokick/app/domain/shared"

type Nickname string

func NewNickname(s string) (Nickname, error) {
	if s == "" {
		return "", &shared.ValidationError{Field: "nickname", Message: "nickname is required"}
	}
	if len(s) > 50 {
		return "", &shared.ValidationError{
			Field:   "nickname",
			Message: "nickname must be at most 50 characters",
		}
	}
	return Nickname(s), nil
}
