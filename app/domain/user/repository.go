package user

import (
	"context"
	"time"
)

type Repository interface {
	Save(ctx context.Context, user *User) error
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, id string) error
	FindByID(ctx context.Context, id string) (*User, error)
	FindByNickname(ctx context.Context, nickname string) (*User, error)
	FindAllActive(ctx context.Context) ([]User, error)
	FindAll(ctx context.Context) ([]User, error)

	// RecordFailedLogin atomically bumps the failed-login counter for the
	// user. The implementation decides reset / lock inside a single SQL
	// statement so two concurrent failed logins can't race past the
	// threshold. Returns the new locked_until when locking was triggered
	// (else nil). Must run OUTSIDE the caller's transaction so the count
	// persists when the login handler returns AuthError and the
	// surrounding bus tx rolls back.
	RecordFailedLogin(
		ctx context.Context,
		userID string,
		threshold int,
		window, lockDuration time.Duration,
	) (*time.Time, error)

	// ResetFailedLogin clears the counter after a successful login. Runs
	// outside the caller's tx for the same reason as RecordFailedLogin.
	ResetFailedLogin(ctx context.Context, userID string) error
}
