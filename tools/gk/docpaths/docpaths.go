// Package docpaths (the `gk docpaths` subcommand) is the first gate on prose.
// Code has ten gates; the docs and skills that TEACH the code had none — so
// every drift sweep re-found the same rot: a skill citing a migration deleted
// in a squash, a Related link to a skill that was renamed away, a `Kód:`
// pointer to a file that moved. Nothing failed; the reader just followed a
// dead path.
//
// This check reads every `.claude/skills/**/*.md` and `docs/**/*.md` and fails
// on two classes of dead reference:
//
//   - a backticked repo path that resolves to nothing (`app/domain/user/x.go`,
//     `migrations/…_create_audit_log.sql`), including a `:NN` line anchor that
//     points past the end of the file, and
//   - a `/gk-<name>` skill cross-reference with no skill behind it (the
//     Related-links rot: `/gk-wire` outlived the skill it named).
//
// It is deliberately ANCHORED, not clever: a backtick span is a path candidate
// only when it starts with a real repo root (app/, assets/, docs/, …). That
// skips `FindAll/FindByIDAcrossTenants` (a prose shorthand that happens to hold
// a slash), `/api/v1/admin/users` (a URL), `@/app-ui/Auth` (a TS alias) and
// `<ctx>` placeholders — without a per-case denylist to maintain. The cost is
// real: a bare `repository.go` with no directory is not checked. Precision
// first — a noisy gate gets switched off, and a gate nobody trusts is worse
// than none.
//
// What it does NOT check is the other half of drift: whether a cited line still
// holds the code the prose claims, or whether an example still compiles. Those
// need the doc to quote the source, not point at it. This gate buys the cheap
// 80%: every pointer resolves.
//
// Escape hatch for a path that is deliberately fictional (the `order` context
// the "add a bounded context" recipes invent):
//
//	<!-- gkdoc:ignore <path> — <reason> -->
//
// anywhere in the same file. It names its path instead of covering the line
// below it, because documan-fix reflows prose and a position-bound marker would
// silently detach from its target. A marker with no reason is a violation, and
// so is one whose path the file never cites — a stale escape must not rot in
// place.
//
// Zero-sites guard: finding no docs, or no path candidates at all, fails the
// check. A gate that sees nothing must scream, not stay green.
package docpaths

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gokick-gk/internal/tool"
)

// scan says which trees to read and where skills live. It is a parameter, not
// a constant, for one blunt reason: the test fixture would otherwise need a
// real `.claude/skills/` directory, and every tool that discovers skills by
// globbing for that path — Claude Code included — would load the fixture's
// fake skill as if it were this repo's. A gate must not pollute the thing it
// guards.
type scan struct {
	docRoots  []string // trees whose prose is read
	skillsDir string   // where a /gk-<name> reference resolves
}

var repoScan = scan{
	docRoots:  []string{".claude/skills", "docs"},
	skillsDir: ".claude/skills",
}

// repoRoots anchor a backtick span to the filesystem: a candidate must start
// with one of these, so prose shorthand holding a slash is never mistaken for
// a path. Keep in sync with the repo's top-level layout.
var repoRoots = []string{
	"app/", "assets/", "cmd/", "docs/", "migrations/",
	"tests/", "tools/", ".claude/", ".github/",
}

// backtick captures the content of every single-backtick code span.
var backtick = regexp.MustCompile("`([^`\n]+)`")

// skillRef captures a /gk-<name> skill cross-reference.
var skillRef = regexp.MustCompile(`/gk-([a-z0-9-]+)`)

// lineAnchor splits a trailing :NN or :NN-MM line reference off a path.
var lineAnchor = regexp.MustCompile(`^(.*?):(\d+)(?:-\d+)?$`)

// symbolAnchor splits a trailing :identifier off a path — the docs point at a
// function as `assets/app.ts:bootstrap`. Only the file is checkable here; the
// symbol is prose.
var symbolAnchor = regexp.MustCompile(`^(.*?):[A-Za-z_][A-Za-z0-9_]*$`)

// ignoreMarker captures `<!-- gkdoc:ignore <path> <reason> -->`. The reason is
// whatever follows the path; an em-dash separator is conventional, not required.
var ignoreMarker = regexp.MustCompile(`<!--\s*gkdoc:ignore\s+(\S+)\s*(.*?)\s*-->`)

