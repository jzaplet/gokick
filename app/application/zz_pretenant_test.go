package application_test

// Pre-tenant gate (static).
//
// A command implementing shared.PreTenant runs on the system plane: on Postgres
// its transaction and statements bypass row-level security. That is right for the
// two commands that act before any tenant is known — login and refresh, which
// look the account up by a nickname or token unique across every tenant — and a
// cross-tenant hole anywhere else. The marker is one method, easy to paste onto
// the wrong command, so the implementers are an allow-list.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// allowedPreTenant lists the commands that may run on the system plane without a
// platform permission, as "<dir under application>.<type>".
var allowedPreTenant = []string{
	"auth/command.LoginCommand",
	"auth/command.RefreshTokenCommand",
}

// preTenantImplementers returns "<dir>.<receiver type>" for every PreTenant()
// method declared in the Go source (src nil = read the file at name).
func preTenantImplementers(t *testing.T, dir, name string, src any) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), name, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	var out []string
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Name.Name != "PreTenant" || len(fn.Recv.List) != 1 {
			continue
		}
		typ := fn.Recv.List[0].Type
		if star, ok := typ.(*ast.StarExpr); ok {
			typ = star.X
		}
		if id, ok := typ.(*ast.Ident); ok {
			out = append(out, dir+"."+id.Name)
		}
	}
	return out
}

func TestPreTenant_OnlyAllowListedCommandsRunOnTheSystemPlane(t *testing.T) {
	root := applicationDir(t)
	var found []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") {
			return err
		}
		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		found = append(found, preTenantImplementers(t, filepath.ToSlash(rel), path, nil)...)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	for _, impl := range found {
		if !slices.Contains(allowedPreTenant, impl) {
			t.Errorf("%s implements shared.PreTenant — it would run on the system plane, "+
				"across every tenant. Only commands that act before any tenant is known "+
				"may; add it to allowedPreTenant only if that is what it does", impl)
		}
	}
	// The allow-list must not go stale either: each entry still exists.
	for _, want := range allowedPreTenant {
		if !slices.Contains(found, want) {
			t.Errorf("allowedPreTenant lists %s, but no such implementer exists", want)
		}
	}
}

func TestPreTenant_FlagsAnImplementer(t *testing.T) {
	const src = `package command
type DeleteEverythingCommand struct{}
func (DeleteEverythingCommand) PreTenant() {}
func (*DeleteEverythingCommand) Other() {}
`
	got := preTenantImplementers(t, "user/command", "x.go", src)
	if !slices.Equal(got, []string{"user/command.DeleteEverythingCommand"}) {
		t.Fatalf("implementers = %v, want the one PreTenant method", got)
	}
}
