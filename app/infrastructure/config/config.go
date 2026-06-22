package config

import (
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	HTTPPort             string
	DBPath               string
	DBJournalMode        string
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
}

func LoadConfig() (*Config, error) {
	_ = godotenv.Load()

	config := &Config{
		HTTPPort:               getEnv("APP_HTTP_PORT", "3000"),
		DBPath:                 getEnv("APP_DB_PATH", "./data/app.db"),
		DBJournalMode:          getEnv("APP_DB_JOURNAL_MODE", "WAL"),
		JWTSecret:              getEnv("APP_JWT_SECRET", ""),
		CORSOrigin:             getEnv("APP_CORS_ORIGIN", "http://localhost:5173"),
		CookieSecure:           getEnv("APP_COOKIE_SECURE", "true") == "true",
		SeedAdminPassword:      getEnv("APP_SEED_ADMIN_PASSWORD", ""),
		SeedSuperAdminPassword: getEnv("APP_SEED_SUPERADMIN_PASSWORD", ""),
		SeedAdminTenant:        getEnv("APP_SEED_ADMIN_TENANT", "Tenant 1"),
		TrustProxyHeaders:      getEnv("APP_TRUST_PROXY_HEADERS", "false") == "true",
		Multitenancy:           getEnv("APP_MULTITENANCY", "false") == "true",
		RateLimitLogin:         getEnv("APP_RATE_LIMIT_LOGIN", "10/min"),
		RateLimitRefresh:       getEnv("APP_RATE_LIMIT_REFRESH", "60/min"),
		FrontendSentryDSN:      getEnv("APP_SENTRY_DSN_FRONTEND", ""),
		SentryEnvironment:      getEnv("APP_SENTRY_ENVIRONMENT", ""),
		SentryDebug:            getEnv("APP_SENTRY_DEBUG", "false") == "true",
	}

	var err error

	config.JWTAccessExpiration, err = time.ParseDuration(getEnv("APP_JWT_ACCESS_EXPIRATION", "15m"))
	if err != nil {
		return nil, fmt.Errorf("invalid APP_JWT_ACCESS_EXPIRATION: %w", err)
	}

	config.JWTRefreshExpiration, err = time.ParseDuration(
		getEnv("APP_JWT_REFRESH_EXPIRATION", "168h"),
	)
	if err != nil {
		return nil, fmt.Errorf("invalid APP_JWT_REFRESH_EXPIRATION: %w", err)
	}

	return config, nil
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
}

// LoadStartup loads .env (best-effort) and reads the bootstrap configuration.
// LoadConfig loads .env again later; godotenv.Load never overrides already-set
// vars, so the repeat is harmless.
func LoadStartup() StartupConfig {
	_ = godotenv.Load()

	return StartupConfig{
		LogFormat:         getEnv("APP_LOG_FORMAT", ""),
		LogLevel:          getEnv("APP_LOG_LEVEL", ""),
		SentryDSN:         getEnv("APP_SENTRY_DSN", ""),
		SentryEnvironment: getEnv("APP_SENTRY_ENVIRONMENT", ""),
		SentryRelease:     getEnv("APP_SENTRY_RELEASE", ""),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
