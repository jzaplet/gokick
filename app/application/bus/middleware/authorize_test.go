package middleware

import (
	"context"
	"errors"
	"testing"
)

// stubChecker is a PermissionChecker whose Check result is configurable and
// which records whether it was consulted at all.
type stubChecker struct {
	err     error
	called  bool
	gotPerm string
}

func (s *stubChecker) Check(_ context.Context, permission string) error {
	s.called = true
	s.gotPerm = permission
	return s.err
}

// permitCmd implements shared.Permissioned.
type permitCmd struct{ perm string }

func (c permitCmd) RequiredPermission() string { return c.perm }

// bareCmd implements NEITHER Permissioned nor SkipPermission — the exact
// mistake the fail-closed default exists to catch.
type bareCmd struct{}

// A command that declares neither Permissioned nor SkipPermission MUST be
// rejected, and the handler MUST NOT run. This is the fail-closed guard
// CLAUDE.md promises ("Forgetting both ... is a runtime error — protects
// against forgotten declarations"). A regression here fails OPEN: an
// unguarded command would execute with no permission check at all.
func TestAuthorizeMiddleware_RejectsCommandWithoutDeclaration(t *testing.T) {
	t.Parallel()
	checker := &stubChecker{}
	mw := AuthorizeMiddleware(checker)

	var ran bool
	_, err := mw(t.Context(), "Bare", bareCmd{}, func(context.Context) (any, error) {
		ran = true
		return nil, nil
	})
	if err == nil {
		t.Fatal("command without Permissioned/SkipPermission must be rejected")
	}
	if ran {
		t.Fatal("handler must NOT run for an undeclared command (fail-closed)")
	}
	if checker.called {
		t.Fatal("checker must not be consulted for an undeclared command")
	}
}

// A Permissioned command forwards its required permission to the checker; a
// denial propagates and the handler does not run.
func TestAuthorizeMiddleware_DeniesWhenCheckerRejects(t *testing.T) {
	t.Parallel()
	denied := errors.New("permission denied")
	checker := &stubChecker{err: denied}
	mw := AuthorizeMiddleware(checker)

	var ran bool
	_, err := mw(
		t.Context(),
		"Create",
		permitCmd{perm: "admin:users:create"},
		func(context.Context) (any, error) {
			ran = true
			return nil, nil
		},
	)
	if !errors.Is(err, denied) {
		t.Fatalf("expected denial to propagate, got %v", err)
	}
	if ran {
		t.Fatal("handler must not run when permission is denied")
	}
	if checker.gotPerm != "admin:users:create" {
		t.Fatalf("checker got permission %q, want admin:users:create", checker.gotPerm)
	}
}

// A Permissioned command with an allowing checker runs the handler.
func TestAuthorizeMiddleware_AllowsWhenCheckerPasses(t *testing.T) {
	t.Parallel()
	checker := &stubChecker{}
	mw := AuthorizeMiddleware(checker)

	got, err := mw(
		t.Context(),
		"Read",
		permitCmd{perm: "profile:read"},
		func(context.Context) (any, error) {
			return "ok", nil
		},
	)
	if err != nil {
		t.Fatalf("allowed command must run: %v", err)
	}
	if got != "ok" {
		t.Fatalf("result: %v", got)
	}
	if !checker.called {
		t.Fatal("checker must be consulted for a Permissioned command")
	}
}

// A SkipPermission command bypasses the checker entirely and runs. noopCommand
// (events_test.go) implements SkipPermissionCheck.
func TestAuthorizeMiddleware_SkipPermissionBypassesChecker(t *testing.T) {
	t.Parallel()
	checker := &stubChecker{err: errors.New("checker should never be called")}
	mw := AuthorizeMiddleware(checker)

	got, err := mw(t.Context(), "Noop", noopCommand{}, func(context.Context) (any, error) {
		return "ran", nil
	})
	if err != nil {
		t.Fatalf("SkipPermission command must run: %v", err)
	}
	if got != "ran" {
		t.Fatalf("result: %v", got)
	}
	if checker.called {
		t.Fatal("checker must not be consulted for a SkipPermission command")
	}
}
