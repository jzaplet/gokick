package tenant

import (
	"errors"
	"testing"

	"gokick/app/domain/shared"
)

func TestNewName_TrimsAndAccepts(t *testing.T) {
	n, err := NewName("  acme  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(n) != "acme" {
		t.Fatalf("expected trimmed 'acme', got %q", n)
	}
}

func TestNewName_RejectsBlank(t *testing.T) {
	for _, blank := range []string{"", "   ", "\t\n"} {
		_, err := NewName(blank)
		var ve *shared.ValidationError
		if !errors.As(err, &ve) || ve.Field != "name" {
			t.Fatalf("expected a name ValidationError for %q, got %T: %v", blank, err, err)
		}
	}
}
