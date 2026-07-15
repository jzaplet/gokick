// Package other trips both name-matching tripwires: a second ValidationError
// type outside domain/shared and a constructor the literal scan cannot see.
package other

type ValidationError struct{ Field, Message string }

func NewValidationError(field, message string) *ValidationError {
	_, _ = field, message
	return nil
}
