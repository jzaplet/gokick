package shared

import (
	"context"
	"errors"
	"testing"
)

func TestRequireClaims(t *testing.T) {
	t.Run("returns the claims when present", func(t *testing.T) {
		want := &AuthClaims{UserID: "u1", Role: "admin"}
		got, err := RequireClaims(ContextWithClaims(context.Background(), want))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != want {
			t.Fatalf("expected the same claims back, got %+v", got)
		}
	})

	t.Run("fails closed with an AuthError when absent", func(t *testing.T) {
		_, err := RequireClaims(context.Background())
		var ae *AuthError
		if !errors.As(err, &ae) {
			t.Fatalf("expected *AuthError (clean 401), got %T: %v", err, err)
		}
	})
}
