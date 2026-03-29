package user

import "myapp/app/domain/shared"

type Nickname string

func NewNickname(s string) (Nickname, error) {
	if s == "" {
		return "", &shared.ValidationError{Field: "nickname", Message: "nickname je povinný"}
	}
	if len(s) > 50 {
		return "", &shared.ValidationError{Field: "nickname", Message: "nickname max 50 znaků"}
	}
	return Nickname(s), nil
}
