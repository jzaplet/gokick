package i18n

import (
	"go/parser"
	"go/token"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestConstName(t *testing.T) {
	tests := []struct{ key, want string }{
		{"auth.invalid_credentials", "AuthInvalidCredentials"},
		{"user.nickname_too_long", "UserNicknameTooLong"},
		{"request.single_json_object_required", "RequestSingleJSONObjectRequired"},
		{"common.internal_error", "CommonInternalError"},
	}
	for _, tt := range tests {
		if got := constName(tt.key); got != tt.want {
			t.Errorf("constName(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestParseCatalog(t *testing.T) {
	parsed, err := parseCatalog([]byte(`{
		"a.plain": "text",
		"a.plural": {"one": "{count} item", "other": "{count} items"}
	}`))
	if err != nil {
		t.Fatalf("parseCatalog: %v", err)
	}
	if parsed["a.plain"].single != "text" {
		t.Errorf("plain message not parsed: %+v", parsed["a.plain"])
	}
	if parsed["a.plural"].forms["other"] != "{count} items" {
		t.Errorf("plural message not parsed: %+v", parsed["a.plural"])
	}
	if got := parsed["a.plural"].params(); len(got) != 1 || got[0] != "count" {
		t.Errorf("params = %v, want [count]", got)
	}

	if _, err := parseCatalog([]byte(`{"a.bad": 42}`)); err == nil {
		t.Error("numeric message value should be rejected")
	}
}

func validCatalogs() map[string]map[string]message {
	return map[string]map[string]message{
		"cs": {
			"user.count": {
				forms: map[string]string{
					"one":   "{count} uživatel",
					"few":   "{count} uživatelé",
					"many":  "{count} uživatele",
					"other": "{count} uživatelů",
				},
			},
			"user.plain": {single: "text"},
		},
		"en": {
			"user.count": {
				forms: map[string]string{"one": "{count} user", "other": "{count} users"},
			},
			"user.plain": {single: "text"},
		},
	}
}

func TestValidateCatalogs(t *testing.T) {
	if problems := validateCatalogs(validCatalogs(), testPlurals); len(problems) != 0 {
		t.Fatalf("valid catalogs flagged: %v", problems)
	}

	tests := []struct {
		name     string
		mutate   func(c map[string]map[string]message)
		fragment string
	}{
		{
			name:     "missing key in the non-canonical catalog",
			mutate:   func(c map[string]map[string]message) { delete(c["cs"], "user.plain") },
			fragment: "missing key",
		},
		{
			name: "extra key in the non-canonical catalog",
			mutate: func(c map[string]map[string]message) {
				c["cs"]["user.extra"] = message{single: "x"}
			},
			fragment: "missing from the canonical",
		},
		{
			name: "plural form a language can select is missing",
			mutate: func(c map[string]map[string]message) {
				delete(c["cs"]["user.count"].forms, "few")
			},
			fragment: `missing the "few" form`,
		},
		{
			name: "plural never references count",
			mutate: func(c map[string]map[string]message) {
				c["en"]["user.count"] = message{
					forms: map[string]string{"one": "one user", "other": "some users"},
				}
				c["cs"]["user.count"] = message{
					forms: map[string]string{
						"one": "uživatel", "few": "uživatelé",
						"many": "uživatele", "other": "uživatelů",
					},
				}
			},
			fragment: "references no {count} placeholder",
		},
		{
			name: "one plural form drops a placeholder the others carry",
			mutate: func(c map[string]map[string]message) {
				c["cs"]["user.count"].forms["other"] = "hodně uživatelů"
			},
			fragment: "every form must carry the same placeholders",
		},
		{
			name: "param mismatch",
			mutate: func(c map[string]map[string]message) {
				c["en"]["user.plain"] = message{single: "hi {name}"}
			},
			fragment: "params",
		},
		{
			name: "uppercase-first placeholder",
			mutate: func(c map[string]map[string]message) {
				c["en"]["user.plain"] = message{single: "hi {Name}"}
				c["cs"]["user.plain"] = message{single: "ahoj {Name}"}
			},
			fragment: "lowercase",
		},
		{
			name: "pluralness mismatch",
			mutate: func(c map[string]map[string]message) {
				c["cs"]["user.plain"] = message{
					forms: map[string]string{"one": "text", "other": "text"},
				}
			},
			fragment: "plural vs plain",
		},
		{
			name: "unknown plural form",
			mutate: func(c map[string]map[string]message) {
				c["en"]["user.count"] = message{
					forms: map[string]string{"some": "x", "other": "{count} users"},
				}
			},
			fragment: "unknown plural form",
		},
		{
			name: "plural without other",
			mutate: func(c map[string]map[string]message) {
				c["en"]["user.count"] = message{forms: map[string]string{"one": "{count} user"}}
			},
			fragment: `"other" form`,
		},
		{
			name: "bad key format",
			mutate: func(c map[string]map[string]message) {
				c["cs"]["BadKey"] = message{single: "x"}
				c["en"]["BadKey"] = message{single: "x"}
			},
			fragment: "snake_case",
		},
		{
			// A new catalog language is not "unsupported" here: the set is
			// discovered from locale/ and every other declaration of it is
			// generated, so a de catalog is validated like any other.
			name: "incomplete extra language",
			mutate: func(c map[string]map[string]message) {
				c["de"] = map[string]message{"user.plain": {single: "text"}}
			},
			fragment: `catalog de: missing key`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			catalogs := clone(validCatalogs())
			tt.mutate(catalogs)
			problems := validateCatalogs(catalogs, testPlurals)
			if len(problems) == 0 {
				t.Fatal("expected a problem, got none")
			}
			found := false
			for _, p := range problems {
				if strings.Contains(p, tt.fragment) {
					found = true
				}
			}
			if !found {
				t.Errorf("no problem contains %q; got %v", tt.fragment, problems)
			}
		})
	}

	empty := map[string]map[string]message{"cs": {}, "en": {}}
	if problems := validateCatalogs(empty, testPlurals); len(problems) == 0 {
		t.Error("empty catalogs must scream, got no problems")
	}

	// The canonical catalog is what every artifact derives from — discovering
	// only other languages must fail loudly, not generate an empty key set.
	noCanonical := map[string]map[string]message{"cs": validCatalogs()["cs"]}
	if problems := validateCatalogs(noCanonical, testPlurals); len(problems) == 0 {
		t.Error("a missing canonical catalog must scream, got no problems")
	}
}

// One pass feeds BOTH Go gates, so it must collect what each of them used to
// walk the tree for on its own: every msgkey selector (import-alias aware) and
// every `Key: msgkey.X` literal with its Params — from production files only,
// and only from the ones the ImportsOnly prefilter lets through.
func TestScanProductionGo(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	write("app/handler/handler.go", `package handler

import mk "`+msgkeyImportPath+`"

var used = mk.UserPlain

var site = shared.MessageError{
	Key:    mk.UserCount,
	Params: map[string]any{"count": 1},
}

var dynamic = shared.MessageError{Key: mk.UserPlain, Params: buildParams()}
`)
	write("app/handler/handler_test.go", `package handler

import "`+msgkeyImportPath+`"

var inTest = msgkey.UserOnlyInTests
`)
	// No msgkey import — the prefilter must drop it before the full parse.
	write("cmd/root.go", `package cmd

var noImport = somepkg.UserPlain
`)

	scan, err := scanProductionGo(root)
	if err != nil {
		t.Fatalf("scanProductionGo: %v", err)
	}
	for _, name := range []string{"UserPlain", "UserCount"} {
		if !scan.usedNames[name] {
			t.Errorf("selector msgkey.%s not collected", name)
		}
	}
	if scan.usedNames["UserOnlyInTests"] {
		t.Error("a _test.go selector must not count as production usage")
	}
	if len(scan.sites) != 2 {
		t.Fatalf("collected %d call sites, want 2", len(scan.sites))
	}
	byConst := map[string]keyedCallSite{}
	for _, site := range scan.sites {
		byConst[site.constName] = site
	}
	if site := byConst["UserCount"]; !site.checkable || !site.params["count"] {
		t.Errorf("literal Params not collected: %+v", site)
	}
	if byConst["UserPlain"].checkable {
		t.Error("non-literal Params must mark the site unchecked, not fake an empty set")
	}
}

// The deadness gate stands on this extraction: a quoted key anywhere in
// frontend source counts as a reference — including one nested inside a Vue
// attribute binding, the shape most of the UI labels take — and the GENERATED
// catalogs must not, since they quote every key and would blind the gate.
func TestFrontendKeyLiterals(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	write("assets/app/Used.vue",
		"t('user.used')\n"+
			":aria-label=\"t('user.nested')\"\n"+
			"// it's prose naming user.bare unquoted\n")
	write("assets/app/Other.ts", "const k = \"user.doubled\";\nconst t = `user.ticked`;\n")
	write("assets/app/skipped.md", "'user.wrongext'\n")
	write(tsOutDir+"/en.ts", "'user.generated': 'x',\n")

	literals, scanned, err := frontendKeyLiterals(root)
	if err != nil {
		t.Fatalf("frontendKeyLiterals: %v", err)
	}
	if scanned != 2 {
		t.Errorf("scanned %d files, want 2 (.md and the generated catalog skipped)", scanned)
	}
	for _, key := range []string{"user.used", "user.nested", "user.doubled", "user.ticked"} {
		if !literals[key] {
			t.Errorf("quoted key %q not extracted", key)
		}
	}
	for _, key := range []string{"user.bare", "user.generated", "user.wrongext"} {
		if literals[key] {
			t.Errorf("key %q must not count as a reference", key)
		}
	}
}

func TestRenderKeysIsGofmtCleanAndSorted(t *testing.T) {
	src, err := renderKeys(map[string]message{
		"b.two": {single: "x"},
		"a.one": {single: "y"},
	})
	if err != nil {
		t.Fatalf("renderKeys: %v", err)
	}
	text := string(src)
	if !strings.HasPrefix(text, genHeader) {
		t.Error("generated file must start with the gen header")
	}
	if strings.Index(text, `"a.one"`) > strings.Index(text, `"b.two"`) {
		t.Error("keys must be emitted sorted")
	}
	if !strings.Contains(text, "AOne Key = \"a.one\"") {
		t.Errorf("missing const line, got:\n%s", text)
	}
}

func TestRenderKeysCollision(t *testing.T) {
	_, err := renderKeys(map[string]message{
		"a.b_c": {single: "x"},
		"a.b.c": {single: "y"},
	})
	if err == nil || !strings.Contains(err.Error(), "both map to const") {
		t.Errorf("collision must fail renderKeys, got err=%v", err)
	}
}

func TestRenderTSCanonical(t *testing.T) {
	got := string(renderTS("en", validCatalogs()["en"]))

	for _, want := range []string{
		"// Code generated by gk i18n from locale/messages.en.json; " +
			"DO NOT EDIT — edit the JSON and run make i18n-gen.\n",
		"export const enCatalog = {\n",
		"    'user.plain': 'text',\n",
		"    'user.count': {\n        one: '{count} user',\n        other: '{count} users',\n    },\n",
		"} as const;\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("en.ts missing %q, got:\n%s", want, got)
		}
	}
	// Data only: the type surface is hand-written in lang.ts, which imports
	// this file — an import here would close the cycle.
	if strings.Contains(got, "import") || strings.Contains(got, "export type") {
		t.Errorf("canonical catalog must declare no type and import nothing, got:\n%s", got)
	}
	if strings.Index(got, "'user.count'") > strings.Index(got, "'user.plain'") {
		t.Error("keys must be emitted sorted")
	}
}

func TestRenderTSNonCanonical(t *testing.T) {
	got := string(renderTS("cs", validCatalogs()["cs"]))

	for _, want := range []string{
		"// Code generated by gk i18n from locale/messages.cs.json; " +
			"DO NOT EDIT — edit the JSON and run make i18n-gen.\n",
		"import type { TranslationCatalog } from '@/app-ui/I18n/lang';\n",
		"export const csCatalog: TranslationCatalog = {\n",
		// CLDR order: one before few before many before other.
		"    'user.count': {\n        one: '{count} uživatel',\n" +
			"        few: '{count} uživatelé',\n        many: '{count} uživatele',\n" +
			"        other: '{count} uživatelů',\n    },\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("cs.ts missing %q, got:\n%s", want, got)
		}
	}
	// Typed against lang.ts, never against a sibling catalog, and declaring
	// no type of its own.
	if strings.Contains(got, "catalog/") || strings.Contains(got, "export type") {
		t.Errorf("non-canonical catalog must only import the type from lang.ts, got:\n%s", got)
	}
}

func TestLoadCatalogsRejectsNonIdentifierLang(t *testing.T) {
	dir := t.TempDir()
	body := []byte(`{"a.plain": "text"}`)
	for _, name := range []string{"messages.en.json", "messages.pt-BR.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	_, err := loadCatalogs(dir)
	if err == nil {
		t.Fatal("region-subtagged catalog accepted — it generates `export const pt-BRCatalog`")
	}
	if !strings.Contains(err.Error(), "pt-BR") {
		t.Errorf("error does not name the offending token: %v", err)
	}
}

func TestTsEscape(t *testing.T) {
	if got := tsEscape(`it's a \ test`); got != `it\'s a \\ test` {
		t.Errorf("tsEscape = %q", got)
	}
	// Both line terminators, not just LF: a raw CR closes a single-quoted TS
	// literal exactly like a raw LF, and no gate downstream can see it.
	if got := tsEscape("one\r\ntwo"); got != `one\r\ntwo` {
		t.Errorf("tsEscape(CRLF) = %q, want the escaped form", got)
	}
}

func clone(src map[string]map[string]message) map[string]map[string]message {
	out := maps.Clone(src)
	for lang := range out {
		out[lang] = maps.Clone(out[lang])
	}
	return out
}

// testPlurals is the CLDR table the catalog-validation tests run against —
// the same shape preparePluralForms hands the real run, pinned here so a
// fixture change cannot quietly weaken the per-language form checks.
var testPlurals = pluralForms{
	"cs": {"few", "many", "one", "other"},
	"en": {"one", "other"},
}

// The generated backend declaration must compile, and must carry every rung a
// consumer reaches for: the constants, the canonical default, the slice and
// the parser. Parsing it with go/parser is the cheap stand-in for "the app
// still builds" — a malformed render fails here instead of in every package.
func TestRenderLangsGo(t *testing.T) {
	src, err := renderLangsGo([]string{"cs", "de", "en"})
	if err != nil {
		t.Fatalf("renderLangsGo: %v", err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "lang_gen.go", src, 0); err != nil {
		t.Fatalf("generated Go does not parse: %v\n%s", err, src)
	}
	got := string(src)
	for _, want := range []string{
		genHeader,
		"package shared",
		`LangCS Lang = "cs"`,
		`LangDE Lang = "de"`,
		`LangEN Lang = "en"`,
		"DefaultLang = LangEN",
		"return []Lang{LangCS, LangDE, LangEN}",
		"case LangDE:",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("generated Go is missing %q:\n%s", want, got)
		}
	}
}

// A language whose token is not the canonical one must still round-trip
// through ParseLang — the switch is generated per language, so a renderer that
// only emitted the canonical case would make every other language unparseable
// while everything still compiled.
func TestRenderLangsGoParsesEveryLanguage(t *testing.T) {
	src, err := renderLangsGo([]string{"cs", "en"})
	if err != nil {
		t.Fatalf("renderLangsGo: %v", err)
	}
	for _, lang := range []string{"cs", "en"} {
		if !strings.Contains(string(src), "case Lang"+strings.ToUpper(lang)+":") {
			t.Errorf("ParseLang has no case for %q:\n%s", lang, src)
		}
	}
}

func TestRenderLangsTS(t *testing.T) {
	got := string(renderLangsTS([]string{"cs", "de", "en"}))
	for _, want := range []string{
		"DO NOT EDIT",
		"export const SUPPORTED_LANGS = ['cs', 'de', 'en'] as const;",
		"export type Lang = (typeof SUPPORTED_LANGS)[number];",
		"export const CANONICAL_LANG: Lang = 'en';",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("generated langs.ts is missing %q:\n%s", want, got)
		}
	}
}

// The catalogs record is what makes vue-tsc re-check key parity per language,
// so it must import EVERY discovered catalog and annotate the record with the
// full Lang union — a record missing a language would type-check as incomplete
// rather than silently render nothing.
func TestRenderCatalogsTS(t *testing.T) {
	got := string(renderCatalogsTS([]string{"cs", "de", "en"}))
	for _, want := range []string{
		"import { csCatalog } from '@/app-ui/I18n/catalog/cs';",
		"import { deCatalog } from '@/app-ui/I18n/catalog/de';",
		"import type { Lang, TranslationCatalog } from '@/app-ui/I18n/lang';",
		"    de: deCatalog,\n",
		"} satisfies Record<Lang, TranslationCatalog>;",
		"export const canonicalCatalog = enCatalog;",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("generated catalogs.ts is missing %q:\n%s", want, got)
		}
	}
}

