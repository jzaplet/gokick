package boundary

import (
	"path/filepath"
	"strings"
	"testing"
)

// The fixture module exercises every rule at once; the assertions pin both
// the presence of each violation class and the exact totals, so a rule that
// silently stops firing (or starts double-firing) breaks the test.
func TestFixture_AllRules(t *testing.T) {
	res, err := run(filepath.Join("testdata", "fixture"))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.jsonSites != 12 || res.decodeSites != 1 {
		t.Errorf("sites: got %d JSON / %d DecodeJSON, want 12 / 1",
			res.jsonSites, res.decodeSites)
	}
	joined := strings.Join(res.violations, "\n")
	for _, want := range []string{
		"is map[string]string, not a named struct",
		"plainDTO has no //gkts: directive",
		"without a reason",
		"must stand alone",
		"unused //gkts:ignore",
		"package-level response.Legacy",
		"direct encoding/json call (Marshal)",
		"direct encoding/json call (Valid)",
		"method value of Responder.JSON",
		"method value of request.DecodeJSON",
		"raw w.Write in a handler",
		"raw io.WriteString",
		"raw fmt.Fprint",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing violation %q in:\n%s", want, joined)
		}
	}
	if got := len(res.violations); got != 16 {
		t.Errorf("want exactly 16 violations, got %d:\n%s", got, joined)
	}
	for _, silenced := range []string{
		`"n": 1`,       // standalone marker with reason — suppressed
		"okDTO",        // annotated payloads never violate
		"NewResponder", // constructors hand out the boundary
		"<html>",       // ignored raw write
	} {
		if strings.Contains(joined, silenced) {
			t.Errorf("violation mentions %q — should have been clean/suppressed:\n%s",
				silenced, joined)
		}
	}
}

// The surface fixture drifts the anchor on purpose: JSON grew a parameter and
// Push is a new any-payload method. Matching must not just silently drop to
// zero — the drift itself has to be violations, and the per-kind counts the
// Run guards consume must read zero.
func TestSurfaceDrift_FailsLoudly(t *testing.T) {
	res, err := run(filepath.Join("testdata", "surface"))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.jsonSites != 0 || res.decodeSites != 0 {
		t.Errorf("sites: got %d JSON / %d DecodeJSON, want 0 / 0 (drifted signatures)",
			res.jsonSites, res.decodeSites)
	}
	joined := strings.Join(res.violations, "\n")
	for _, want := range []string{
		"Responder.JSON has 5 parameters",
		"Responder.Push takes an `any` parameter",
		"call with 5 args",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing drift violation %q in:\n%s", want, joined)
		}
	}
}
