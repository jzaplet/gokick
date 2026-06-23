package middleware

import (
	"context"
	"errors"
	"testing"

	"gokick/app/domain/shared"
)

// stubTenantResolver is a test double for shared.TenantResolver. It is shared
// across this package's tests (zz_gap builds a BaseChain that needs one).
type stubTenantResolver struct {
	id  string
	err error
}

func (r stubTenantResolver) Resolve(context.Context) (string, error) {
	return r.id, r.err
}

// TenantMiddleware must put the resolved tenant id into the handler's ctx so
// every downstream read/write scopes to it.
func TestTenantMiddleware_InjectsResolvedTenantIntoContext(t *testing.T) {
	var seen string
	mw := TenantMiddleware(stubTenantResolver{id: "tenant-xyz"})

	_, err := mw(
		context.Background(),
		"AnyCmd",
		struct{}{},
		func(ctx context.Context) (any, error) {
			seen = shared.TenantIDFromContext(ctx)
			return nil, nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seen != "tenant-xyz" {
		t.Fatalf("handler ctx tenant = %q, want %q", seen, "tenant-xyz")
	}
}

// A resolver error must abort before the handler runs — an unresolvable tenant
// is never silently treated as "no tenant".
func TestTenantMiddleware_AbortsAndDoesNotCallNextOnResolverError(t *testing.T) {
	wantErr := errors.New("resolver boom")
	called := false
	mw := TenantMiddleware(stubTenantResolver{err: wantErr})

	_, err := mw(context.Background(), "AnyCmd", struct{}{}, func(context.Context) (any, error) {
		called = true
		return nil, nil
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if called {
		t.Fatal("next handler must NOT run when tenant resolution fails")
	}
}