// The Node bridge is the only part of the gate that is not pure Go, so it is
// pinned directly: Czech must come back with the forms Intl actually selects,
// and a tag ICU does not know must FAIL rather than silently inherit the
// fallback locale's categories.
func TestCldrPluralForms(t *testing.T) {
	requireNode(t)
	forms, err := cldrPluralForms([]string{"cs", "en"})
	if err != nil {
		t.Fatalf("cldrPluralForms: %v", err)
	}
	for _, want := range []string{"one", "few", "many", "other"} {
		if !slices.Contains(forms["cs"], want) {
			t.Errorf("cs is missing the %q category: %v", want, forms["cs"])
		}
	}
	if !slices.Equal(forms["en"], []string{"one", "other"}) {
		t.Errorf("en = %v, want [one other]", forms["en"])
	}

	if _, err := cldrPluralForms([]string{"zzz"}); err == nil {
		t.Error("an ICU-unknown language must fail, not inherit the fallback locale")
	}
}

// requireNode skips the tests that shell out. The gate itself hard-fails
// without Node (deliberately — a cached plural table ratifies its own errors),
// but a Go-only environment should skip these rather than report a red suite
// for a missing frontend toolchain.
func requireNode(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not on PATH; `gk i18n generate` needs it, `check` does not")
	}
}

