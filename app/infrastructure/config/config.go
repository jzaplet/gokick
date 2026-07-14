package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	HTTPPort      string
	DBPath        string
	DBJournalMode string
	// DBMaxConns caps the SQLite connection pool. <= 0 means auto (NewSqliteManager
	// derives it from CPU count). APP_DB_MAX_CONNS overrides.
	DBMaxConns           int
	JWTSecret            string
	JWTAccessExpiration  time.Duration
	JWTRefreshExpiration time.Duration
	CORSOrigin           string
	CookieSecure         bool
	SeedAdminPassword    string
	// SeedSuperAdminPassword is OPTIONAL — empty means "do not seed a superadmin"
	// (the platform plane is off by default). When set, the seeder mints a
	// superadmin account.
	SeedSuperAdminPassword string
	// SeedAdminTenant names the tenant the seeded admin lands in WHEN multitenancy
	// is on (find-or-create). With multitenancy off the admin stays in the default
	// tenant and this is ignored.
	SeedAdminTenant string

	// Sentry frontend config — injected into index.html at serve time so the
	// SPA reads DSN + environment at runtime (one built image works across
	// environments; the DSN is public, safe to embed). The release stays baked
	// at build (VITE_SENTRY_RELEASE) since the image is per-version. Backend
	// Sentry + logger config is read separately via StartupConfig (below),
	// before this Config loads.
	//
	// SentryDebug gates the BE + FE error-trigger affordances used to verify
	// Sentry end-to-end. Keep it OFF in production — the app logs a warning at
	// startup when it is on, since it exposes deliberate error triggers.
	FrontendSentryDSN string
	SentryEnvironment string
	SentryDebug       bool

	// RunDebug gates the /debug/runs endpoints and the e2e:* test run kinds, so a
	// live or local deploy can exercise the durable-run engine's crash-recovery /
	// drain guarantees that an in-process test cannot reach. Keep it OFF in
	// production — it lets any caller enqueue and cancel runs; the app logs a
	// warning at startup when it is on.
	RunDebug bool

	// TrustProxyHeaders flips IP extraction from RemoteAddr to X-Real-IP.
	// Leave false unless the app sits behind a reverse proxy that you
	// trust to rewrite X-Real-IP — any client can forge the header
	// otherwise and bypass per-IP rate limiting in one curl.
	TrustProxyHeaders bool

	// Multitenancy picks the tenant-scoping ENFORCEMENT mode (resolution is
	// always data-driven: the JWT carries the tenant, the resolver returns it).
	// false (default): fail-OPEN — a request with no resolved tenant falls back
	// to the default tenant, so a single-tenant deployment works unchanged.
	// true: fail-CLOSED — a missing tenant panics rather than silently scoping
	// to the default tenant (the cross-tenant leak this whole epic prevents).
	// A buggy multitenant app is indistinguishable from a single-tenant one in
	// the data, so this can't be inferred — it must be configured.
	Multitenancy bool

	// Rate-limit rules in "N/duration" form (e.g. "10/min", "5/30s").
	// Empty disables that limit entirely.
	RateLimitLogin   string
	RateLimitRefresh string

	// Durable run worker (the "agent" engine). Lease is the crash-reclaim
	// window; Heartbeat must be << Lease (0 → the worker uses Lease/3); MaxInFlight
	// is the backpressure cap; MaxReclaims bounds a poison crash-loop.
	RunWorkerLease        time.Duration
	RunWorkerHeartbeat    time.Duration
	RunWorkerPoll         time.Duration
	RunWorkerDrainTimeout time.Duration
	RunWorkerMaxInFlight  int
	RunWorkerMaxReclaims  int
}

func LoadConfig() (*Config, error) {
	// A missing .env is fine (real env / defaults); a MALFORMED one is a broken
	// deploy config and fails fast here, consistent with every other LoadConfig
	// validation. LoadStartup ran first and already surfaced the same parse error
	// as a Warn before the logger existed — this is the authoritative hard stop.
	if err := loadDotenv(); err != nil {
		return nil, fmt.Errorf("invalid .env: %w", err)
	}

	config := &Config{
		HTTPPort:               getEnv("APP_HTTP_PORT", "3000"),
		DBPath:                 getEnv("APP_DB_PATH", "./data/app.db"),
		DBJournalMode:          getEnv("APP_DB_JOURNAL_MODE", "WAL"),
		JWTSecret:              getEnv("APP_JWT_SECRET", ""),
		CORSOrigin:             getEnv("APP_CORS_ORIGIN", "http://localhost:5173"),
		SeedAdminPassword:      getEnv("APP_SEED_ADMIN_PASSWORD", ""),
		SeedSuperAdminPassword: getEnv("APP_SEED_SUPERADMIN_PASSWORD", ""),
		SeedAdminTenant:        getEnv("APP_SEED_ADMIN_TENANT", "Tenant 1"),
		RateLimitLogin:         getEnv("APP_RATE_LIMIT_LOGIN", "10/min"),
		RateLimitRefresh:       getEnv("APP_RATE_LIMIT_REFRESH", "60/min"),
		FrontendSentryDSN:      getEnv("APP_SENTRY_DSN_FRONTEND", ""),
		SentryEnvironment:      getEnv("APP_SENTRY_ENVIRONMENT", ""),
	}

	var err error

	// Booleans are parsed strictly (getEnvBool): a typo fails fast instead of
	// silently coercing to false — for APP_COOKIE_SECURE that would ship insecure
	// cookies + drop HSTS unnoticed.
	for _, b := range []struct {
		dst *bool
		key string
		def bool
	}{
		{&config.CookieSecure, "APP_COOKIE_SECURE", true},
		{&config.TrustProxyHeaders, "APP_TRUST_PROXY_HEADERS", false},
		{&config.Multitenancy, "APP_MULTITENANCY", false},
		{&config.SentryDebug, "APP_SENTRY_DEBUG", false},
		{&config.RunDebug, "APP_RUN_DEBUG", false},
	} {
		if *b.dst, err = getEnvBool(b.key, b.def); err != nil {
			return nil, err
		}
	}

	config.JWTAccessExpiration, err = time.ParseDuration(getEnv("APP_JWT_ACCESS_EXPIRATION", "15m"))
	if err != nil {
		return nil, fmt.Errorf("invalid APP_JWT_ACCESS_EXPIRATION: %w", err)
	}
	// Sign guard sits with the parse (its sibling): a non-positive expiration parses
	// fine but would mint already-expired / never-expiring tokens. The JwtService
	// constructor stays unguarded on purpose — tests feed it a negative access
	// expiration to exercise expired-token rejection; operator config is validated here.
	if config.JWTAccessExpiration <= 0 {
		return nil, fmt.Errorf("APP_JWT_ACCESS_EXPIRATION must be positive")
	}

	config.JWTRefreshExpiration, err = time.ParseDuration(
		getEnv("APP_JWT_REFRESH_EXPIRATION", "168h"),
	)
	if err != nil {
		return nil, fmt.Errorf("invalid APP_JWT_REFRESH_EXPIRATION: %w", err)
	}
	if config.JWTRefreshExpiration <= 0 {
		return nil, fmt.Errorf("APP_JWT_REFRESH_EXPIRATION must be positive")
	}

	// 0 = auto (NewSqliteManager derives the cap from CPU count).
	if config.DBMaxConns, err = getEnvInt("APP_DB_MAX_CONNS", 0); err != nil {
		return nil, fmt.Errorf("invalid APP_DB_MAX_CONNS: %w", err)
	}

	if err := loadRunWorkerConfig(config); err != nil {
		return nil, err
	}

	return config, nil
}

