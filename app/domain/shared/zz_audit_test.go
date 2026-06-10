package shared

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestValidationError_Methods pins the three documented return values of
// ValidationError in one place: Error() echoes Message, HTTPStatus() is 400,
// and ErrorField() echoes Field. Closes domain-46 and (via the 400 mapping
// mechanism) overview-25 / guide-forms-fe-10.
func TestValidationError_Methods(t *testing.T) {
	t.Parallel()

	ve := &ValidationError{Field: "nickname", Message: "nickname is required"}

	if got := ve.Error(); got != "nickname is required" {
		t.Errorf("Error() = %q, want %q", got, "nickname is required")
	}
	if got := ve.HTTPStatus(); got != 400 {
		t.Errorf("HTTPStatus() = %d, want 400", got)
	}
	if got := ve.ErrorField(); got != "nickname" {
		t.Errorf("ErrorField() = %q, want %q", got, "nickname")
	}
}

// TestValidationError_SatisfiesErrorAndHTTPStatusContracts proves the duck-typed
// contracts the presentation layer relies on: *ValidationError is a standard
// error (errors.As works) AND exposes HTTPStatus()==400 through the same
// interface response.HandleError dispatches on. Closes overview-25 /
// guide-forms-fe-10 at the value level (the 400 source of the HTTP mapping).
func TestValidationError_SatisfiesErrorAndHTTPStatusContracts(t *testing.T) {
	t.Parallel()

	var err error = &ValidationError{Field: "f", Message: "boom"}

	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("errors.As should unwrap to *ValidationError, got %T", err)
	}

	// The HTTP layer maps via a duck-typed HTTPStatus() int interface.
	type httpError interface{ HTTPStatus() int }
	he, ok := err.(httpError)
	if !ok {
		t.Fatal("*ValidationError must implement HTTPStatus() int")
	}
	if got := he.HTTPStatus(); got != 400 {
		t.Errorf("HTTPStatus() = %d, want 400", got)
	}
}

// TestPermissionError_Methods pins PermissionError's documented behavior:
// Error() echoes Message and HTTPStatus() returns 403. Closes domain-49 and
// (via the 403 mapping mechanism) overview-27 / guide-forms-fe-12.
func TestPermissionError_Methods(t *testing.T) {
	t.Parallel()

	pe := &PermissionError{Message: "forbidden"}

	if got := pe.Error(); got != "forbidden" {
		t.Errorf("Error() = %q, want %q", got, "forbidden")
	}
	if got := pe.HTTPStatus(); got != 403 {
		t.Errorf("HTTPStatus() = %d, want 403", got)
	}
}

// TestPermissionError_SatisfiesErrorAndHTTPStatusContracts confirms the
// duck-typed mapping path: *PermissionError is an error and exposes
// HTTPStatus()==403 through the interface the response layer dispatches on.
// Closes overview-27 / guide-forms-fe-12 at the value level.
func TestPermissionError_SatisfiesErrorAndHTTPStatusContracts(t *testing.T) {
	t.Parallel()

	var err error = &PermissionError{Message: "nope"}

	var pe *PermissionError
	if !errors.As(err, &pe) {
		t.Fatalf("errors.As should unwrap to *PermissionError, got %T", err)
	}

	type httpError interface{ HTTPStatus() int }
	he, ok := err.(httpError)
	if !ok {
		t.Fatal("*PermissionError must implement HTTPStatus() int")
	}
	if got := he.HTTPStatus(); got != 403 {
		t.Errorf("HTTPStatus() = %d, want 403", got)
	}
}

// TestErrorTypes_DistinctHTTPStatuses guards against a copy/paste regression
// across the three sibling error types: each maps to its own documented status
// (Validation 400, Auth 401, Permission 403). If any two collapsed to the same
// status the HTTP error contract would silently break; this fails if so.
func TestErrorTypes_DistinctHTTPStatuses(t *testing.T) {
	t.Parallel()

	if s := (&ValidationError{}).HTTPStatus(); s != 400 {
		t.Errorf("ValidationError.HTTPStatus() = %d, want 400", s)
	}
	if s := (&AuthError{}).HTTPStatus(); s != 401 {
		t.Errorf("AuthError.HTTPStatus() = %d, want 401", s)
	}
	if s := (&PermissionError{}).HTTPStatus(); s != 403 {
		t.Errorf("PermissionError.HTTPStatus() = %d, want 403", s)
	}
}

// stubEvent is a minimal DomainEvent for exercising the collector.
type stubEvent struct {
	name string
	at   time.Time
}

func (e stubEvent) EventName() string     { return e.name }
func (e stubEvent) OccurredAt() time.Time { return e.at }

// TestEventCollectorFromContext_ThrowawayOutsideBus proves the documented CLI
// bypass behavior: with no collector installed in ctx, EventCollectorFromContext
// returns a fresh throwaway each call (never a shared singleton that something
// would flush), and Collect on it succeeds (the event is retained locally) yet
// is invisible to any other collector — i.e. silently dropped from the bus's
// perspective. Closes app-events-audit-10.
func TestEventCollectorFromContext_ThrowawayOutsideBus(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	c1 := EventCollectorFromContext(ctx)
	if c1 == nil {
		t.Fatal("EventCollectorFromContext must never return nil")
	}

	// Two bare-context lookups must yield DISTINCT instances — proving the
	// returned collector is a throwaway, not a persistent one anybody flushes.
	c2 := EventCollectorFromContext(ctx)
	if c1 == c2 {
		t.Fatal(
			"outside the bus each call must return a fresh throwaway collector, got the same instance",
		)
	}

	// Collect succeeds (does not panic — this is not a forbidden collector) and
	// the event is retained on c1 only.
	c1.Collect(stubEvent{name: "user.created", at: time.Now()})

	if got := c1.Flush(); len(got) != 1 || got[0].EventName() != "user.created" {
		t.Fatalf("Collect should retain the event on the throwaway collector, got %+v", got)
	}

	// The event never reached the sibling throwaway — nothing dispatches it.
	if got := c2.Flush(); len(got) != 0 {
		t.Fatalf("event must not leak to an unrelated throwaway collector, got %+v", got)
	}
}

// TestEventCollectorFromContext_ReturnsInstalledCollector is the contrast case:
// when the bus HAS installed a collector, the same lookup returns THAT instance
// (so the command flow's events are the ones that get flushed/dispatched),
// confirming the throwaway path above is specific to the no-collector bypass.
func TestEventCollectorFromContext_ReturnsInstalledCollector(t *testing.T) {
	t.Parallel()

	ctx, installed := ContextWithEventCollector(context.Background())

	if got := EventCollectorFromContext(ctx); got != installed {
		t.Fatal(
			"inside the bus, EventCollectorFromContext must return the installed collector, not a throwaway",
		)
	}
}
