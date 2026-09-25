package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gokick/app/infrastructure/database"
)

// F-065: a malformed .env must fail fast at LoadConfig, not be silently swallowed
// (indistinguishable from an absent .env). An absent .env stays fine (defaults).
func TestLoadConfig_MalformedDotenvFails(t *testing.T) {
	dir := t.TempDir()
	// A line with no '=' separator — godotenv can't split key from value.
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("THIS_IS_NOT_A_PAIR\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "invalid .env") {
		t.Fatalf("expected an invalid .env error, got %v", err)
	}
}

func TestLoadConfig_AbsentDotenvIsFine(t *testing.T) {
	t.Chdir(t.TempDir()) // empty dir, no .env
	if _, err := LoadConfig(); err != nil {
		t.Fatalf("absent .env must not error, got %v", err)
	}
}

// LoadConfig parses the APP_RUN_WORKER_* env vars onto the Config. No test exercised
// this glue, so its parse + wrapped-error branches were dead. These do NOT use
// t.Parallel — t.Setenv forbids it.

func TestLoadConfig_RunWorker_ParsesValues(t *testing.T) {
	t.Setenv("APP_RUN_WORKER_LEASE", "42s")
	t.Setenv("APP_RUN_WORKER_HEARTBEAT", "7s")
	t.Setenv("APP_RUN_WORKER_POLL", "2s")
	t.Setenv("APP_RUN_WORKER_DRAIN_TIMEOUT", "5s")
	t.Setenv("APP_RUN_WORKER_MAX_IN_FLIGHT", "3")
	t.Setenv("APP_RUN_WORKER_MAX_RECLAIMS", "9")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.RunWorkerLease != 42*time.Second {
		t.Fatalf("RunWorkerLease: got %s want 42s", cfg.RunWorkerLease)
	}
	if cfg.RunWorkerHeartbeat != 7*time.Second {
		t.Fatalf("RunWorkerHeartbeat: got %s want 7s", cfg.RunWorkerHeartbeat)
	}
	if cfg.RunWorkerPoll != 2*time.Second {
		t.Fatalf("RunWorkerPoll: got %s want 2s", cfg.RunWorkerPoll)
	}
	if cfg.RunWorkerDrainTimeout != 5*time.Second {
		t.Fatalf("RunWorkerDrainTimeout: got %s want 5s", cfg.RunWorkerDrainTimeout)
	}
	if cfg.RunWorkerMaxInFlight != 3 {
		t.Fatalf("RunWorkerMaxInFlight: got %d want 3", cfg.RunWorkerMaxInFlight)
	}
	if cfg.RunWorkerMaxReclaims != 9 {
		t.Fatalf("RunWorkerMaxReclaims: got %d want 9", cfg.RunWorkerMaxReclaims)
	}
}

func TestLoadConfig_RunWorker_InvalidDurationFails(t *testing.T) {
	t.Setenv("APP_RUN_WORKER_LEASE", "banana")
	_, err := LoadConfig()
	if err == nil || !strings.Contains(err.Error(), "invalid APP_RUN_WORKER_LEASE") {
		t.Fatalf("expected a wrapped invalid-duration error, got %v", err)
	}
}

func TestLoadConfig_RunWorker_InvalidIntFails(t *testing.T) {
	t.Setenv("APP_RUN_WORKER_MAX_IN_FLIGHT", "foo")
	_, err := LoadConfig()
	if err == nil || !strings.Contains(err.Error(), "invalid APP_RUN_WORKER_MAX_IN_FLIGHT") {
		t.Fatalf("expected a wrapped invalid-int error, got %v", err)
	}
}

// F-050: a boolean typo must fail fast, not silently coerce to false (which for
// APP_COOKIE_SECURE would ship insecure cookies).
func TestLoadConfig_StrictBool_RejectsTypo(t *testing.T) {
	t.Setenv("APP_COOKIE_SECURE", "ture")
	_, err := LoadConfig()
	if err == nil || !strings.Contains(err.Error(), "APP_COOKIE_SECURE") {
		t.Fatalf("expected a strict-bool error naming the key, got %v", err)
	}
}

