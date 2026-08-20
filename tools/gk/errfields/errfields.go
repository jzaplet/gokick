// Package errfields (the `gk errfields` subcommand) is the static half of the
// error-key parity loop (gokick-roadmap follow-up ④, without golden tests):
// the backend names field errors with string literals
// (shared.ValidationError{Field: "nickname"}) and the frontend mirrors them as
// optional keys on hand-written *Errors types ({ nickname?: string }). Nothing
// ties the two — a typo or a rename drifts silently and the error "disappears"
// into nothing. This check extracts BOTH sides and fails on:
//
//   - a Go Field literal with no home in any FE *Errors type (the error the
//     backend can emit would render nowhere), and
//   - an FE *Errors key (except the conventional `general`) no Go literal
//     produces (a phantom key that can never light up).
//
// The check is deliberately GLOBAL, not per-endpoint: matching a field to the
// exact form it belongs to would need call-graph analysis (handler → command →
// value object). Global catches the realistic drift class — typos and renames
// — and stays cheap; per-endpoint precision can layer on later.
//
// Escape hatch for Go fields that are real ValidationErrors but never render
// in a form (path-param lookups surfaced via redirect, CLI-only commands):
//
//	//gkerrf:exempt <reason>
//
// standing ALONE on the line directly above the statement holding the literal
// (the same semantics as //gkts:ignore — a trailing marker is a violation, a
// reasonless one too, and so is a marker that exempts nothing, so stale
// escapes cannot rot in place). The exemption is keyed on the line where the
// COMPOSITE LITERAL starts, so a golines rewrap of a long literal does not
// silently detach it.
//
// The collector matches by type NAME (plain parser, no type-checking — cheap),
// which two tripwires keep honest: declaring another `type ValidationError`
// under app/ outside domain/shared, or a `NewValidationError` constructor
// (whose call sites the literal scan could never see), is a violation until
// this tool is taught about them.
//
// Zero-sites guards: finding no Field literals, or no *Errors files, fails the
// check — a gate that sees nothing must scream, not stay green. And an
// *Errors-shaped type declared OUTSIDE a *Errors.ts file is a violation: the
// FE scan only reads *Errors.ts files, so an inline type would silently dodge
// the parity check.
package errfields

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gokick-gk/internal/tool"
)

const exemptMarker = "gkerrf:exempt"

// Run is the `gk errfields` entrypoint; args are the words after "errfields".
func Run(args []string) {
	if len(args) > 0 {
		fatal("unknown argument %q (gk errfields takes none)", args[0])
	}
	root, err := tool.RepoRoot()
	if err != nil {
		fatal("%v", err)
	}

	goFields, violations, err := collectGoFields(filepath.Join(root, "app"))
	if err != nil {
		fatal("collect Go fields: %v", err)
	}
	if len(goFields) == 0 {
		fatal("no ValidationError{Field: ...} literals found under app/ — the scan is broken")
	}

	feKeys, err := collectFeKeys(filepath.Join(root, "assets"))
	if err != nil {
		fatal("collect FE keys: %v", err)
	}
	if len(feKeys) == 0 {
		fatal("no *Errors.ts types found under assets/ — the scan is broken")
	}

	stray, err := strayErrorsTypes(filepath.Join(root, "assets"))
	if err != nil {
		fatal("scan for stray *Errors types: %v", err)
	}
	violations = append(violations, stray...)

	violations = append(violations, diff(goFields, feKeys)...)
	if len(violations) > 0 {
		sort.Strings(violations)
		fmt.Fprintf(os.Stderr, "errfields: %d violation(s):\n  %s\n",
			len(violations), strings.Join(violations, "\n  "))
		os.Exit(1)
	}
	fmt.Printf("errfields: %d Go field literal(s) ↔ %d FE error key(s) OK\n",
		len(goFields), len(feKeys))
}

// fieldRef is one Field: "..." literal occurrence.
type fieldRef struct {
	key string
	pos string // file:line for the report
}

// collectGoFields parses every non-test .go file under dir and returns the
// Field literals of shared.ValidationError composite literals (minus the
// //gkerrf:exempt-ed ones) plus the violations found on the way: marker
// misuse, dynamic or positional literals the scan cannot see, and the two
// name-matching tripwires.
func collectGoFields(dir string) ([]fieldRef, []string, error) {
	var refs []fieldRef
	var violations []string
	fset := token.NewFileSet()
	err := filepath.WalkDir(dir, func(path string, e os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		// Cheap prefilter: most files can contribute neither a literal, a
		// tripwire, nor a marker — skip the parse entirely.
		if !bytes.Contains(src, []byte("ValidationError")) &&
			!bytes.Contains(src, []byte(exemptMarker)) {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, src, parser.ParseComments)
		if perr != nil {
			return fmt.Errorf("parse %s: %w", path, perr)
		}
		fileRefs, fileViolations := collectFile(fset, f, path, src)
		refs = append(refs, fileRefs...)
		violations = append(violations, fileViolations...)
		return nil
	})
	return refs, violations, err
}

