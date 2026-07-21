package config

import "testing"

func TestLoadRejectsOpenRouterWithoutCredentials(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "openrouter")
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("OPENROUTER_MODEL", "")

	if _, err := Load(); err == nil {
		t.Fatal("expected missing OpenRouter credentials to fail")
	}
}

func TestLoadDefaultsToSingleFixedTenant(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "fake")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Tenant.ID != DefaultTenantID {
		t.Fatalf("tenant ID = %q, want %q", cfg.Tenant.ID, DefaultTenantID)
	}
}
