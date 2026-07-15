// Package boundary (the `gk boundary` subcommand) closes the wire-boundary
// loop that tsgen opened (gokick-roadmap follow-up ②): tsgen guarantees that
// every ANNOTATED Go DTO matches its TS type, but it is opt-in — a handler
// that responds with an inline map, `any`, or an unannotated struct simply
// bypasses it. This check makes the annotation mandatory at the boundary:
//
//   - every payload passed to (*response.Responder).JSON, and
//   - every destination passed to request.DecodeJSON
//
// must be a named struct carrying a //gkts: directive (slices/pointers/aliases
// of one are fine — tsgen maps them). Everything else fails the lint.
//
// Both rules match on TYPE identity (the method's receiver / the function's
// package), never on variable names — `h.resp.JSON`, `r.JSON`, any alias is
// caught, and taking a METHOD VALUE (`send := resp.JSON`) is a violation in
// itself, because a payload sent through the copied value would dodge the
// check. Going AROUND the boundary inside the wire layer (presentation/http/**
// minus the response/ and request/ plumbing) fails too: direct encoding/json
// calls and package-level response.* functions are violations, and inside
// handler/** so are raw writes to the http.ResponseWriter (w.Write,
// io.WriteString, fmt.Fprint*) — a handler cannot hand-encode a payload the
// gate never sees.
//
// Escape hatch — same discipline as the raw-pool exemptions: a comment
//
//	//gkts:ignore <reason>
//
// standing ALONE on the line directly above the offending call. The reason is
// mandatory and a marker that shields nothing is itself a violation, so stale
// escapes cannot rot in place. The escape covers payload-shape violations and
// raw handler writes (non-SPA endpoints: infra /health, APP_RUN_DEBUG E2E
// endpoints, the embedded SPA shell) — it does NOT silence encoding/json or
// package-level response.* bypasses, which have no legitimate wire-layer use.
// CALL-SITE ONLY, deliberately: a type-level ignore would sit in the type's
// doc where tsgen parses directives, and tsgen would read "ignore" as an
// output path.
//
// Drift guards — a gate that sees nothing must scream, not stay green:
//
//   - zero Responder.JSON sites or zero DecodeJSON sites is a hard failure
//     (per kind — losing one kind to a rename/move must not hide behind the
//     other kind's count),
//   - a JSON/DecodeJSON call whose arity is not the expected one is a
//     violation (signature drift would otherwise silently unpin the matcher),
//   - a new exported Responder method with an `any` parameter is a violation
//     until this gate is taught about it (a payload route the gate does not
//     check must not exist).
package boundary

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

	"gokick-gk/internal/tool"
)

const (
	responderPath = "gokick/app/presentation/http/response"
	requestPath   = "gokick/app/presentation/http/request"
	wireLayer     = "gokick/app/presentation/http/"
	handlerPath   = "gokick/app/presentation/http/handler"
	decodeJSONFn  = requestPath + ".DecodeJSON"
	ignoreMarker  = "gkts:ignore"
	gktsMarker    = "gkts:"

	jsonArity   = 4 // JSON(ctx, w, status, payload)
	decodeArity = 3 // DecodeJSON(w, r, dst)
)

// Run is the `gk boundary` entrypoint; args are the words after "boundary".
func Run(args []string) {
	if len(args) > 0 {
		fatal("unknown argument %q (gk boundary takes none)", args[0])
	}
	root, err := tool.RepoRoot()
	if err != nil {
		fatal("%v", err)
	}
	result, err := run(root)
	if err != nil {
		fatal("%v", err)
	}
	if result.jsonSites == 0 {
		fatal("no Responder.JSON call sites found — the scan is broken, not the code")
	}
	if result.decodeSites == 0 {
		fatal("no request.DecodeJSON call sites found — the scan is broken, not the code")
	}
	if len(result.violations) > 0 {
		sort.Strings(result.violations)
		fmt.Fprintf(os.Stderr, "boundary: %d violation(s):\n  %s\n",
			len(result.violations), strings.Join(result.violations, "\n  "))
		os.Exit(1)
	}
	fmt.Printf("boundary: %d wire call site(s) OK\n", result.jsonSites+result.decodeSites)
}

type result struct {
	violations  []string
	jsonSites   int
	decodeSites int
}

