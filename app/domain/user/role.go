package user

import "gokick/app/domain/shared"

type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

func NewRole(s string) (Role, error) {
	switch Role(s) {
	case RoleAdmin, RoleUser:
		return Role(s), nil
	default:
		return "", &shared.ValidationError{Field: "role", Message: "invalid role"}
	}
}
