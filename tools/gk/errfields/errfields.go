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
// on the line directly above the `Field:` literal. The reason is mandatory.
//
// Zero-sites guards: finding no Field literals, or no *Errors files, fails the
// check — a gate that sees nothing must scream, not stay green.
package errfields

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const exemptMarker = "gkerrf:exempt"

// Run is the `gk errfields` entrypoint; args are the words after "errfields".
func Run(args []string) {
	if len(args) > 0 {
		fatal("unknown argument %q (gk errfields takes none)", args[0])
	}
	root := repoRoot()

	goFields, err := collectGoFields(filepath.Join(root, "app"))
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

	violations := diff(goFields, feKeys)
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
// Field literals of shared.ValidationError composite literals, minus the
// //gkerrf:exempt-ed ones. An exempt without a reason is an error.
func collectGoFields(dir string) ([]fieldRef, error) {
	var refs []fieldRef
	fset := token.NewFileSet()
	err := filepath.WalkDir(dir, func(path string, e os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if perr != nil {
			return fmt.Errorf("parse %s: %w", path, perr)
		}
		exempts, eerr := exemptLines(fset, f, path)
		if eerr != nil {
			return eerr
		}
		var walkErr error
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok || !isValidationError(lit.Type) {
				return true
			}
			for _, elt := range lit.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := kv.Key.(*ast.Ident)
				if !ok || key.Name != "Field" {
					continue
				}
				pos := fset.Position(kv.Pos())
				if exempts[pos.Line] {
					continue
				}
				val, ok := kv.Value.(*ast.BasicLit)
				if !ok || val.Kind != token.STRING {
					walkErr = fmt.Errorf(
						"%s:%d: ValidationError.Field is not a string literal — the parity "+
							"check cannot see a dynamic field name; use a literal or "+
							"//gkerrf:exempt <reason>", path, pos.Line)
					return false
				}
				key2 := strings.Trim(val.Value, `"`)
				if key2 == "" {
					// Field "" routes to the FE `general` slot by convention —
					// always a valid home.
					continue
				}
				refs = append(refs, fieldRef{
					key: key2,
					pos: fmt.Sprintf("%s:%d", path, pos.Line),
				})
			}
			return true
		})
		return walkErr
	})
	return refs, err
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

// exemptLines maps line numbers exempted by a //gkerrf:exempt comment — the
// marker exempts the line DIRECTLY below it. A bare marker (no reason) errors.
func exemptLines(fset *token.FileSet, f *ast.File, path string) (map[int]bool, error) {
	lines := map[int]bool{}
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			text := strings.TrimPrefix(c.Text, "//")
			if !strings.HasPrefix(text, exemptMarker) {
				continue
			}
			reason := strings.TrimSpace(strings.TrimPrefix(text, exemptMarker))
			line := fset.Position(c.Pos()).Line
			if reason == "" {
				return nil, fmt.Errorf(
					"%s:%d: //gkerrf:exempt without a reason — say WHY this field "+
						"never renders in a form", path, line)
			}
			lines[line+1] = true
		}
	}
	return lines, nil
}

// errorsKeyRe matches one optional key line of an *Errors type, e.g.
// "    nickname?: string;". The FE convention (gk-frontend-forms) is exactly
// this shape; anything else in an *Errors file is a parse error on purpose.
var errorsKeyRe = regexp.MustCompile(`^\s{4}([A-Za-z_][A-Za-z0-9_]*)\?: string;$`)

// structuralRe matches the non-key lines an *Errors file may contain.
var structuralRe = regexp.MustCompile(`^(//.*|export type \w+Errors = \{|\};|)$`)

// collectFeKeys reads every assets/**/types/*Errors.ts and returns the union
// of declared keys with the positions declaring them.
func collectFeKeys(dir string) (map[string][]string, error) {
	keys := map[string][]string{}
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
				keys[m[1]] = append(keys[m[1]], fmt.Sprintf("%s:%d", path, i+1))
				continue
			}
			if !structuralRe.MatchString(strings.TrimRight(line, "\r")) {
				return fmt.Errorf(
					"%s:%d: unexpected line in an *Errors type (want `key?: string;` "+
						"per gk-frontend-forms): %q", path, i+1, line)
			}
		}
		return nil
	})
	return keys, err
}

// diff runs both directions of the parity check.
func diff(goFields []fieldRef, feKeys map[string][]string) []string {
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
	for key, positions := range feKeys {
		if key == "general" {
			continue // the conventional catch-all — always produced (Field "" + synthesized failures)
		}
		if !goSet[key] {
			violations = append(violations, fmt.Sprintf(
				"%s: FE error key %q has no Go ValidationError{Field: ...} producer — "+
					"a phantom key that can never light up", positions[0], key))
		}
	}
	return violations
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
	fmt.Fprintf(os.Stderr, "errfields: "+format+"\n", args...)
	os.Exit(1)
}