// run loads the app packages rooted at root and returns every violation plus
// the per-kind site counts. Split from Run so the whole pipeline is testable
// against a fixture module.
func run(root string) (*result, error) {
	pkgs, err := load(root)
	if err != nil {
		return nil, err
	}
	res := &result{}
	res.violations = append(res.violations, responderSurface(pkgs)...)
	gkts := annotatedTypes(pkgs)
	check(pkgs, gkts, res)
	return res, nil
}

func load(root string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		// No NeedDeps on purpose: only the root gokick/app/... packages are
		// walked, and cross-package type identity resolves via export data —
		// type-checking the whole dependency closure was measured ~8x slower
		// for an identical result.
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo,
		Dir: root,
	}
	pkgs, err := packages.Load(cfg, "gokick/app/...")
	if err != nil {
		return nil, fmt.Errorf("load: %w", err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		return nil, fmt.Errorf("packages contain errors")
	}
	return pkgs, nil
}

// responderSurface guards the matcher's anchor: the Responder must expose
// exactly one payload-carrying method (JSON, with the pinned arity). A new
// exported method with an `any` parameter would be a payload route this gate
// never checks, so it is a violation until the gate is taught about it.
func responderSurface(pkgs []*packages.Package) []string {
	var violations []string
	for _, pkg := range pkgs {
		if pkg.PkgPath != responderPath {
			continue
		}
		obj := pkg.Types.Scope().Lookup("Responder")
		if obj == nil {
			violations = append(violations,
				responderPath+": type Responder not found — the boundary anchor moved")
			continue
		}
		named, ok := types.Unalias(obj.Type()).(*types.Named)
		if !ok {
			continue
		}
		for i := range named.NumMethods() {
			m := named.Method(i)
			if !m.Exported() {
				continue
			}
			sig, ok := m.Type().(*types.Signature)
			if !ok {
				continue
			}
			if m.Name() == "JSON" {
				if sig.Params().Len() != jsonArity {
					violations = append(violations, fmt.Sprintf(
						"%s.JSON has %d parameters, gk boundary expects %d — "+
							"update the matcher before changing the signature",
						"Responder", sig.Params().Len(), jsonArity))
				}
				continue
			}
			if hasAnyParam(sig) {
				violations = append(violations, fmt.Sprintf(
					"Responder.%s takes an `any` parameter but gk boundary only checks "+
						"JSON — teach the gate about the new payload method", m.Name()))
			}
		}
	}
	return violations
}

