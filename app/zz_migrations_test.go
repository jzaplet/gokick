package app_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Migration-twins gate. Every migration exists once per dialect under the SAME
// version — migrations/sqlite/<v>_x.sql and migrations/postgres/<v>_x.sql — so a
// deployment reaches the same logical schema whichever adapter it runs, and a
// schema change written for one database cannot ship without its twin.

var migrationVersionRe = regexp.MustCompile(`^(\d+)_[^/]+\.sql$`)

// migrationDialects are the per-dialect directories under migrations/.
var migrationDialects = []string{"sqlite", "postgres"}

// migrationVersions returns the versions of the .sql files in dir, and the names
// of files that carry no version prefix (goose would refuse them).
func migrationVersions(t *testing.T, dir string) (versions map[string]bool, unversioned []string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	versions = map[string]bool{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		m := migrationVersionRe.FindStringSubmatch(e.Name())
		if m == nil {
			unversioned = append(unversioned, e.Name())
			continue
		}
		versions[m[1]] = true
	}
	return versions, unversioned
}

// twinViolations lists every version present in one dialect but not in another.
func twinViolations(byDialect map[string]map[string]bool) []string {
	var v []string
	for dialect, versions := range byDialect {
		for version := range versions {
			for other, otherVersions := range byDialect {
				if other != dialect && !otherVersions[version] {
					v = append(
						v,
						fmt.Sprintf("version %s is in migrations/%s but not in migrations/%s",
							version, dialect, other),
					)
				}
			}
		}
	}
	sort.Strings(v)
	return v
}

func TestMigrations_EveryVersionHasItsTwin(t *testing.T) {
	byDialect := map[string]map[string]bool{}
	for _, dialect := range migrationDialects {
		versions, unversioned := migrationVersions(
			t,
			filepath.Join(repoRoot(), "migrations", dialect),
		)
		if len(unversioned) > 0 {
			t.Errorf(
				"migrations/%s has files without a <version>_ prefix: %v",
				dialect,
				unversioned,
			)
		}
		// Anti-vacuity: an empty directory would pair with anything.
		if len(versions) == 0 {
			t.Fatalf("migrations/%s holds no migration — the scan is looking in the wrong place",
				dialect)
		}
		byDialect[dialect] = versions
	}
	if v := twinViolations(byDialect); len(v) > 0 {
		t.Fatalf("migrations without their twin:\n  %s", strings.Join(v, "\n  "))
	}
}

// The gate bites: a version on one side only is reported, from either side.
func TestMigrations_TwinGateBites(t *testing.T) {
	v := twinViolations(map[string]map[string]bool{
		"sqlite":   {"1": true, "2": true},
		"postgres": {"1": true, "3": true},
	})
	want := []string{
		"version 2 is in migrations/sqlite but not in migrations/postgres",
		"version 3 is in migrations/postgres but not in migrations/sqlite",
	}
	if strings.Join(v, "|") != strings.Join(want, "|") {
		t.Fatalf("twinViolations = %v, want %v", v, want)
	}
	if v := twinViolations(map[string]map[string]bool{
		"sqlite": {"1": true}, "postgres": {"1": true},
	}); len(v) != 0 {
		t.Fatalf("matching sets must pass, got %v", v)
	}
}