type violation struct {
	file   string // repo-relative doc path
	line   int
	detail string
}

// result is what one scan found — split out so the test can drive run()
// against a fixture module instead of the repo.
type result struct {
	violations []violation
	candidates int
	docs       int
}

func Run(args []string) {
	if len(args) > 0 {
		fatalf("unknown argument %q (usage: gk docpaths)", args[0])
	}
	root, err := tool.RepoRoot()
	if err != nil {
		fatalf("%v", err)
	}
	res, err := run(root, repoScan)
	if err != nil {
		fatalf("%v", err)
	}
	if len(res.violations) > 0 {
		report(res)
	}
	fmt.Printf("docpaths: ok — %d path citations across %d docs all resolve\n",
		res.candidates, res.docs)
}

// run scans every doc under root. It is the testable half of Run.
func run(root string, sc scan) (*result, error) {
	docs, err := collectDocs(root, sc)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf(
			"found no .md files under %s — the gate is looking in the wrong place",
			strings.Join(sc.docRoots, ", "))
	}

	res := &result{docs: len(docs)}
	for _, doc := range docs {
		v, n, err := checkDoc(root, doc, sc)
		if err != nil {
			return nil, err
		}
		res.violations = append(res.violations, v...)
		res.candidates += n
	}
	if res.candidates == 0 {
		return nil, fmt.Errorf(
			"scanned %d docs but found no repo-path citations at all — "+
				"the extractor is broken, not the docs", len(docs))
	}
	return res, nil
}

// collectDocs walks docRoots for markdown files, sorted for stable output. A
// missing root is not an error: a fixture module carries only the tree its
// case needs.
func collectDocs(root string, sc scan) ([]string, error) {
	var docs []string
	for _, dir := range sc.docRoots {
		if _, err := os.Stat(filepath.Join(root, dir)); err != nil {
			continue
		}
		err := filepath.WalkDir(
			filepath.Join(root, dir),
			func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !d.IsDir() && strings.HasSuffix(path, ".md") {
					rel, relErr := filepath.Rel(root, path)
					if relErr != nil {
						return relErr
					}
					docs = append(docs, rel)
				}
				return nil
			},
		)
		if err != nil {
			return nil, fmt.Errorf("walk %s: %w", dir, err)
		}
	}
	sort.Strings(docs)
	return docs, nil
}

// checkDoc verifies every path citation and skill reference in one doc and
// returns its violations plus the number of path candidates it examined.
func checkDoc(root, doc string, sc scan) ([]violation, int, error) {
	src, err := os.ReadFile(filepath.Join(root, doc))
	if err != nil {
		return nil, 0, fmt.Errorf("read %s: %w", doc, err)
	}
	lines := strings.Split(string(src), "\n")

	exempt, markerViolations := parseIgnores(doc, lines)
	violations := markerViolations
	candidates := 0

	for i, line := range lines {
		for _, m := range backtick.FindAllStringSubmatch(line, -1) {
			span := m[1]
			if !isPathCandidate(span) {
				continue
			}
			candidates++
			if _, ok := exempt[span]; ok {
				exempt[span] = true // consumed
				continue
			}
			if d := resolve(root, span); d != "" {
				violations = append(violations, violation{doc, i + 1, d})
			}
		}
		for _, m := range skillRef.FindAllStringSubmatch(line, -1) {
			name := "gk-" + m[1]
			if _, err := os.Stat(filepath.Join(root, sc.skillsDir, name, "SKILL.md")); err != nil {
				violations = append(violations, violation{doc, i + 1,
					fmt.Sprintf(
						"skill reference /%s has no %s/%s/SKILL.md",
						name,
						sc.skillsDir,
						name,
					)})
			}
		}
	}

	for path, used := range exempt {
		if !used {
			violations = append(violations, violation{doc, 0,
				fmt.Sprintf(
					"gkdoc:ignore %q exempts nothing — the doc never cites that path; drop the marker",
					path,
				)})
		}
	}
	return violations, candidates, nil
}

// parseIgnores collects the file's escape markers. A reasonless marker is
// itself a violation.
func parseIgnores(doc string, lines []string) (map[string]bool, []violation) {
	exempt := map[string]bool{}
	var violations []violation
	for i, line := range lines {
		for _, m := range ignoreMarker.FindAllStringSubmatch(line, -1) {
			path, reason := m[1], strings.TrimLeft(m[2], "—- \t")
			if strings.TrimSpace(reason) == "" {
				violations = append(violations, violation{doc, i + 1,
					fmt.Sprintf(
						"gkdoc:ignore %q has no reason — say why the path is fictional",
						path,
					)})
				continue
			}
			exempt[path] = false
		}
	}
	return exempt, violations
}