func hasAnyParam(sig *types.Signature) bool {
	for i := range sig.Params().Len() {
		if iface, ok := types.Unalias(sig.Params().At(i).Type()).(*types.Interface); ok &&
			iface.Empty() {
			return true
		}
	}
	return false
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
					if !hasGktsDirective(gd.Doc) && !hasGktsDirective(ts.Doc) {
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

// hasGktsDirective reports whether the comment group contains a //gkts:<path>
// directive. //gkts:ignore is explicitly NOT a directive here. The text is
// whitespace-trimmed exactly like tsgen's parseDirective, so the two tools
// can never disagree on what counts as a directive.
func hasGktsDirective(doc *ast.CommentGroup) bool {
	if doc == nil {
		return false
	}
	for _, c := range doc.List {
		text := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
		if strings.HasPrefix(text, gktsMarker) && !strings.HasPrefix(text, ignoreMarker) {
			return true
		}
	}
	return false
}

// check walks every loaded file for the two boundary calls and validates the
// payload type; in the wire layer it also hunts the ways around the boundary.
func check(pkgs []*packages.Package, gkts map[types.Object]bool, res *result) {
	for _, pkg := range pkgs {
		inWire := strings.HasPrefix(pkg.PkgPath, wireLayer) &&
			pkg.PkgPath != responderPath && pkg.PkgPath != requestPath
		inHandler := pkg.PkgPath == handlerPath
		for _, f := range pkg.Syntax {
			checkFile(&fileCheck{
				pkg:       pkg,
				file:      f,
				gkts:      gkts,
				inWire:    inWire,
				inHandler: inHandler,
				res:       res,
			})
		}
	}
}

type fileCheck struct {
	pkg       *packages.Package
	file      *ast.File
	gkts      map[types.Object]bool
	inWire    bool
	inHandler bool
	res       *result
}

func checkFile(fc *fileCheck) {
	fset := fc.pkg.Fset
	filename := fset.Position(fc.file.Pos()).Filename
	src, err := os.ReadFile(filename)
	if err != nil {
		fc.res.violations = append(fc.res.violations,
			fmt.Sprintf("%s: cannot re-read source for escape markers: %v", filename, err))
		return
	}
	ignores, misplaced := tool.EscapeLines(fset, fc.file, src, ignoreMarker)
	for _, line := range misplaced {
		fc.res.violations = append(fc.res.violations, fmt.Sprintf(
			"%s:%d: //gkts:ignore must stand alone on the line above the call — "+
				"a trailing marker would silently cover the NEXT line too", filename, line))
	}
	for _, esc := range ignores {
		if esc.Reason == "" {
			fc.res.violations = append(fc.res.violations, fmt.Sprintf(
				"%s:%d: //gkts:ignore without a reason — say WHY this endpoint may "+
					"bypass the codegen", filename, esc.MarkerLine))
		}
	}
	consumed := map[int]bool{}
	// report records a violation unless an escape covers it. Only ignorable
	// violation kinds (payload shape, raw handler writes) honor the escape —
	// encoding/json and free-function bypasses stay violations regardless.
	report := func(pos token.Position, msg string, ignorable bool) {
		if esc, ok := ignores[pos.Line]; ok && ignorable {
			consumed[esc.MarkerLine] = true
			return // reasonless markers were already reported above
		}
		fc.res.violations = append(fc.res.violations,
			fmt.Sprintf("%s:%d: %s", pos.Filename, pos.Line, msg))
	}
	fc.walk(report)
	for _, esc := range ignores {
		if esc.Reason != "" && !consumed[esc.MarkerLine] {
			fc.res.violations = append(fc.res.violations, fmt.Sprintf(
				"%s:%d: unused //gkts:ignore — nothing on the next line needs it; "+
					"remove the stale marker", filename, esc.MarkerLine))
		}
	}
}

func (fc *fileCheck) walk(report func(token.Position, string, bool)) {
	fset := fc.pkg.Fset
	callFuns := map[*ast.SelectorExpr]bool{}
	ast.Inspect(fc.file, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok && !callFuns[sel] {
			if kind := boundaryFn(fc.pkg, sel); kind != "" {
				report(fset.Position(sel.Pos()), fmt.Sprintf(
					"method value of %s — a payload sent through the copied value "+
						"dodges the gate; call it directly", kind), false)
			}
			return true
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			callFuns[sel] = true
		}
		if fc.checkBoundaryCall(call, report) {
			return true
		}
		if fc.inWire {
			if bypass := bypassCall(fc.pkg, call); bypass != "" {
				report(fset.Position(call.Pos()), bypass, false)
			}
		}
		if fc.inHandler {
			if raw := rawWrite(fc.pkg, call); raw != "" {
				report(fset.Position(call.Pos()), raw, true)
			}
		}
		return true
	})
}

// checkBoundaryCall handles a Responder.JSON / request.DecodeJSON call; it
// reports true when the call was one (so the walker skips the bypass rules).
func (fc *fileCheck) checkBoundaryCall(
	call *ast.CallExpr,
	report func(token.Position, string, bool),
) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	kind := boundaryFn(fc.pkg, sel)
	if kind == "" {
		return false
	}
	pos := fc.pkg.Fset.Position(call.Pos())
	arity := jsonArity
	payloadIdx := 3
	if kind == "request.DecodeJSON" {
		arity = decodeArity
		payloadIdx = 2
	}
	if len(call.Args) != arity {
		report(pos, fmt.Sprintf(
			"%s call with %d args, gk boundary expects %d — signature drift would "+
				"silently unpin the matcher; update the gate", kind, len(call.Args), arity),
			false)
		return true
	}
	if kind == "Responder.JSON" {
		fc.res.jsonSites++
	} else {
		fc.res.decodeSites++
	}
	if ok, why := payloadOK(fc.pkg, call.Args[payloadIdx], fc.gkts); !ok {
		report(pos, fmt.Sprintf(
			"%s payload %s — wire types must be named structs with a "+
				"//gkts: directive (or //gkts:ignore <reason> for non-SPA endpoints)",
			kind, why), true)
	}
	return true
}

// boundaryFn classifies a selector as one of the two boundary functions by
// type identity ("" when it is neither) — shared by the call matcher and the
// method-value detector.
func boundaryFn(pkg *packages.Package, sel *ast.SelectorExpr) string {
	fn, ok := pkg.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok {
		return ""
	}
	switch {
	case fn.Name() == "JSON" && receiverIsResponder(fn):
		return "Responder.JSON"
	case fn.FullName() == decodeJSONFn:
		return "request.DecodeJSON"
	}
	return ""
}

