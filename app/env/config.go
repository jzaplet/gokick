package env

import (
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	HTTPPort             string
	DBPath               string
	JWTSecret            string
	JWTAccessExpiration  time.Duration
	JWTRefreshExpiration time.Duration
	CORSOrigin           string
}

func LoadConfig() (*Config, error) {
	_ = godotenv.Load()

	config := &Config{
		HTTPPort:   getEnv("APP_HTTP_PORT", "3000"),
		DBPath:     getEnv("APP_DB_PATH", "./data/app.db"),
		JWTSecret:  os.Getenv("APP_JWT_SECRET"),
		CORSOrigin: getEnv("APP_CORS_ORIGIN", "http://localhost:5173"),
	}

	var err error

	config.JWTAccessExpiration, err = time.ParseDuration(getEnv("APP_JWT_ACCESS_EXPIRATION", "15m"))
	if err != nil {
		return nil, fmt.Errorf("invalid APP_JWT_ACCESS_EXPIRATION: %w", err)
	}

	config.JWTRefreshExpiration, err = time.ParseDuration(getEnv("APP_JWT_REFRESH_EXPIRATION", "168h"))
	if err != nil {
		return nil, fmt.Errorf("invalid APP_JWT_REFRESH_EXPIRATION: %w", err)
	}

	return config, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
