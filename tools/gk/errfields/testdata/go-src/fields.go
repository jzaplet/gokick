// Package fixtures exercises the clean collector paths: qualified + local
// literals, raw-string and escaped Field values (strconv.Unquote territory),
// the Field "" general route, and exemptions over single- AND multi-line
// literals (the exemption keys on the literal's start line, so a golines
// rewrap must not detach it).
package fixtures

import "gokick/app/domain/shared"

func qualified() error {
	return &shared.ValidationError{Field: "nickname", Message: "taken"}
}

func local() error {
	return &ValidationError{Field: "email", Message: "invalid"}
}

func generalRoute() error {
	return &ValidationError{Field: "", Message: "goes to the general slot"}
}

func rawString() error {
	return &ValidationError{Field: `rawfield`, Message: "backquotes must unquote"}
}

func escaped() error {
	return &ValidationError{Field: "esc\u0061ped", Message: "escapes must decode"}
}

func exempted() error {
	//gkerrf:exempt path-param lookup, never a form field
	return &ValidationError{Field: "id", Message: "not found"}
}

func exemptedMultiline() error {
	//gkerrf:exempt path-param lookup wrapped by golines — must stay exempt
	return &ValidationError{
		Field:   "avatar",
		Message: "not found",
	}
}

func multiline() error {
	return &ValidationError{
		Field:   "role",
		Message: "invalid",
	}
}
