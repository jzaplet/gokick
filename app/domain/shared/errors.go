package shared

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string   { return e.Message }
func (e *ValidationError) HTTPStatus() int { return 400 }

type AuthError struct {
	Message string
}

func (e *AuthError) Error() string   { return e.Message }
func (e *AuthError) HTTPStatus() int { return 403 }
