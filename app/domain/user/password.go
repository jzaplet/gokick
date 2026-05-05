package user

import "gokick/app/domain/shared"

type Password string

func NewPassword(s string) (Password, error) {
	if s == "" {
		return "", &shared.ValidationError{Field: "password", Message: "heslo je povinné"}
	}
	if len(s) < 8 {
		return "", &shared.ValidationError{
			Field:   "password",
			Message: "heslo musí mít aspoň 8 znaků",
		}
	}
	if len(s) > 128 {
		return "", &shared.ValidationError{Field: "password", Message: "heslo max 128 znaků"}
	}
	return Password(s), nil
}