// APP_DB_DRIVER picks the adapter: unset means sqlite, a known name is taken as
// is, and a typo fails the start instead of silently running on the default.
func TestLoadConfig_DBDriver(t *testing.T) {
	t.Chdir(t.TempDir()) // no .env: only the vars set below count
	setPostgresURLs(t)

	for env, want := range map[string]database.Driver{
		"":         database.DriverSQLite,
		"sqlite":   database.DriverSQLite,
		"postgres": database.DriverPostgres,
	} {
		t.Setenv("APP_DB_DRIVER", env)
		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("APP_DB_DRIVER=%q: %v", env, err)
		}
		if cfg.DBDriver != want {
			t.Fatalf("APP_DB_DRIVER=%q: got %q want %q", env, cfg.DBDriver, want)
		}
	}

	t.Setenv("APP_DB_DRIVER", "sqlite3")
	if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "APP_DB_DRIVER") {
		t.Fatalf("expected an APP_DB_DRIVER error for a typo, got %v", err)
	}
}

// setPostgresURLs sets the three role DSNs APP_DB_DRIVER=postgres requires.
func setPostgresURLs(t *testing.T) {
	t.Helper()
	t.Setenv("APP_DB_URL", "postgres://gokick_app:pw@db:5432/gokick?sslmode=disable")
	t.Setenv("APP_DB_SYSTEM_URL", "postgres://gokick_system:pw@db:5432/gokick")
	t.Setenv("APP_DB_MIGRATE_URL", "postgresql://gokick_owner:pw@db/gokick")
}

func TestLoadConfig_PostgresURLs(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("APP_DB_DRIVER", "postgres")
	setPostgresURLs(t)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.DBURL == "" || cfg.DBSystemURL == "" || cfg.DBMigrateURL == "" {
		t.Fatalf("DSNs not loaded: %+v", []string{cfg.DBURL, cfg.DBSystemURL, cfg.DBMigrateURL})
	}

	for _, key := range []string{"APP_DB_URL", "APP_DB_SYSTEM_URL", "APP_DB_MIGRATE_URL"} {
		for _, bad := range []struct{ value, want string }{
			{"", key + " is required"},
			{"mysql://u:secret-pw@db/gokick", "invalid " + key},
			{"postgres://db/gokick", "invalid " + key}, // no user
			{"postgres:///gokick?user=u", "invalid " + key},
		} {
			setPostgresURLs(t)
			t.Setenv(key, bad.value)
			_, err := LoadConfig()
			if err == nil || !strings.Contains(err.Error(), bad.want) {
				t.Fatalf(
					"%s=%q: got %v, want an error containing %q",
					key,
					bad.value,
					err,
					bad.want,
				)
			}
			if strings.Contains(err.Error(), "secret-pw") {
				t.Fatalf("the error must not echo the DSN (it carries a password): %v", err)
			}
		}
	}
}

// The DSNs are only required on Postgres — the SQLite default needs none.
func TestLoadConfig_PostgresURLsNotRequiredOnSQLite(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("APP_DB_DRIVER", "sqlite")
	if _, err := LoadConfig(); err != nil {
		t.Fatalf("sqlite must not require Postgres DSNs: %v", err)
	}
}

