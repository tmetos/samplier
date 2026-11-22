package config

import "testing"

func TestFromEnvDefaults(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("AUTH_ENABLED", "")
	t.Setenv("OIDC_ISSUER", "")
	t.Setenv("OIDC_CLIENT_ID", "")
	t.Setenv("MAX_UPLOAD_BYTES", "")

	cfg := FromEnv()
	if cfg.Port != "8080" {
		t.Errorf("default port = %q", cfg.Port)
	}
	if cfg.DatabaseURL == "" {
		t.Error("expected default database URL")
	}
	if cfg.AuthEnabled {
		t.Error("auth must default to disabled")
	}
	if cfg.MaxUploadBytes != 32<<20 {
		t.Errorf("default max upload = %d", cfg.MaxUploadBytes)
	}
}

func TestFromEnvOverrides(t *testing.T) {
	t.Setenv("PORT", "9999")
	t.Setenv("DATABASE_URL", "postgres://x/y")
	t.Setenv("AUTH_ENABLED", "true")
	t.Setenv("OIDC_ISSUER", "https://idp/realms/samplier")
	t.Setenv("OIDC_CLIENT_ID", "samplier-ui")
	t.Setenv("MAX_UPLOAD_BYTES", "1024")

	cfg := FromEnv()
	if cfg.Port != "9999" || cfg.DatabaseURL != "postgres://x/y" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if !cfg.AuthEnabled {
		t.Error("expected auth enabled")
	}
	if cfg.OIDCIssuer != "https://idp/realms/samplier" || cfg.OIDCClientID != "samplier-ui" {
		t.Fatalf("unexpected OIDC config: %+v", cfg)
	}
	if cfg.MaxUploadBytes != 1024 {
		t.Errorf("max upload = %d", cfg.MaxUploadBytes)
	}
}

func TestInvalidMaxUploadFallsBack(t *testing.T) {
	t.Setenv("MAX_UPLOAD_BYTES", "not-a-number")
	if got := FromEnv().MaxUploadBytes; got != 32<<20 {
		t.Errorf("got %d, want default", got)
	}
	t.Setenv("MAX_UPLOAD_BYTES", "-5")
	if got := FromEnv().MaxUploadBytes; got != 32<<20 {
		t.Errorf("got %d, want default for negative", got)
	}
}

func TestGetenvFallback(t *testing.T) {
	t.Setenv("SAMPLIER_TEST_KEY", "")
	if got := getenv("SAMPLIER_TEST_KEY", "fallback"); got != "fallback" {
		t.Errorf("got %q", got)
	}
	t.Setenv("SAMPLIER_TEST_KEY", "value")
	if got := getenv("SAMPLIER_TEST_KEY", "fallback"); got != "value" {
		t.Errorf("got %q", got)
	}
}