// bypassCall names the violation when a wire-layer call goes around the typed
// boundary: any encoding/json call (Marshal, NewEncoder(w).Encode, …) or a
// package-level function of the response package. Returns "" when fine.
func bypassCall(pkg *packages.Package, call *ast.CallExpr) string {
	var ident *ast.Ident
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		ident = fun.Sel
	case *ast.Ident:
		ident = fun
	default:
		return ""
	}
	fn, ok := pkg.TypesInfo.Uses[ident].(*types.Func)
	if !ok || fn.Pkg() == nil {
		return ""
	}
	switch fn.Pkg().Path() {
	case "encoding/json":
		return fmt.Sprintf(
			"direct encoding/json call (%s) in the wire layer — encode/decode must go "+
				"through resp.JSON / request.DecodeJSON so payloads stay gkts-typed",
			fn.Name())
	case responderPath:
		sig, ok := fn.Type().(*types.Signature)
		if ok && sig.Recv() == nil && !returnsResponder(sig) {
			return fmt.Sprintf(
				"package-level response.%s call — the wire boundary is the *Responder "+
					"methods; a free function would dodge the payload check", fn.Name())
		}
	}
	return ""
}

// rawWrite names the violation when handler code writes to the
// http.ResponseWriter directly (w.Write, io.WriteString, fmt.Fprint*) — a
// hand-built payload the gate never sees. Middleware is deliberately out of
// scope: ResponseWriter wrappers and the recovery panic path write raw by
// design.
func rawWrite(pkg *packages.Package, call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		fn, ok := pkg.TypesInfo.Uses[fun.Sel].(*types.Func)
		if !ok {
			return ""
		}
		if fn.Name() == "Write" && isResponseWriter(pkg, fun.X) {
			return "raw w.Write in a handler — JSON payloads must go through resp.JSON " +
				"(//gkts:ignore <reason> for non-JSON responses like the SPA shell)"
		}
		if fn.Pkg() == nil {
			return ""
		}
		pkgPath, name := fn.Pkg().Path(), fn.Name()
		writer := pkgPath == "io" && name == "WriteString"
		printer := pkgPath == "fmt" && strings.HasPrefix(name, "Fprint")
		if (writer || printer) && len(call.Args) > 0 && isResponseWriter(pkg, call.Args[0]) {
			return fmt.Sprintf(
				"raw %s.%s to the ResponseWriter in a handler — JSON payloads must go "+
					"through resp.JSON (//gkts:ignore <reason> for non-JSON responses)",
				pkgPath, name)
		}
	}
	return ""
}

func isResponseWriter(pkg *packages.Package, e ast.Expr) bool {
	t := pkg.TypesInfo.TypeOf(e)
	if t == nil {
		return false
	}
	named, ok := types.Unalias(t).(*types.Named)
	return ok && named.Obj().Name() == "ResponseWriter" &&
		named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "net/http"
}

// returnsResponder reports whether the signature returns a *Responder —
// constructors (NewResponder) hand out the boundary, they don't write to it.
func returnsResponder(sig *types.Signature) bool {
	for i := range sig.Results().Len() {
		t := types.Unalias(sig.Results().At(i).Type())
		if ptr, ok := t.(*types.Pointer); ok {
			t = types.Unalias(ptr.Elem())
		}
		if named, ok := t.(*types.Named); ok && named.Obj().Name() == "Responder" {
			return true
		}
	}
	return false
}

func receiverIsResponder(fn *types.Func) bool {
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() == nil {
		return false
	}
	t := types.Unalias(sig.Recv().Type())
	if ptr, ok := t.(*types.Pointer); ok {
		t = types.Unalias(ptr.Elem())
	}
	named, ok := t.(*types.Named)
	return ok && named.Obj().Name() == "Responder" &&
		named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == responderPath
}

// payloadOK unwraps pointers/slices/aliases and requires a named struct whose
// declaration carries a //gkts: directive. Returns why not on failure.
func payloadOK(pkg *packages.Package, arg ast.Expr, gkts map[types.Object]bool) (bool, string) {
	t := pkg.TypesInfo.TypeOf(arg)
	if t == nil {
		return false, "has no resolvable type"
	}
	orig := t
	for unwrapping := true; unwrapping; {
		t = types.Unalias(t)
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

func fatal(format string, args ...any) {
	tool.Fatalf("boundary", format, args...)
}
