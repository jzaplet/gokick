package app_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

// Postgres system-role gate.
//
// A Postgres repository statement runs where ctx says (BaseRepository.Conn): on
// the tenant plane under row-level security. Two calls step outside that on
// purpose — SystemConn (cross-tenant work) and the raw system pool r.DB.System()
// (a write that must commit on its own) — and both run on the role that bypasses
// row-level security. Every method that makes such a call is listed here, the
// Postgres counterpart of the SQLite adapter's raw-pool exceptions: a new one is a
// conscious decision in review, not a side effect. A platform-port method
// (*AcrossTenants) needs no entry — reaching every tenant is its contract, and
// only application/platform may call it (zz_platform_isolation_test.go).

var allowedSystemRoleMethods = []string{
	"user.FindByNickname",    // global identity lookup: login, nickname uniqueness
	"user.RecordLogin",       // raw pool: login stamps commit on their own
	"user.RecordFailedLogin", // raw pool: brute-force counter survives any rollback
	"user.ResetFailedLogin",  // raw pool: same
	"token.DeleteExpired",    // the scheduler's sweep over every tenant's tokens
	"run.ClaimDue",           // the worker drains every tenant's queue
	"run.RenewLease",         // the worker's heartbeat
	"run.fenced",             // the worker's owner-fenced writes (checkpoint, finalizers)
	"audit.Save",             // raw pool: audit survives the business rollback
}

// systemRoleCallers returns "<pkg>.<func>" for every function in the Go source
// (src nil = read the file at name) that calls SystemConn or System.
func systemRoleCallers(t *testing.T, pkg, name string, src any) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), name, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	var out []string
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		calls := false
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok &&
					(sel.Sel.Name == "SystemConn" || sel.Sel.Name == "System") {
					calls = true
				}
			}
			return !calls
		})
		if calls {
			out = append(out, pkg+"."+fn.Name.Name)
		}
	}
	return out
}

func systemRoleViolation(caller string) bool {
	return !strings.HasSuffix(caller, "AcrossTenants") &&
		!slices.Contains(allowedSystemRoleMethods, caller)
}

func TestPostgresSystemRole_OnlyListedMethods(t *testing.T) {
	root := adapterDir("postgres")
	var found, violations []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		// The repositories live in the sub-packages; the adapter's root defines
		// SystemConn itself.
		if err != nil || d.IsDir() || filepath.Dir(path) == root ||
			!strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		for _, c := range systemRoleCallers(t, filepath.Base(filepath.Dir(path)), path, nil) {
			found = append(found, c)
			if systemRoleViolation(c) {
				violations = append(violations, c)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Errorf("these repository methods run on the system role (SystemConn / "+
			"DB.System()), which bypasses row-level security, without being listed in "+
			"allowedSystemRoleMethods:\n  %s", strings.Join(violations, "\n  "))
	}
	// The list must not go stale: each entry still makes such a call.
	for _, want := range allowedSystemRoleMethods {
		if !slices.Contains(found, want) {
			t.Errorf(
				"allowedSystemRoleMethods lists %s, which no longer uses the system role",
				want,
			)
		}
	}
}

func TestPostgresSystemRole_GateBites(t *testing.T) {
	const src = `package user
func (r *Repository) FindAll(ctx context.Context) error { return r.SystemConn(ctx).Get() }
func (r *Repository) Save(ctx context.Context) error { return r.DB.System().Exec() }
func (r *Repository) CountAcrossTenants(ctx context.Context) error { return r.SystemConn(ctx).Get() }
func (r *Repository) Update(ctx context.Context) error { return r.Conn(ctx).Exec() }
`
	got := systemRoleCallers(t, "user", "x.go", src)
	want := []string{"user.FindAll", "user.Save", "user.CountAcrossTenants"}
	if !slices.Equal(got, want) {
		t.Fatalf("callers = %v, want %v", got, want)
	}
	var flagged []string
	for _, c := range got {
		if systemRoleViolation(c) {
			flagged = append(flagged, c)
		}
	}
	if fmt.Sprint(flagged) != "[user.FindAll user.Save]" {
		t.Fatalf("flagged %v, want the two unlisted methods only", flagged)
	}
}