// The freshness gate is only as complete as this map: a path dropped from it
// stops being GENERATED and stops being CHECKED in the same edit, silently and
// with every other test still green. That completeness is the whole load-bearing
// claim left after the three-way language mirror was deleted, so it is pinned
// here rather than left to the individual renderer tests.
func TestRenderArtifactsCoversEveryGeneratedPath(t *testing.T) {
	catalogs := validCatalogs()
	langs := []string{"cs", "en"}

	artifacts, err := renderArtifacts(catalogs, langs)
	if err != nil {
		t.Fatalf("renderArtifacts: %v", err)
	}

	want := []string{
		keysOutPath,
		langsGoOutPath,
		langsTSOutPath,
		catalogsTSPath,
		tsOutDir + "/cs.ts",
		tsOutDir + "/en.ts",
	}
	got := slices.Sorted(maps.Keys(artifacts))
	if !slices.Equal(got, slices.Sorted(slices.Values(want))) {
		t.Fatalf("generated path set drifted:\n got  %v\n want %v", got, want)
	}
	for _, rel := range got {
		if len(artifacts[rel]) == 0 {
			t.Errorf("%s renders empty", rel)
		}
	}
}

// End-to-end on the one link that is not pure Go: the categories Node reports
// must be the ones validation actually enforces. Asserting through
// validateCatalogs rather than on cldrPluralForms' return value is the point —
// it fails if the table stops reaching the check, not merely if Node changes.
func TestNodeDerivedFormsReachValidation(t *testing.T) {
	requireNode(t)

	plurals, err := cldrPluralForms([]string{"cs", "en"})
	if err != nil {
		t.Fatalf("cldrPluralForms: %v", err)
	}
	if problems := validateCatalogs(validCatalogs(), plurals); len(problems) != 0 {
		t.Fatalf("valid catalogs flagged against live CLDR data: %v", problems)
	}

	catalogs := clone(validCatalogs())
	delete(catalogs["cs"]["user.count"].forms, "few")
	problems := strings.Join(validateCatalogs(catalogs, plurals), "\n")
	if !strings.Contains(problems, `missing the "few" form`) {
		t.Fatalf("a cs catalog without \"few\" passed against live CLDR data: %q", problems)
	}
}
