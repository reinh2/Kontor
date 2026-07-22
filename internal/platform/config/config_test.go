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

func TestLoadAllowsCustomDemoTenantID(t *testing.T) {
	const tenantID = "00000000-0000-4000-8000-000000000002"
	t.Setenv("FIXED_TENANT_ID", tenantID)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Tenant.ID != tenantID {
		t.Fatalf("tenant ID = %q, want %q", cfg.Tenant.ID, tenantID)
	}
}

func TestLoadDefaultsToMultiTenantDemo(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "fake")
	t.Setenv("MULTI_TENANT", "")
	t.Setenv("TENANT_HOST_SUFFIX", "")
	t.Setenv("TENANT_CHANNEL_ENCRYPTION_KEY", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Tenant.ID != DefaultTenantID {
		t.Fatalf("demo tenant ID = %q, want %q", cfg.Tenant.ID, DefaultTenantID)
	}
	if !cfg.Tenancy.Enabled || cfg.Tenancy.HostSuffix != "localhost" {
		t.Fatalf("tenancy = %+v, want enabled localhost tenancy", cfg.Tenancy)
	}
	if len(cfg.Tenancy.ChannelEncryptionKey) != 32 {
		t.Fatalf("encryption key length = %d, want 32", len(cfg.Tenancy.ChannelEncryptionKey))
	}
	if cfg.Operator.SessionTTL != 12*time.Hour {
		t.Fatalf("operator session TTL = %v, want 12h", cfg.Operator.SessionTTL)
	}
	if cfg.Demo.WidgetOrigin != "http://salon-nord.localhost:8080" {
		t.Fatalf("demo widget origin = %q", cfg.Demo.WidgetOrigin)
	}
	if cfg.ShutdownTimeout < cfg.Agent.TurnTimeout+5*time.Second {
		t.Fatalf("shutdown timeout %v cannot drain a %v turn", cfg.ShutdownTimeout, cfg.Agent.TurnTimeout)
	}
}

func TestLoadBuildsDemoWidgetOriginFromTenantHost(t *testing.T) {
	t.Setenv("FIXED_TENANT_SLUG", "studio-east")
	t.Setenv("TENANT_HOST_SUFFIX", "kontor.example")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Demo.WidgetOrigin != "http://studio-east.kontor.example:8080" {
		t.Fatalf("demo widget origin = %q", cfg.Demo.WidgetOrigin)
	}
}

func TestLoadRejectsMissingChannelKeyOutsideDemo(t *testing.T) {
	t.Setenv("DEMO_MODE", "false")
	t.Setenv("MULTI_TENANT", "true")
	t.Setenv("TENANT_CHANNEL_ENCRYPTION_KEY", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected missing tenant channel key to fail outside demo mode")
	}
}

func TestLoadRejectsInvalidTenantHostSuffix(t *testing.T) {
	for _, suffix := range []string{"not a host", "kontor.example:443", "https://kontor.example", "kontor..example", "-kontor.example"} {
		t.Run(suffix, func(t *testing.T) {
			t.Setenv("TENANT_HOST_SUFFIX", suffix)
			if _, err := Load(); err == nil {
				t.Fatal("expected an invalid tenant host suffix to fail")
			}
		})
	}
}

func TestLoadRejectsUnsafeOperatorSessionTTL(t *testing.T) {
	t.Setenv("OPERATOR_SESSION_TTL", "1m")
	if _, err := Load(); err == nil {
		t.Fatal("expected a session TTL below five minutes to fail")
	}
}

func TestLoadRejectsShutdownShorterThanTurnDrainWindow(t *testing.T) {
	t.Setenv("AGENT_TURN_TIMEOUT", "25s")
	t.Setenv("SHUTDOWN_TIMEOUT", "29s")
	if _, err := Load(); err == nil {
		t.Fatal("expected an unsafe shutdown timeout to fail")
	}
}

