package application_test

// Platform-isolation gate (static).
//
// The cross-tenant escape-hatch methods on user.Repository (named *AcrossTenants
// — FindPageAcrossTenants, CountAcrossTenants, UpdateAcrossTenants,
// DeleteAcrossTenants) DELIBERATELY bypass tenant scoping. They are the
// superadmin platform plane and may be called ONLY from application/platform/**.
//
// A non-platform handler calling one would read or modify across tenants, and
// neither existing defence catches it:
//   - the permission gate checks a command's RequiredPermission(), not which
//     repository methods its handler invokes;
//   - the per-query conformance gate admits these queries on purpose (they carry
//     the tenant-scope-exempt marker).
// Only code review stands in the way — so this gate turns such a call into a
// build failure.
//
// This is the pre-segregation guard. The fix introduces a dedicated
// user.PlatformRepository port so the compiler ALSO enforces the boundary; this
// gate then stays as belt-and-suspenders against a future re-merge of the ports.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const crossTenantSuffix = "AcrossTenants"

// applicationDir resolves app/application/ from this test file's location, so the
// walk is independent of the working directory.
func applicationDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed; cannot locate test source file")
	}
	dir := filepath.Dir(thisFile)
	if filepath.Base(dir) != "application" || filepath.Base(filepath.Dir(dir)) != "app" {
		t.Fatalf("expected this test to live in .../app/application, got %q", dir)
	}
	return dir
}

// crossTenantCalls returns the names of *AcrossTenants methods invoked in the Go
// source. src is nil to read the file at name. Matching is by method-name suffix
// so any future cross-tenant method following the naming convention is covered.
func crossTenantCalls(t *testing.T, name string, src any) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), name, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok &&
			strings.HasSuffix(sel.Sel.Name, crossTenantSuffix) {
			out = append(out, sel.Sel.Name)
		}
		return true
	})
	return out
}

// isPlatformPath reports whether a file lives under application/platform/ — the
// one plane allowed to call the cross-tenant escape hatches.
func isPlatformPath(path string) bool {
	return strings.Contains(filepath.ToSlash(path), "/platform/")
}

// No application package outside platform/ calls a cross-tenant escape-hatch
// method. A non-platform caller fails here.
func TestPlatformIsolation_NonPlatformMustNotCallCrossTenant(t *testing.T) {
	root := applicationDir(t)
	var violations []string
	parsed := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if isPlatformPath(path) {
			return nil // the platform plane is the sanctioned caller
		}
		parsed++
		for _, m := range crossTenantCalls(t, path, nil) {
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				rel = path
			}
			violations = append(violations, fmt.Sprintf("%s calls %s", rel, m))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk application: %v", err)
	}
	if parsed == 0 {
		t.Fatal("no non-platform application .go files were parsed; the walk found nothing")
	}
	if len(violations) > 0 {
		t.Fatalf(
			"cross-tenant escape-hatch called outside application/platform/:\n  %s\n"+
				"these *AcrossTenants methods bypass tenant scoping — confine them to the platform plane",
			strings.Join(violations, "\n  "))
	}
}

// The gate bites a non-platform caller...
func TestPlatformIsolation_FlagsCrossTenantCall(t *testing.T) {
	const fake = "package fake\n\n" +
		"func run(r repo, ctx ctx) {\n" +
		"\t_, _ = r.FindPageAcrossTenants(ctx)\n" +
		"}\n"
	if len(crossTenantCalls(t, "fake.go", fake)) == 0 {
		t.Fatal("a call to a *AcrossTenants method must be detected; the gate does not bite")
	}
}

// ...and ignores ordinary scoped calls.
func TestPlatformIsolation_IgnoresScopedCall(t *testing.T) {
	const fake = "package fake\n\n" +
		"func run(r repo, ctx ctx) {\n" +
		"\t_, _ = r.FindAll(ctx)\n" +
		"}\n"
	if c := crossTenantCalls(t, "fake.go", fake); len(c) != 0 {
		t.Fatalf("a scoped call must not be flagged, got %v", c)
	}
}

// platformPorts are the ports whose own methods are cross-tenant by definition.
var platformPorts = []struct{ pkg, iface string }{
	{"user", "PlatformRepository"},
	{"tenant", "PlatformRepository"},
}

// The gate above matches on the *AcrossTenants SUFFIX, so a platform method
// named without it is invisible to it — silently, at rename time. That is not
// hypothetical: tenant's overview read shipped as OverviewPage and dropped out
// of this gate until it was renamed back. The naming was a convention held up
// by a doc comment; this makes it the gate it claimed to be.
//
// Embedded ports (PlatformRepository embeds Repository) are the scoped half and
// are skipped — only methods DECLARED on the platform port must carry the suffix.
func TestPlatformIsolation_EveryPlatformPortMethodCarriesTheSuffix(t *testing.T) {
	domainDir := filepath.Join(filepath.Dir(applicationDir(t)), "domain")
	checked := 0

	for _, port := range platformPorts {
		path := filepath.Join(domainDir, port.pkg, "repository.go")
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}

		methods := interfaceMethods(f, port.iface)
		if len(methods) == 0 {
			t.Fatalf("found no methods on %s.%s — the gate is looking at the wrong "+
				"interface and would pass vacuously", port.pkg, port.iface)
		}
		for _, name := range methods {
			checked++
			if !strings.HasSuffix(name, crossTenantSuffix) {
				t.Errorf("%s.%s declares %q, which does not end in %q — the "+
					"isolation gate matches on that suffix, so this method bypasses "+
					"tenant scoping with nothing watching its call sites",
					port.pkg, port.iface, name, crossTenantSuffix)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no platform-port methods found at all; the gate does not bite")
	}
}

// interfaceMethods returns the names of methods DECLARED on the named interface
// (embedded interfaces carry no name and are skipped).
func interfaceMethods(f *ast.File, iface string) []string {
	var names []string
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || ts.Name.Name != iface {
			return true
		}
		it, ok := ts.Type.(*ast.InterfaceType)
		if !ok {
			return true
		}
		for _, field := range it.Methods.List {
			for _, name := range field.Names {
				names = append(names, name.Name)
			}
		}
		return false
	})
	return names
}