// isPathCandidate reports whether a backtick span claims to be a repo path.
// An ellipsis means the doc elided part of the name (`migrations/…_runs.sql`),
// which is prose, not a citation — there is nothing to resolve.
func isPathCandidate(s string) bool {
	if strings.ContainsAny(s, " \t<>|") || strings.Contains(s, "://") {
		return false
	}
	if strings.Contains(s, "…") {
		return false
	}
	for _, r := range repoRoots {
		if strings.HasPrefix(s, r) {
			return true
		}
	}
	return false
}

// expandBraces turns `a/{b,c}.go` — the docs' shorthand for a set of sibling
// files — into one path per member. One level deep is all the prose uses.
func expandBraces(s string) []string {
	i := strings.Index(s, "{")
	j := strings.Index(s, "}")
	if i < 0 || j < i {
		return []string{s}
	}
	var out []string
	for _, member := range strings.Split(s[i+1:j], ",") {
		out = append(out, s[:i]+strings.TrimSpace(member)+s[j+1:])
	}
	return out
}

// resolve returns "" when every path a span names resolves, or the reason the
// first one does not.
func resolve(root, span string) string {
	for _, path := range expandBraces(span) {
		if d := resolveOne(root, path, span); d != "" {
			return d
		}
	}
	return ""
}

// resolveOne checks a single concrete path. It understands four shapes: a :NN
// line anchor, a :symbol anchor, a `*`/`...` subtree pattern, and a plain file
// or directory.
func resolveOne(root, path, span string) string {
	line := 0
	if m := lineAnchor.FindStringSubmatch(path); m != nil {
		// A Go package pattern (app/...) ends in dots, not digits, so only a
		// real :NN anchor reaches here.
		path = m[1]
		line, _ = strconv.Atoi(m[2])
	} else if m := symbolAnchor.FindStringSubmatch(path); m != nil {
		path = m[1]
	}
	path = strings.TrimSuffix(path, "/")

	if i := strings.IndexAny(path, "*"); i >= 0 {
		return resolveBase(root, path[:i], span)
	}
	if strings.HasSuffix(path, "/...") {
		return resolveBase(root, strings.TrimSuffix(path, "..."), span)
	}

	info, err := os.Stat(filepath.Join(root, path))
	if err != nil {
		return fmt.Sprintf("%s does not exist", span)
	}
	if line > 0 {
		if info.IsDir() {
			return fmt.Sprintf("%s anchors a line on a directory", span)
		}
		src, readErr := os.ReadFile(filepath.Join(root, path))
		if readErr != nil {
			return fmt.Sprintf("%s: %v", span, readErr)
		}
		if n := strings.Count(string(src), "\n") + 1; line > n {
			return fmt.Sprintf("%s anchors line %d but the file has %d", span, line, n)
		}
	}
	return ""
}

// resolveBase checks the fixed directory prefix of a subtree pattern.
func resolveBase(root, base, span string) string {
	base = strings.TrimSuffix(base, "/")
	if base == "" {
		return ""
	}
	if _, err := os.Stat(filepath.Join(root, base)); err != nil {
		return fmt.Sprintf("%s: the %s subtree does not exist", span, base)
	}
	return ""
}

func report(res *result) {
	var b strings.Builder
	for _, v := range res.violations {
		if v.line > 0 {
			fmt.Fprintf(&b, "\n  %s:%d: %s", v.file, v.line, v.detail)
		} else {
			fmt.Fprintf(&b, "\n  %s: %s", v.file, v.detail)
		}
	}
	fatalf("%d dead reference(s) across %d docs (%d citations checked):%s\n\n"+
		"Every path a doc cites must resolve. Fix the path, or — if it is a\n"+
		"deliberately fictional example — mark it:\n"+
		"  <!-- gkdoc:ignore <path> — <why it is fictional> -->",
		len(res.violations), res.docs, res.candidates, b.String())
}

func fatalf(format string, args ...any) {
	tool.Fatalf("docpaths", format, args...)
}