func collectFile(
	fset *token.FileSet,
	f *ast.File,
	path string,
	src []byte,
) (refs []fieldRef, violations []string) {
	exempts, misplaced := tool.EscapeLines(fset, f, src, exemptMarker)
	for _, line := range misplaced {
		violations = append(violations, fmt.Sprintf(
			"%s:%d: //gkerrf:exempt must stand alone on the line above the literal",
			path, line))
	}
	for _, esc := range exempts {
		if esc.Reason == "" {
			violations = append(violations, fmt.Sprintf(
				"%s:%d: //gkerrf:exempt without a reason — say WHY this field never "+
					"renders in a form", path, esc.MarkerLine))
		}
	}
	consumed := map[int]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		if v := tripwire(fset, n, path); v != "" {
			violations = append(violations, v)
			return true
		}
		lit, ok := n.(*ast.CompositeLit)
		if !ok || !isValidationError(lit.Type) {
			return true
		}
		litLine := fset.Position(lit.Pos()).Line
		if esc, ok := exempts[litLine]; ok {
			consumed[esc.MarkerLine] = true
			return true
		}
		r, v := literalField(fset, lit, path)
		if v != "" {
			violations = append(violations, v)
		} else if r.key != "" {
			refs = append(refs, r)
		}
		return true
	})
	for _, esc := range exempts {
		if esc.Reason != "" && !consumed[esc.MarkerLine] {
			violations = append(violations, fmt.Sprintf(
				"%s:%d: unused //gkerrf:exempt — no ValidationError literal starts on "+
					"the next line; remove the stale marker", path, esc.MarkerLine))
		}
	}
	return refs, violations
}

// literalField extracts the Field key of one ValidationError composite
// literal; a shape the scan cannot see (positional elements, a dynamic Field
// expression, a non-plain string literal) is a violation. Field "" routes to
// the FE `general` slot by convention and returns an empty ref.
func literalField(
	fset *token.FileSet,
	lit *ast.CompositeLit,
	path string,
) (fieldRef, string) {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			return fieldRef{}, fmt.Sprintf(
				"%s:%d: positional ValidationError literal — the parity check can only "+
					"see keyed fields; write Field:/Message: explicitly",
				path, fset.Position(lit.Pos()).Line)
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != "Field" {
			continue
		}
		pos := fset.Position(kv.Pos())
		val, ok := kv.Value.(*ast.BasicLit)
		if !ok || val.Kind != token.STRING {
			return fieldRef{}, fmt.Sprintf(
				"%s:%d: ValidationError.Field is not a string literal — the parity "+
					"check cannot see a dynamic field name; use a literal or "+
					"//gkerrf:exempt <reason>", path, pos.Line)
		}
		name, uerr := strconv.Unquote(val.Value)
		if uerr != nil {
			return fieldRef{}, fmt.Sprintf(
				"%s:%d: cannot unquote Field literal %s: %v", path, pos.Line, val.Value, uerr)
		}
		if name == "" {
			// Field "" routes to the FE `general` slot by convention —
			// always a valid home.
			return fieldRef{}, ""
		}
		return fieldRef{key: name, pos: fmt.Sprintf("%s:%d", path, pos.Line)}, ""
	}
	return fieldRef{}, ""
}

// tripwire guards the name-based matching: a second ValidationError type
// under app/ (outside domain/shared) would inject foreign fields, and a
// NewValidationError constructor would make its call sites invisible to the
// literal scan. Both are violations until the tool is taught about them.
func tripwire(fset *token.FileSet, n ast.Node, path string) string {
	switch d := n.(type) {
	case *ast.TypeSpec:
		if d.Name.Name == "ValidationError" &&
			!strings.Contains(filepath.ToSlash(path), "domain/shared/") {
			return fmt.Sprintf(
				"%s:%d: a second ValidationError type — the parity check matches by "+
					"name and would collect its fields too; rename it or teach gk errfields",
				path, fset.Position(d.Pos()).Line)
		}
	case *ast.FuncDecl:
		if d.Name.Name == "NewValidationError" {
			return fmt.Sprintf(
				"%s:%d: NewValidationError constructor — its call sites are invisible "+
					"to the literal scan; keep constructing ValidationError literals "+
					"inline or teach gk errfields", path, fset.Position(d.Pos()).Line)
		}
	}
	return ""
}

