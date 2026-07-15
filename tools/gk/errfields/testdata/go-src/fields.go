// Package fixtures exercises the Go-side collector: qualified + local
// ValidationError literals, the Field "" general route, an exempted field and
// a multi-line composite literal.
package fixtures

import "gokick/app/domain/shared"

type ValidationError struct{ Field, Message string }

func qualified() error {
	return &shared.ValidationError{Field: "nickname", Message: "taken"}
}

func local() error {
	return &ValidationError{Field: "email", Message: "invalid"}
}

func generalRoute() error {
	return &ValidationError{Field: "", Message: "goes to the general slot"}
}

func exempted() error {
	//gkerrf:exempt path-param lookup, never a form field
	return &ValidationError{Field: "id", Message: "not found"}
}

func multiline() error {
	return &ValidationError{
		Field:   "role",
		Message: "invalid",
	}
}
