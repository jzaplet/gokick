// Package tool holds the pieces every gk subcommand shares: repo-root
// discovery, the prefix-carrying fatal, the escape-marker line scanner, and
// the generate/check pair every code-generating subcommand needs. One
// implementation instead of a copy per subcommand — the copies had already
// drifted (boundary honored trailing same-line markers, errfields did not),
// and a drifting escape-hatch dialect is exactly the kind of bug these gates
// exist to prevent.
package tool

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// RepoRoot resolves the gokick module root by walking up from the working
// directory (gk runs from tools/gk).
func RepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getwd: %w", err)
	}
	for {
		mod := filepath.Join(dir, "go.mod")
		if b, err := os.ReadFile(mod); err == nil &&
			strings.Contains(string(b), "module gokick\n") {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find gokick go.mod above %s", dir)
		}
		dir = parent
	}
}

// Fatalf prints "<prefix>: <message>" to stderr and exits 1.
func Fatalf(prefix, format string, args ...any) {
	fmt.Fprintf(os.Stderr, prefix+": "+format+"\n", args...)
	os.Exit(1)
}

// WriteArtifacts writes every generated file — repo-relative path to exact
// content — creating parent directories on the way, and reports each write on
// stdout in path order (a stable log is what makes a generator diffable).
func WriteArtifacts(prefix, root string, files map[string][]byte) error {
	for _, rel := range slices.Sorted(maps.Keys(files)) {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return fmt.Errorf("mkdir for %s: %w", rel, err)
		}
		if err := os.WriteFile(abs, files[rel], 0o644); err != nil {
			return fmt.Errorf("write %s: %w", rel, err)
		}
		fmt.Printf("%s: wrote %s\n", prefix, rel)
	}
	return nil
}

// CheckArtifacts is the freshness half of WriteArtifacts: it returns one
// sorted message per generated file that is missing from disk or differs from
// what the generator just produced (empty = every artifact current). regenCmd
// names the make target that fixes it. The comparison is byte-exact — the
// generator is the only author of these files.
func CheckArtifacts(root string, files map[string][]byte, regenCmd string) []string {
	var stale []string
	for _, rel := range slices.Sorted(maps.Keys(files)) {
		got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		switch {
		case err != nil:
			stale = append(stale, fmt.Sprintf(
				"%s is missing — run `%s` and commit the result", rel, regenCmd))
		case !bytes.Equal(got, files[rel]):
			stale = append(stale, fmt.Sprintf(
				"%s is stale — run `%s` and commit the result", rel, regenCmd))
		}
	}
	return stale
}

// Escape is one escape-hatch marker comment (//gkts:ignore, //gkerrf:exempt).
type Escape struct {
	Reason     string // text after the marker, whitespace-trimmed; "" = missing
	MarkerLine int    // the marker comment's own line
}

// EscapeLines finds every standalone `//<marker> <reason>` comment in f and
// maps THE LINE BELOW the marker to its Escape — one shared semantic for all
// gk escape hatches: the marker stands alone on its own line and covers
// exactly the statement that starts on the next line. A marker trailing code
// on the same line is NOT an escape; it is returned in misplaced so the
// caller can fail loudly instead of silently ignoring it (a trailing marker
// used to leak its exemption onto the following line). Reason validation and
// unused-marker detection stay with the caller — the semantics of "consumed"
// differ per tool.
func EscapeLines(
	fset *token.FileSet,
	f *ast.File,
	src []byte,
	marker string,
) (covered map[int]Escape, misplaced []int) {
	srcLines := strings.Split(string(src), "\n")
	covered = map[int]Escape{}
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			text := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
			if !strings.HasPrefix(text, marker) {
				continue
			}
			pos := fset.Position(c.Pos())
			if pos.Line-1 < len(srcLines) {
				prefix := srcLines[pos.Line-1][:pos.Column-1]
				if strings.TrimSpace(prefix) != "" {
					misplaced = append(misplaced, pos.Line)
					continue
				}
			}
			reason := strings.TrimSpace(strings.TrimPrefix(text, marker))
			covered[pos.Line+1] = Escape{Reason: reason, MarkerLine: pos.Line}
		}
	}
	return covered, misplaced
}