func isValidationError(t ast.Expr) bool {
	switch x := t.(type) {
	case *ast.SelectorExpr:
		return x.Sel.Name == "ValidationError"
	case *ast.Ident:
		return x.Name == "ValidationError"
	default:
		return false
	}
}

// errorsKeyRe matches one optional key line of an *Errors type, e.g.
// "    nickname?: ApiMessage;". The FE convention (gk-frontend-forms) is
// exactly this shape — error values are keyed ApiMessage objects rendered by
// tm(); anything else in an *Errors file is a parse error on purpose.
var errorsKeyRe = regexp.MustCompile(`^\s{4}([A-Za-z_][A-Za-z0-9_]*)\?: ApiMessage;$`)

// structuralRe matches the non-key lines an *Errors file may contain: a
// comment, ANY type-only import, the type header, the closing brace, a blank
// line. The import is matched by shape, not by path — the ApiMessage location
// is owned by the //gkts: directive in app/presentation/http/response, and
// pinning it here byte-exact would fail the gate on a move with an error
// naming neither the cause nor the file to change.
var structuralRe = regexp.MustCompile(
	`^(//.*|import type \{[^{}]*\} from '[^']*';|export type \w+Errors = \{|\};|)$`,
)

// strayErrorsRe finds an *Errors type declared outside a *Errors.ts file —
// line-anchored so `import { type XxxErrors }` lines don't match.
var strayErrorsRe = regexp.MustCompile(`^\s*(?:export\s+)?(?:type|interface)\s+\w*Errors\b`)

// collectFeKeys reads every *Errors.ts under dir and returns the declared
// keys, each with the first position declaring it.
func collectFeKeys(dir string) (map[string]string, error) {
	keys := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, e os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() || !strings.HasSuffix(path, "Errors.ts") {
			return nil
		}
		content, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		for i, line := range strings.Split(string(content), "\n") {
			if m := errorsKeyRe.FindStringSubmatch(line); m != nil {
				if _, ok := keys[m[1]]; !ok {
					keys[m[1]] = fmt.Sprintf("%s:%d", path, i+1)
				}
				continue
			}
			if !structuralRe.MatchString(strings.TrimRight(line, "\r")) {
				return fmt.Errorf(
					"%s:%d: unexpected line in an *Errors type (want `key?: ApiMessage;` "+
						"per gk-frontend-forms): %q", path, i+1, line)
			}
		}
		return nil
	})
	return keys, err
}

// strayErrorsTypes flags *Errors-shaped type declarations outside *Errors.ts
// files: collectFeKeys never reads those, so an inline declaration would
// silently dodge the parity check.
func strayErrorsTypes(dir string) ([]string, error) {
	var violations []string
	err := filepath.WalkDir(dir, func(path string, e os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		isTS := strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".vue")
		if e.IsDir() || !isTS || strings.HasSuffix(path, "Errors.ts") {
			return nil
		}
		content, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		for i, line := range strings.Split(string(content), "\n") {
			if strayErrorsRe.MatchString(line) {
				violations = append(violations, fmt.Sprintf(
					"%s:%d: *Errors type declared outside a *Errors.ts file — the parity "+
						"check only reads *Errors.ts; move it to <Domain>/types/", path, i+1))
			}
		}
		return nil
	})
	return violations, err
}

// diff runs both directions of the parity check.
func diff(goFields []fieldRef, feKeys map[string]string) []string {
	var violations []string
	goSet := map[string]bool{}
	for _, r := range goFields {
		goSet[r.key] = true
		if _, ok := feKeys[r.key]; !ok {
			violations = append(violations, fmt.Sprintf(
				"%s: Field %q has no home in any FE *Errors type — the error would "+
					"render nowhere (add the key, or //gkerrf:exempt <reason> if it "+
					"never reaches a form)", r.pos, r.key))
		}
	}
	for key, position := range feKeys {
		if key == "general" {
			continue // the conventional catch-all — always produced (Field "" + synthesized failures)
		}
		if !goSet[key] {
			violations = append(violations, fmt.Sprintf(
				"%s: FE error key %q has no Go ValidationError{Field: ...} producer — "+
					"a phantom key that can never light up", position, key))
		}
	}
	return violations
}

func fatal(format string, args ...any) {
	tool.Fatalf("errfields", format, args...)
}
