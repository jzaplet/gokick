// Package boundary (the `gk boundary` subcommand) closes the wire-boundary
// loop that tsgen opened (gokick-roadmap follow-up ②): tsgen guarantees that
// every ANNOTATED Go DTO matches its TS type, but it is opt-in — a handler
// that responds with an inline map, `any`, or an unannotated struct simply
// bypasses it. This check makes the annotation mandatory at the boundary:
//
//   - every payload passed to (*response.Responder).JSON, and
//   - every destination passed to request.DecodeJSON
//
// must be a named struct carrying a //gkts: directive (slices/pointers of one
// are fine — tsgen maps them). Everything else fails the lint.
//
// Escape hatch — same discipline as the raw-pool exemptions: a comment
//
//	//gkts:ignore <reason>
//
// on the line directly above the offending call. The reason is mandatory; a
// bare marker fails. CALL-SITE ONLY, deliberately: a type-level ignore would
// sit in the type's doc where tsgen parses directives, and tsgen would read
// "ignore" as an output path. Intended for wire responses the SPA never
// consumes (infra /health, APP_RUN_DEBUG E2E endpoints) — NOT for skipping
// the codegen on real app endpoints.
//
// Zero-sites guard: if the scan finds no JSON/DecodeJSON call sites at all,
// the check FAILS instead of passing silently — a gate that sees nothing
// (moved packages, renamed methods) must scream, not stay green (the same
// lesson as tsgen's orphan-file pruning).
package boundary

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

const (
	responderPath = "gokick/app/presentation/http/response"
	decodeJSONFn  = "gokick/app/presentation/http/request.DecodeJSON"
	ignoreMarker  = "gkts:ignore"
	gktsMarker    = "gkts:"
)

// Run is the `gk boundary` entrypoint; args are the words after "boundary".
func Run(args []string) {
	if len(args) > 0 {
		fatal("unknown argument %q (gk boundary takes none)", args[0])
	}
	pkgs := load()
	gkts := annotatedTypes(pkgs)
	violations, sites := check(pkgs, gkts)
	if sites == 0 {
		fatal("no JSON/DecodeJSON call sites found — the scan is broken, not the code")
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		fmt.Fprintf(os.Stderr, "boundary: %d violation(s):\n  %s\n",
			len(violations), strings.Join(violations, "\n  "))
		os.Exit(1)
	}
	fmt.Printf("boundary: %d wire call site(s) OK\n", sites)
}

func load() []*packages.Package {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedDeps,
		Dir: repoRoot(),
	}
	pkgs, err := packages.Load(cfg, "gokick/app/...")
	if err != nil {
		fatal("load: %v", err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		fatal("packages contain errors")
	}
	return pkgs
}

// annotatedTypes collects every named type whose declaration carries a real
// //gkts: directive (the tsgen annotation), keyed by its types.Object. Both
// the TypeSpec doc and the enclosing GenDecl doc count — gofmt moves the
// comment to the GenDecl for single-spec decls. A //gkts:ignore in a type doc
// does NOT count: the escape is call-site only (see the package doc).
func annotatedTypes(pkgs []*packages.Package) map[types.Object]bool {
	marked := map[types.Object]bool{}
	for _, pkg := range pkgs {
		for _, f := range pkg.Syntax {
			for _, decl := range f.Decls {
				gd, ok := decl.(*ast.GenDecl)
				if !ok || gd.Tok != token.TYPE {
					continue
				}
				for _, spec := range gd.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					if !hasMarker(gd.Doc, gktsMarker) && !hasMarker(ts.Doc, gktsMarker) {
						continue
					}
					if obj := pkg.TypesInfo.Defs[ts.Name]; obj != nil {
						marked[obj] = true
					}
				}
			}
		}
	}
	return marked
}

// hasMarker reports whether the comment group contains a //gkts:<path>
// directive. //gkts:ignore is explicitly NOT a directive here.
func hasMarker(doc *ast.CommentGroup, marker string) bool {
	if doc == nil {
		return false
	}
	for _, c := range doc.List {
		text := strings.TrimPrefix(c.Text, "//")
		if strings.HasPrefix(text, marker) && !strings.HasPrefix(text, ignoreMarker) {
			return true
		}
	}
	return false
}