func TestLoadConfig_PostgresTimeouts(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.DBLockTimeout != 5*time.Second || cfg.DBStatementTimeout != 30*time.Second ||
		cfg.DBIdleTxTimeout != time.Minute {
		t.Fatalf("defaults: lock %s, statement %s, idle tx %s — want 5s/30s/1m",
			cfg.DBLockTimeout, cfg.DBStatementTimeout, cfg.DBIdleTxTimeout)
	}

	t.Setenv("APP_DB_LOCK_TIMEOUT", "0s") // 0 disables the limit
	t.Setenv("APP_DB_STATEMENT_TIMEOUT", "2m")
	if cfg, err = LoadConfig(); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.DBLockTimeout != 0 || cfg.DBStatementTimeout != 2*time.Minute {
		t.Fatalf("overrides: lock %s, statement %s", cfg.DBLockTimeout, cfg.DBStatementTimeout)
	}

	for _, bad := range []struct{ key, value string }{
		{"APP_DB_LOCK_TIMEOUT", "soon"},
		{"APP_DB_IDLE_TX_TIMEOUT", "-1s"},
	} {
		t.Setenv(bad.key, bad.value)
		if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), bad.key) {
			t.Fatalf("%s=%q: expected an error naming the key, got %v", bad.key, bad.value, err)
		}
		t.Setenv(bad.key, "")
	}
}

// F-054: a non-positive JWT expiration parses fine but would mint already-expired
// or never-expiring tokens — reject it fail-fast at load.
func TestLoadConfig_RejectsNonPositiveJWTExpiration(t *testing.T) {
	for _, tc := range []struct{ key, val string }{
		{"APP_JWT_ACCESS_EXPIRATION", "0s"},
		{"APP_JWT_ACCESS_EXPIRATION", "-5m"},
		{"APP_JWT_REFRESH_EXPIRATION", "-1h"},
	} {
		t.Run(tc.key+"="+tc.val, func(t *testing.T) {
			t.Setenv(tc.key, tc.val)
			_, err := LoadConfig()
			if err == nil || !strings.Contains(err.Error(), tc.key+" must be positive") {
				t.Fatalf("expected positive-required error for %s=%s, got %v", tc.key, tc.val, err)
			}
		})
	}
}

func TestLoadConfig_StrictBool_ParsesBothLiterals(t *testing.T) {
	t.Setenv("APP_MULTITENANCY", "true")
	t.Setenv("APP_COOKIE_SECURE", "false")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Multitenancy != true {
		t.Fatal("APP_MULTITENANCY=true must parse true")
	}
	if cfg.CookieSecure != false {
		t.Fatal("APP_COOKIE_SECURE=false must parse false")
	}
}

// F-069: APP_CORS_ORIGIN must be one concrete scheme://host[:port] origin.
// CORSMiddleware always sends Allow-Credentials: true, so a wildcard (or an
// empty/malformed value) is a broken deploy config and must fail at load.
func TestLoadConfig_RejectsBadCORSOrigin(t *testing.T) {
	for _, origin := range []string{
		"*",
		"null",
		"example.com",             // no scheme
		"ftp://example.com",       // non-http(s) scheme
		"http://example.com/app",  // path
		"http://example.com/",     // trailing slash is a path
		"http://example.com?q=1",  // query
		"http://user@example.com", // credentials
	} {
		t.Run(origin, func(t *testing.T) {
			t.Setenv("APP_CORS_ORIGIN", origin)
			if _, err := LoadConfig(); err == nil ||
				!strings.Contains(err.Error(), "APP_CORS_ORIGIN") {
				t.Fatalf("expected an APP_CORS_ORIGIN error for %q, got %v", origin, err)
			}
		})
	}
}

func TestLoadConfig_AcceptsConcreteCORSOrigin(t *testing.T) {
	for _, origin := range []string{
		"http://localhost:5173",
		"https://app.example.com",
	} {
		t.Run(origin, func(t *testing.T) {
			t.Setenv("APP_CORS_ORIGIN", origin)
			cfg, err := LoadConfig()
			if err != nil {
				t.Fatalf("a concrete origin must load, got %v", err)
			}
			if cfg.CORSOrigin != origin {
				t.Fatalf("CORSOrigin = %q, want %q", cfg.CORSOrigin, origin)
			}
		})
	}
}
