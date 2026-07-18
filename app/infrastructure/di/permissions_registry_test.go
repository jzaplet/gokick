package di

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// F-040: providePermissionsRegistry is a hand-maintained enrollment list — a new
// command/query that declares RequiredPermission but is forgotten there silently
// blanks that permission for the whole frontend. This gate walks app/application,
// finds every RequiredPermission() declaration + its permission string, and
// asserts the FE-facing ones (those NOT marked shared.CLIOnly) each appear in
// providePermissionsRegistry().All() — and that the registry carries nothing extra.
// It fails on a forgotten enrollment (declared but not registered) or a stale
// phantom (registered but nothing declares it, or a CLI-only leak).
func TestProvidePermissionsRegistry_EnrollsEveryFEFacingPermission(t *testing.T) {
	perms, cliOnly := scanRequiredPermissions(t, filepath.Join("..", "..", "application"))

	want := map[string]bool{}
	for typeName, perm := range perms {
		if !cliOnly[typeName] {
			want[perm] = true
		}
	}

	got := map[string]bool{}
	for _, p := range providePermissionsRegistry().All() {
		got[p] = true
	}

	for perm := range want {
		if !got[perm] {
			t.Errorf(
				"permission %q is declared by a non-CLI command/query but is NOT enrolled in providePermissionsRegistry",
				perm,
			)
		}
	}
	for perm := range got {
		if !want[perm] {
			t.Errorf(
				"registry carries %q but no non-CLI command/query declares it (stale phantom / CLI-only leak)",
				perm,
			)
		}
	}
}

// scanRequiredPermissions walks dir for production .go sources and returns, keyed
// by receiver type name: the permission string of each RequiredPermission() method,
// and the set of types that also declare a CLIOnly() marker method.
func scanRequiredPermissions(t *testing.T, dir string) (map[string]string, map[string]bool) {
	t.Helper()
	perms := map[string]string{}
	cliOnly := map[string]bool{}
	fset := token.NewFileSet()

	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return perr
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}
			recv := receiverTypeName(fn.Recv.List[0].Type)
			switch fn.Name.Name {
			case "CLIOnly":
				cliOnly[recv] = true
			case "RequiredPermission":
				lit := returnStringLiteral(fn)
				if lit == "" {
					t.Fatalf(
						"%s: %s.RequiredPermission is not a simple string-literal return — this gate can't read it; revisit it",
						path,
						recv,
					)
				}
				perms[recv] = lit
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("scan %s: %v", dir, walkErr)
	}
	if len(perms) == 0 {
		t.Fatalf(
			"scanned %s but found no RequiredPermission declarations — is the path wrong?",
			dir,
		)
	}
	return perms, cliOnly
}

func receiverTypeName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		if id, ok := e.X.(*ast.Ident); ok {
			return id.Name
		}
	}
	return ""
}

func returnStringLiteral(fn *ast.FuncDecl) string {
	if fn.Body == nil || len(fn.Body.List) != 1 {
		return ""
	}
	ret, ok := fn.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return ""
	}
	lit, ok := ret.Results[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return ""
	}
	return s
}

// F-037: CLI-only permissions must NOT leak into the FE-facing PermissionsRegistry.
// This is the OUTCOME guard — it exercises the REAL provider (every command wired
// in), so it fails if someone adds a CLI-only command without the shared.CLIOnly
// marker, or drops a marker. The mechanism (the filter itself) is covered by a
// unit test in domain/shared; this pins the concrete perms that must stay out.
func TestProvidePermissionsRegistry_ExcludesCLIOnlyPermissions(t *testing.T) {
	all := providePermissionsRegistry().All()

	// These come only from CLI-only commands (create-superadmin, get-tenant) — no
	// HTTP route, so the frontend can never invoke them.
	//
	// The list shrank when the superadmin plane grew tenant/user creation over
	// HTTP: platform:tenants:create left it because create-tenant now serves both
	// buses (the CLI's and the superadmin's), and platform:users:create was never
	// replaced here — it is create-superadmin that moved, to its own
	// platform:superadmins:create, precisely so this list could stay true. A
	// permission belongs here only while EVERY command declaring it is CLI-only.
	cliOnly := []string{
		"platform:superadmins:create",
		"platform:tenants:read",
	}
	for _, perm := range cliOnly {
		if slices.Contains(all, perm) {
			t.Errorf("CLI-only permission %q leaked into the FE-facing registry: %v", perm, all)
		}
	}

	// Sanity: HTTP-reachable permissions ARE present, so the registry isn't empty
	// by accident (which would make the exclusion assertion pass vacuously). The
	// two creates are named explicitly because they are the ones that just crossed
	// over — a lost CLIOnly drop would silently blank them on the frontend.
	for _, perm := range []string{
		"platform:overview",
		"platform:users:create",
		"platform:tenants:create",
	} {
		if !slices.Contains(all, perm) {
			t.Fatalf("expected FE-facing permission %q in registry, got %v", perm, all)
		}
	}
}
