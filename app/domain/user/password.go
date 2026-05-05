package user

import "gokick/app/domain/shared"

type Password string

func NewPassword(s string) (Password, error) {
	if s == "" {
		return "", &shared.ValidationError{Field: "password", Message: "password is required"}
	}
	if len(s) < 8 {
		return "", &shared.ValidationError{
			Field:   "password",
			Message: "password must be at least 8 characters",
		}
	}
	if len(s) > 128 {
		return "", &shared.ValidationError{
			Field:   "password",
			Message: "password must be at most 128 characters",
		}
	}
	return Password(s), nil
}
