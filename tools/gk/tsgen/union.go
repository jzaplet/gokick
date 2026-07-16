package tsgen

// Union emission: a named Go string type whose constants become the TS triple
// `const X = {...} as const` + `type X` + `isX` guard.
//
// Why a triple and not just `type X = 'a' | 'b'`: a union gives the FE a TYPE,
// but the call sites need VALUES — `[Role.Admin]: '/admin/dashboard'`,
// `initial.role ?? Role.User`. The hand-written enum this replaces existed for
// exactly that, so generating only the type would not have let it go away.
//
// Why the resolution below hops across packages: the canonical role strings
// live in domain/shared (the authorization ladder compares against them) and
// the VO consts are CONVERSIONS of them — `RoleSuperAdmin = Role(shared.RoleSuperAdmin)`
// — deliberately, so the ladder and the value object share one definition.
// Reading only the local file would therefore find no literal at all, and any
// shortcut that re-states the strings here would recreate the duplication the
// Go side went out of its way to avoid. So: resolve the conversion's argument
// through the file's imports into the declaring package. Both shapes are
// supported — a direct `const X T = "lit"` and a conversion of another
// package's const.

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

// unionMember is one constant of a union type.
type unionMember struct {
	goName string // RoleSuperAdmin
	tsName string // SuperAdmin — goName with the type-name prefix stripped
	value  string // superadmin
}

// unionDef is a named string type annotated with a `union` directive.
type unionDef struct {
	goName  string
	dir     directive
	members []unionMember // declaration order — the emitted order
}

// parsedFile is one parsed Go file plus what cross-package const resolution
// needs: which package it belongs to, and what its imports are called locally.
type parsedFile struct {
	path    string
	pkgPath string // gokick/app/domain/user
	file    *ast.File
	imports map[string]string // local name -> import path
}

// constIndex maps an import path to that package's string constants.
type constIndex map[string]map[string]string

// fileImports maps each import's local name to its path, honoring aliases.
func fileImports(f *ast.File) map[string]string {
	out := map[string]string{}
	for _, imp := range f.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		name := path[strings.LastIndex(path, "/")+1:]
		if imp.Name != nil {
			name = imp.Name.Name
		}
		out[name] = path
	}
	return out
}

// buildConstIndex records every package-level string constant under app/, so a
// conversion in one package can be resolved against the package that declares
// the literal.
func buildConstIndex(files []parsedFile) constIndex {
	idx := constIndex{}
	for _, pf := range files {
		for _, decl := range pf.file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range vs.Names {
					if i >= len(vs.Values) {
						continue // iota or a repeated expression — not a string literal
					}
					lit, ok := stringLit(vs.Values[i])
					if !ok {
						continue
					}
					if idx[pf.pkgPath] == nil {
						idx[pf.pkgPath] = map[string]string{}
					}
					idx[pf.pkgPath][name.Name] = lit
				}
			}
		}
	}
	return idx
}

// stringLit unwraps a plain string literal.
func stringLit(e ast.Expr) (string, bool) {
	bl, ok := e.(*ast.BasicLit)
	if !ok || bl.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(bl.Value)
	if err != nil {
		return "", false
	}
	return s, true
}

// collectUnions resolves every `union`-annotated named string type to its
// members. It errors rather than guessing: a union that resolves to nothing, or
// a member whose value cannot be traced to a literal, is a silent contract hole
// on the wire — the FE guard would either reject everything or admit anything.
func collectUnions(files []parsedFile, idx constIndex) ([]unionDef, error) {
	var out []unionDef
	for _, pf := range files {
		for _, decl := range pf.file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				dir, ok, err := parseDirective(ts.Doc, gd.Doc)
				if err != nil {
					return nil, fmt.Errorf("%s: %w", pf.path, err)
				}
				if !ok || !dir.union {
					continue
				}
				if id, isIdent := ts.Type.(*ast.Ident); !isIdent || id.Name != "string" {
					return nil, fmt.Errorf(
						"%s: type %s carries a `union` directive but is not a named string type",
						pf.path, ts.Name.Name)
				}
				members, merr := unionMembers(files, idx, pf.pkgPath, ts.Name.Name)
				if merr != nil {
					return nil, fmt.Errorf("%s: type %s: %w", pf.path, ts.Name.Name, merr)
				}
				if len(members) == 0 {
					return nil, fmt.Errorf(
						"%s: type %s is a `union` but has no constants — the guard would reject every value",
						pf.path,
						ts.Name.Name,
					)
				}
				out = append(out, unionDef{goName: ts.Name.Name, dir: dir, members: members})
			}
		}
	}
	return out, nil
}