// check walks every loaded file for the two boundary calls and validates the
// payload type. Returns the violation messages and the number of call sites
// seen (for the zero-sites guard).
func check(pkgs []*packages.Package, gkts map[types.Object]bool) ([]string, int) {
	var violations []string
	sites := 0
	for _, pkg := range pkgs {
		for _, f := range pkg.Syntax {
			ignores := ignoreLines(pkg.Fset, f)
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				arg, kind := boundaryArg(pkg, call)
				if arg == nil {
					return true
				}
				sites++
				if ok, why := payloadOK(pkg, arg, gkts); !ok {
					pos := pkg.Fset.Position(call.Pos())
					if reason, exempt := ignores[pos.Line]; exempt {
						if strings.TrimSpace(reason) == "" {
							violations = append(violations, fmt.Sprintf(
								"%s:%d: //gkts:ignore without a reason — say WHY this "+
									"endpoint may bypass the codegen", pos.Filename, pos.Line))
						}
						return true
					}
					violations = append(violations, fmt.Sprintf(
						"%s:%d: %s payload %s — wire types must be named structs with a "+
							"//gkts: directive (or //gkts:ignore <reason> for non-SPA endpoints)",
						pos.Filename, pos.Line, kind, why))
				}
				return true
			})
		}
	}
	return violations, sites
}

// boundaryArg returns the payload argument of a boundary call (and which kind
// of call it is), or nil when the call is not one of the two boundary calls.
func boundaryArg(pkg *packages.Package, call *ast.CallExpr) (ast.Expr, string) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, ""
	}
	obj := pkg.TypesInfo.Uses[sel.Sel]
	if obj == nil {
		return nil, ""
	}
	fn, ok := obj.(*types.Func)
	if !ok {
		return nil, ""
	}
	switch {
	case fn.Name() == "JSON" && receiverIsResponder(fn) && len(call.Args) == 4:
		return call.Args[3], "Responder.JSON"
	case fn.FullName() == decodeJSONFn && len(call.Args) == 3:
		return call.Args[2], "request.DecodeJSON"
	}
	return nil, ""
}

func receiverIsResponder(fn *types.Func) bool {
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() == nil {
		return false
	}
	t := sig.Recv().Type()
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	return ok && named.Obj().Name() == "Responder" &&
		named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == responderPath
}

// payloadOK unwraps pointers/slices and requires a named struct whose
// declaration carries a //gkts: directive. Returns why not on failure.
func payloadOK(pkg *packages.Package, arg ast.Expr, gkts map[types.Object]bool) (bool, string) {
	t := pkg.TypesInfo.TypeOf(arg)
	if t == nil {
		return false, "has no resolvable type"
	}
	orig := t
	for unwrapping := true; unwrapping; {
		switch u := t.(type) {
		case *types.Pointer:
			t = u.Elem()
		case *types.Slice:
			t = u.Elem()
		case *types.Array:
			t = u.Elem()
		default:
			unwrapping = false
		}
	}
	named, ok := t.(*types.Named)
	if !ok {
		return false, fmt.Sprintf("is %s, not a named struct", orig)
	}
	if _, isStruct := named.Underlying().(*types.Struct); !isStruct {
		return false, fmt.Sprintf("%s is not a struct", named.Obj().Name())
	}
	if !gkts[named.Obj()] {
		return false, fmt.Sprintf("%s has no //gkts: directive", named.Obj().Name())
	}
	return true, ""
}

// ignoreLines maps each line covered by a //gkts:ignore comment — the marker
// line itself and the line right after it — to the ignore reason, so a marker
// directly above a call (or a multi-line call starting there) exempts it.
func ignoreLines(fset *token.FileSet, f *ast.File) map[int]string {
	lines := map[int]string{}
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			text := strings.TrimPrefix(c.Text, "//")
			if !strings.HasPrefix(text, ignoreMarker) {
				continue
			}
			reason := strings.TrimPrefix(text, ignoreMarker)
			line := fset.Position(c.Pos()).Line
			lines[line] = reason
			lines[line+1] = reason
		}
	}
	return lines
}

// repoRoot resolves the gokick module root by walking up from the working
// directory (gk runs from tools/gk).
func repoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		fatal("getwd: %v", err)
	}
	for {
		mod := filepath.Join(dir, "go.mod")
		if b, err := os.ReadFile(mod); err == nil &&
			strings.Contains(string(b), "module gokick\n") {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			fatal("could not find gokick go.mod above %s", dir)
		}
		dir = parent
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "boundary: "+format+"\n", args...)
	os.Exit(1)
}
