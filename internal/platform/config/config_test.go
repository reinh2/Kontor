package config

import (
	"testing"
	"time"
)

func TestLoadRejectsOpenRouterWithoutCredentials(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "openrouter")
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("OPENROUTER_MODEL", "")

	if _, err := Load(); err == nil {
		t.Fatal("expected missing OpenRouter credentials to fail")
	}
}

func TestLoadRejectsChangingFixedTenantID(t *testing.T) {
	t.Setenv("FIXED_TENANT_ID", "00000000-0000-4000-8000-000000000002")
	if _, err := Load(); err == nil {
		t.Fatal("expected a changed fixed tenant ID to fail")
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
	if cfg.ShutdownTimeout < cfg.Agent.TurnTimeout+5*time.Second {
		t.Fatalf("shutdown timeout %v cannot drain a %v turn", cfg.ShutdownTimeout, cfg.Agent.TurnTimeout)
	}
}

func TestLoadRejectsShutdownShorterThanTurnDrainWindow(t *testing.T) {
	t.Setenv("AGENT_TURN_TIMEOUT", "25s")
	t.Setenv("SHUTDOWN_TIMEOUT", "29s")
	if _, err := Load(); err == nil {
		t.Fatal("expected an unsafe shutdown timeout to fail")
	}
}
