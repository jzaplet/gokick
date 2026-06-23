package domain_test

// Born-scoped factory gate (static).
//
// A domain factory for a tenant-owned entity must receive the tenant as a
// PARAMETER — it must never hard-code it (e.g. shared.DefaultTenantID). The
// column users.tenant_id is NOT NULL with an FK and no DB default, so a factory
// that synthesizes a default tenant lets a caller SILENTLY persist a row into
// the wrong tenant when multitenancy is on: no panic, no failing query, and the
// per-query conformance gate (infrastructure/sqlite/zz_tenant_test.go) only
// checks that a query REFERENCES tenant_id, not that the value is correct. This
// gate closes that gap at the source — omit the tenant and this test bites (and
// after the fix, the factory signature won't compile without it).
//
// It pairs with the runtime fail-closed machinery (BaseRepository.Tenant(ctx)
// panics on a missing tenant in MT mode): born scoped, stays scoped.
//
// Scope: only factories that SET a TenantID field are constrained. A factory
// that leaves the field zero and has the tenant stamped later by an explicit,
// reviewed path (job.NewJob → dispatcher) is out of scope precisely because it
// makes no hidden default choice here.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const tenantIDField = "TenantID"

// factoryParamNames returns the set of parameter identifier names of a factory.
func factoryParamNames(fd *ast.FuncDecl) map[string]bool {
	names := map[string]bool{}
	if fd.Type.Params == nil {
		return names
	}
	for _, field := range fd.Type.Params.List {
		for _, n := range field.Names {
			names[n.Name] = true
		}
	}
	return names
}

// tenantIDAssignedValue returns the value expression assigned to a TenantID
// field by node n — via a composite-literal key (`TenantID: x`) or an
// assignment (`u.TenantID = x`) — and whether such an assignment was found.
func tenantIDAssignedValue(n ast.Node) (ast.Expr, bool) {
	switch e := n.(type) {
	case *ast.KeyValueExpr:
		if id, ok := e.Key.(*ast.Ident); ok && id.Name == tenantIDField {
			return e.Value, true
		}
	case *ast.AssignStmt:
		for i, lhs := range e.Lhs {
			if sel, ok := lhs.(*ast.SelectorExpr); ok && sel.Sel.Name == tenantIDField && i < len(e.Rhs) {
				return e.Rhs[i], true
			}
		}
	}
	return nil, false
}

// tenantExprName renders a tenant value expression for diagnostics (Ident,
// pkg.Selector, or a literal); anything else becomes "<expr>".
func tenantExprName(expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		if x, ok := v.X.(*ast.Ident); ok {
			return x.Name + "." + v.Sel.Name
		}
		return v.Sel.Name
	case *ast.BasicLit:
		return v.Value
	default:
		return "<expr>"
	}
}

// factoryTenantViolations returns a message for every New* factory in the source
// that assigns the TenantID field from anything other than one of its own
// parameters (i.e. a hard-coded tenant). src is nil to read the file at name.
func factoryTenantViolations(t *testing.T, name string, src any) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), name, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	var out []string
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Body == nil || !strings.HasPrefix(fd.Name.Name, "New") {
			continue
		}
		params := factoryParamNames(fd)
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			val, found := tenantIDAssignedValue(n)
			if !found {
				return true
			}
			if id, ok := val.(*ast.Ident); ok && params[id.Name] {
				return true // born scoped: the tenant came in as a parameter
			}
			out = append(out, fmt.Sprintf(
				"factory %s hard-codes TenantID (= %s); a tenant-owned entity factory "+
					"must take the tenant as a parameter so it can never be silently defaulted",
				fd.Name.Name, tenantExprName(val)))
			return true
		})
	}
	return out
}

// Every tenant-owned entity factory in the domain takes its tenant as a
// parameter. A factory that hard-codes shared.DefaultTenantID (or any constant)
// fails here.
func TestBornScoped_FactoriesTakeTenantParam(t *testing.T) {
	root := domainDir(t)
	var violations []string
	parsed := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed++
		for _, v := range factoryTenantViolations(t, path, nil) {
			violations = append(violations, fmt.Sprintf("%s: %s", filepath.Base(path), v))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk domain: %v", err)
	}
	if parsed == 0 {
		t.Fatal(
			"no domain .go files were parsed; the walk found nothing (gate would pass vacuously)",
		)
	}
	if len(violations) > 0 {
		t.Fatalf("born-scoped violations:\n  %s", strings.Join(violations, "\n  "))
	}
}

// The gate bites a factory that hard-codes the tenant...
func TestBornScoped_FlagsHardcodedTenant(t *testing.T) {
	const fake = "package fake\n\n" +
		"type User struct{ TenantID string }\n\n" +
		"func NewUser(nickname string) *User {\n" +
		"\treturn &User{TenantID: shared.DefaultTenantID}\n" +
		"}\n"
	if len(factoryTenantViolations(t, "fake.go", fake)) == 0 {
		t.Fatal("a factory hard-coding TenantID must be a violation; the gate does not bite")
	}
}

// ...and accepts a factory that takes the tenant as a parameter.
func TestBornScoped_AcceptsParamTenant(t *testing.T) {
	const fake = "package fake\n\n" +
		"type User struct{ TenantID string }\n\n" +
		"func NewUser(nickname, tenantID string) *User {\n" +
		"\treturn &User{TenantID: tenantID}\n" +
		"}\n"
	if v := factoryTenantViolations(t, "fake.go", fake); len(v) != 0 {
		t.Fatalf("a factory taking the tenant as a parameter must pass, got %v", v)
	}
}