// loadRunWorkerConfig parses the durable-run worker knobs onto config, split out
// of LoadConfig to keep it under the length gate.
func loadRunWorkerConfig(config *Config) error {
	for _, d := range []struct {
		dst *time.Duration
		key string
		def string
	}{
		{&config.RunWorkerLease, "APP_RUN_WORKER_LEASE", "5m"},
		{&config.RunWorkerHeartbeat, "APP_RUN_WORKER_HEARTBEAT", "0s"}, // 0 → worker uses Lease/3
		{&config.RunWorkerPoll, "APP_RUN_WORKER_POLL", "1s"},
		{&config.RunWorkerDrainTimeout, "APP_RUN_WORKER_DRAIN_TIMEOUT", "10s"},
	} {
		var err error
		if *d.dst, err = time.ParseDuration(getEnv(d.key, d.def)); err != nil {
			return fmt.Errorf("invalid %s: %w", d.key, err)
		}
	}

	var err error
	if config.RunWorkerMaxInFlight, err = getEnvInt("APP_RUN_WORKER_MAX_IN_FLIGHT", 8); err != nil {
		return fmt.Errorf("invalid APP_RUN_WORKER_MAX_IN_FLIGHT: %w", err)
	}
	if config.RunWorkerMaxReclaims, err = getEnvInt("APP_RUN_WORKER_MAX_RECLAIMS", 20); err != nil {
		return fmt.Errorf("invalid APP_RUN_WORKER_MAX_RECLAIMS: %w", err)
	}
	return nil
}

// StartupConfig is the slice of configuration read at the very start of main —
// before the full Config — because the logger and error reporter are built
// first, so that a failure inside LoadConfig itself can still be logged and
// reported. It is read through the same getEnv path as Config, keeping os.Getenv
// in exactly one place (getEnv) instead of scattered raw across cmd/.
type StartupConfig struct {
	LogFormat         string
	LogLevel          string
	SentryDSN         string
	SentryEnvironment string
	SentryRelease     string
	// DotenvError carries a malformed-.env parse error (nil when .env is absent or
	// valid). LoadStartup runs before the logger exists, so it stashes the error
	// here for main to Warn once the logger is built.
	DotenvError error
}

// LoadStartup loads .env (best-effort) and reads the bootstrap configuration.
// A malformed .env is stashed in DotenvError (not fatal here — the logger it
// would be reported through doesn't exist yet). LoadConfig loads .env again later
// and fails fast on the same parse error; godotenv never overrides already-set
// vars, so the repeat is harmless.
func LoadStartup() StartupConfig {
	dotenvErr := loadDotenv()

	return StartupConfig{
		LogFormat:         getEnv("APP_LOG_FORMAT", ""),
		LogLevel:          getEnv("APP_LOG_LEVEL", ""),
		SentryDSN:         getEnv("APP_SENTRY_DSN", ""),
		SentryEnvironment: getEnv("APP_SENTRY_ENVIRONMENT", ""),
		SentryRelease:     getEnv("APP_SENTRY_RELEASE", ""),
		DotenvError:       dotenvErr,
	}
}

// loadDotenv loads .env, treating an absent file as success (real env / defaults)
// but returning a genuine parse error so callers can surface it — a malformed
// .env must never be silently indistinguishable from an absent one.
func loadDotenv() error {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// getEnvInt reads an integer env var, falling back when unset/empty.
func getEnvInt(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	return strconv.Atoi(v)
}

// getEnvBool reads a strict boolean env var: only "true" or "false" are accepted.
// Any other non-empty value is an error, so a typo fails fast at load rather than
// silently coercing to false (the trap the old `== "true"` had).
func getEnvBool(key string, fallback bool) (bool, error) {
	switch os.Getenv(key) {
	case "":
		return fallback, nil
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be \"true\" or \"false\"", key)
	}
}