// unionMembers finds the constants OF the named type within its own package and
// resolves each to its string value.
func unionMembers(
	files []parsedFile,
	idx constIndex,
	pkgPath, typeName string,
) ([]unionMember, error) {
	var out []unionMember
	for _, pf := range files {
		if pf.pkgPath != pkgPath {
			continue
		}
		for _, decl := range pf.file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range vs.Names {
					if i >= len(vs.Values) {
						continue
					}
					value, isMember, err := memberValue(pf, idx, typeName, vs.Type, vs.Values[i])
					if err != nil {
						return nil, fmt.Errorf("const %s: %w", name.Name, err)
					}
					if !isMember {
						continue
					}
					out = append(out, unionMember{
						goName: name.Name,
						tsName: memberTSName(name.Name, typeName),
						value:  value,
					})
				}
			}
		}
	}
	return out, nil
}

// memberValue reports whether a const belongs to the union type and, if so, its
// literal value. Two shapes count:
//
//	const RoleAdmin Role = "admin"     // declared type + literal
//	RoleAdmin = Role(shared.RoleAdmin) // conversion, value declared elsewhere
func memberValue(
	pf parsedFile,
	idx constIndex,
	typeName string,
	declType ast.Expr,
	value ast.Expr,
) (string, bool, error) {
	// Shape 1: `const X Role = "lit"`.
	if id, ok := declType.(*ast.Ident); ok && id.Name == typeName {
		lit, ok := stringLit(value)
		if !ok {
			return "", false, fmt.Errorf(
				"declared as %s but its value is not a string literal — tsgen cannot resolve it",
				typeName)
		}
		return lit, true, nil
	}

	// Shape 2: `X = Role(...)` — a conversion is what makes it a member.
	call, ok := value.(*ast.CallExpr)
	if !ok {
		return "", false, nil
	}
	fn, ok := call.Fun.(*ast.Ident)
	if !ok || fn.Name != typeName || len(call.Args) != 1 {
		return "", false, nil
	}
	if lit, ok := stringLit(call.Args[0]); ok {
		return lit, true, nil
	}
	sel, ok := call.Args[0].(*ast.SelectorExpr)
	if !ok {
		return "", false, fmt.Errorf(
			"converts to %s but its argument is neither a literal nor a package constant", typeName)
	}
	pkgIdent, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", false, fmt.Errorf("converts to %s from an unresolvable expression", typeName)
	}
	importPath, ok := pf.imports[pkgIdent.Name]
	if !ok {
		return "", false, fmt.Errorf(
			"converts %s.%s but %q is not imported here",
			pkgIdent.Name,
			sel.Sel.Name,
			pkgIdent.Name,
		)
	}
	lit, ok := idx[importPath][sel.Sel.Name]
	if !ok {
		return "", false, fmt.Errorf(
			"converts %s.%s, but %s declares no string constant %s "+
				"(tsgen reads only packages under app/)",
			pkgIdent.Name, sel.Sel.Name, importPath, sel.Sel.Name)
	}
	return lit, true, nil
}

// memberTSName strips the type-name prefix Go needs (RoleAdmin) but TS does not
// (Role.Admin). A const that does not carry the prefix keeps its name.
func memberTSName(constName, typeName string) string {
	if trimmed := strings.TrimPrefix(constName, typeName); trimmed != "" && trimmed != constName {
		return trimmed
	}
	return constName
}

// renderUnion emits the const object + type + guard. The const object carries
// the values, the type is derived FROM it (so the two can never disagree), and
// the guard compares against the object's own members — deliberately not
// `Object.values(X) as string[]`, which would put a type assertion in a
// generated DO-NOT-EDIT file and trip the no-as ratchet.
func renderUnion(sb *strings.Builder, u unionDef) {
	fmt.Fprintf(sb, "export const %s = {\n", u.dir.tsName)
	for _, m := range u.members {
		fmt.Fprintf(sb, "    %s: '%s',\n", m.tsName, m.value)
	}
	sb.WriteString("} as const;\n\n")

	fmt.Fprintf(sb, "export type %s = typeof %s[keyof typeof %s];\n",
		u.dir.tsName, u.dir.tsName, u.dir.tsName)

	if u.dir.noguard {
		return
	}
	sb.WriteString("\n")
	fmt.Fprintf(sb, "export const is%s = (v: unknown): v is %s =>", u.dir.tsName, u.dir.tsName)
	for i, m := range u.members {
		sep := "\n    || "
		if i == 0 {
			sep = "\n    "
		}
		fmt.Fprintf(sb, "%sv === %s.%s", sep, u.dir.tsName, m.tsName)
	}
	sb.WriteString(";\n")
}
