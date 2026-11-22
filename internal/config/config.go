package config

import (
	"os"
	"strconv"
)

// Config holds runtime configuration sourced from environment variables.
type Config struct {
	Port        string
	DatabaseURL string

	// OIDC (Keycloak). Auth is enforced only when AuthEnabled is true.
	AuthEnabled  bool
	OIDCIssuer   string
	OIDCClientID string

	// MaxUploadBytes caps EDD uploads (default 32 MiB).
	MaxUploadBytes int64
}

// FromEnv builds a Config from the process environment, applying dev defaults.
func FromEnv() Config {
	return Config{
		Port:           getenv("PORT", "8080"),
		DatabaseURL:    getenv("DATABASE_URL", "postgres://samplier:samplier@localhost:5432/samplier"),
		AuthEnabled:    os.Getenv("AUTH_ENABLED") == "true",
		OIDCIssuer:     os.Getenv("OIDC_ISSUER"),
		OIDCClientID:   os.Getenv("OIDC_CLIENT_ID"),
		MaxUploadBytes: int64(getenvInt("MAX_UPLOAD_BYTES", 32<<20)),
	}
}

func getenvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
