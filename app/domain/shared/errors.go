package shared

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string      { return e.Message }
func (e *ValidationError) HTTPStatus() int    { return 400 }
func (e *ValidationError) ErrorField() string { return e.Field }

// AuthError indicates the caller is not authenticated (no/invalid/expired credentials).
// Maps to HTTP 401.
type AuthError struct {
	Message string
}

func (e *AuthError) Error() string   { return e.Message }
func (e *AuthError) HTTPStatus() int { return 401 }

// PermissionError indicates the caller is authenticated but not permitted to perform
// the requested operation. Maps to HTTP 403.
type PermissionError struct {
	Message string
}

func (e *PermissionError) Error() string   { return e.Message }
func (e *PermissionError) HTTPStatus() int { return 403 }