func TestLoadDefaultsPermissiveCORSAndBoundedRateLimit(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTP.AllowedOrigin != "*" {
		t.Fatalf("allowed origin = %q, want wildcard demo default", cfg.HTTP.AllowedOrigin)
	}
	if cfg.HTTP.RateLimitPerMinute < 1 || cfg.HTTP.RateLimitBurst < 1 {
		t.Fatalf("rate limit = %+v, want positive defaults", cfg.HTTP)
	}
}

func TestLoadRejectsNonPositiveRateLimit(t *testing.T) {
	t.Setenv("HTTP_RATE_LIMIT_PER_MINUTE", "0")
	if _, err := Load(); err == nil {
		t.Fatal("expected a non-positive rate limit to fail")
	}
}

func TestLoadRejectsEmptyAllowedOrigin(t *testing.T) {
	t.Setenv("HTTP_ALLOWED_ORIGIN", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected an explicitly empty allowed origin to fail")
	}
}

func TestLoadRejectsDisabledMultiTenantRuntime(t *testing.T) {
	t.Setenv("MULTI_TENANT", "false")
	if _, err := Load(); err == nil {
		t.Fatal("expected disabled multi-tenant runtime to fail after Stage 6")
	}
}

func TestLoadAllowsExplicitLegacyTenantBootstrapOutsideDemo(t *testing.T) {
	setValidLegacyBootstrap(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	bootstrap, err := LoadLegacyTenantBootstrap(cfg.DemoMode)
	if err != nil {
		t.Fatalf("LoadLegacyTenantBootstrap: %v", err)
	}
	if !bootstrap.Enabled || bootstrap.TenantSlug != "legacy-north" || bootstrap.WidgetOrigin != "https://north.example" || bootstrap.OwnerEmail != "owner@north.example" {
		t.Fatalf("bootstrap=%+v", bootstrap)
	}
}

func TestLoadRejectsUnsafeLegacyTenantBootstrapConfiguration(t *testing.T) {
	t.Run("fields without explicit gate", func(t *testing.T) {
		t.Setenv("STAGE6_BOOTSTRAP_OWNER_PASSWORD", "correct-horse-battery-staple")
		if _, err := Load(); err == nil {
			t.Fatal("expected bootstrap field without gate to fail")
		}
	})
	t.Run("incomplete enabled bootstrap", func(t *testing.T) {
		t.Setenv("DEMO_MODE", "false")
		t.Setenv("STAGE6_BOOTSTRAP_ENABLED", "true")
		t.Setenv("STAGE6_BOOTSTRAP_TENANT_ID", "00000000-0000-4000-8000-000000000009")
		if _, err := Load(); err == nil {
			t.Fatal("expected incomplete bootstrap to fail")
		}
	})
	t.Run("demo mode", func(t *testing.T) {
		setValidLegacyBootstrap(t)
		t.Setenv("DEMO_MODE", "true")
		if _, err := Load(); err == nil {
			t.Fatal("expected demo-mode bootstrap to fail")
		}
	})
}

func setValidLegacyBootstrap(t *testing.T) {
	t.Helper()
	t.Setenv("DEMO_MODE", "false")
	t.Setenv("SLOT_TOKEN_SECRET", "slot-token-secret-0123456789abcdef01")
	t.Setenv("TENANT_CHANNEL_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("STAGE6_BOOTSTRAP_ENABLED", "true")
	t.Setenv("STAGE6_BOOTSTRAP_TENANT_ID", "00000000-0000-4000-8000-000000000009")
	t.Setenv("STAGE6_BOOTSTRAP_TENANT_SLUG", "legacy-north")
	t.Setenv("STAGE6_BOOTSTRAP_WIDGET_ORIGIN", "https://north.example")
	t.Setenv("STAGE6_BOOTSTRAP_OWNER_EMAIL", "owner@north.example")
	t.Setenv("STAGE6_BOOTSTRAP_OWNER_DISPLAY_NAME", "North owner")
	t.Setenv("STAGE6_BOOTSTRAP_OWNER_PASSWORD", "correct-horse-battery-staple")
}

func TestLoadAdoptsLegacyTelegramOnlyThroughExplicitBootstrap(t *testing.T) {
	t.Run("complete bootstrap", func(t *testing.T) {
		setValidLegacyBootstrap(t)
		t.Setenv("TELEGRAM_BOT_TOKEN", "123456:legacy-bot-token")
		t.Setenv("TELEGRAM_WEBHOOK_SECRET", "legacy-webhook-secret")
		t.Setenv("TELEGRAM_API_BASE_URL", "https://telegram.internal")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if !cfg.Telegram.Enabled() || cfg.Telegram.APIBaseURL != "https://telegram.internal" {
			t.Fatalf("telegram config = %+v", cfg.Telegram)
		}
	})
	t.Run("missing bootstrap gate", func(t *testing.T) {
		t.Setenv("DEMO_MODE", "false")
		t.Setenv("TENANT_CHANNEL_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
		t.Setenv("TELEGRAM_BOT_TOKEN", "123456:legacy-bot-token")
		t.Setenv("TELEGRAM_WEBHOOK_SECRET", "legacy-webhook-secret")
		if _, err := Load(); err == nil {
			t.Fatal("expected legacy Telegram credentials without bootstrap gate to fail")
		}
	})
	t.Run("partial credentials", func(t *testing.T) {
		t.Setenv("TELEGRAM_BOT_TOKEN", "123456:legacy-bot-token")
		if _, err := Load(); err == nil {
			t.Fatal("expected partial legacy Telegram credentials to fail")
		}
	})
}

func TestLoadRejectsDemoSlotSecretOutsideDemo(t *testing.T) {
	t.Setenv("DEMO_MODE", "false")
	t.Setenv("TENANT_CHANNEL_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	// SLOT_TOKEN_SECRET intentionally left unset so it falls back to the demo
	// default, which must be rejected outside demo mode.
	if _, err := Load(); err == nil {
		t.Fatal("expected the demo SLOT_TOKEN_SECRET to be rejected outside demo mode")
	}
}

func TestLoadRejectsDemoChannelKeyOutsideDemo(t *testing.T) {
	t.Setenv("DEMO_MODE", "false")
	t.Setenv("SLOT_TOKEN_SECRET", "slot-token-secret-0123456789abcdef01")
	t.Setenv("TENANT_CHANNEL_ENCRYPTION_KEY", demoChannelEncryptionKey)
	if _, err := Load(); err == nil {
		t.Fatal("expected the demo TENANT_CHANNEL_ENCRYPTION_KEY to be rejected outside demo mode")
	}
}

func TestLoadMetricsDefaultsDisabled(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Metrics.Enabled {
		t.Fatal("metrics endpoint must be disabled by default")
	}
	if cfg.Metrics.Token != "" {
		t.Fatalf("metrics token = %q, want empty by default", cfg.Metrics.Token)
	}
}

func TestLoadRejectsShortMetricsToken(t *testing.T) {
	t.Setenv("METRICS_ENABLED", "true")
	t.Setenv("METRICS_TOKEN", "too-short")
	if _, err := Load(); err == nil {
		t.Fatal("expected a metrics token below 16 bytes to fail")
	}
}

func TestLoadAcceptsEnabledMetricsWithToken(t *testing.T) {
	t.Setenv("METRICS_ENABLED", "true")
	t.Setenv("METRICS_TOKEN", "metrics-scrape-token-0123456789")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Metrics.Enabled {
		t.Fatal("metrics endpoint should be enabled")
	}
	if cfg.Metrics.Token != "metrics-scrape-token-0123456789" {
		t.Fatalf("metrics token = %q", cfg.Metrics.Token)
	}
}
