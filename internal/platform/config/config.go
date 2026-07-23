package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// DefaultTenantID identifies the repeatable demo tenant. It is not a runtime
// tenant selector: Stage 6 resolves live tenants from sessions, hosts, and
// webhook paths.
const DefaultTenantID = "00000000-0000-4000-8000-000000000001"

const demoChannelEncryptionKey = "demo-only-tenant-channel-key-32!"

// demoSlotTokenSecret is the intentionally weak default used only for the
// zero-key demo. Startup rejects it outside demo mode so a production deploy
// cannot silently sign slot tokens with a public value.
const demoSlotTokenSecret = "demo-only-change-me-32-bytes-minimum"

type Config struct {
	Environment string
	HTTPAddr    string
	DatabaseURL string
	DemoMode    bool

	// Tenant is used only to seed the deterministic demo business. Production
	// traffic never reads it to select a tenant.
	Tenant   Tenant
	Tenancy  Tenancy
	Demo     Demo
	Telegram Telegram
	Agent    Agent
	LLM      LLM
	Operator Operator

	SlotTokenSecret string
	ShutdownTimeout time.Duration

	HTTP    HTTP
	Metrics Metrics
}

// Metrics controls the Prometheus exposition endpoint. It is disabled by
// default; the endpoint is only mounted when Enabled is true. When Token is
// set, scrapers must present it as a bearer token, since the endpoint sits
// outside the operator session and widget CORS edges.
type Metrics struct {
	Enabled bool
	Token   string
}

type HTTP struct {
	// AllowedOrigin remains the Stage 1-5 single-tenant fallback. Stage 6
	// widget traffic is checked against tenant_channels.widget_origin instead.
	AllowedOrigin      string
	RateLimitPerMinute int
	RateLimitBurst     int
	// TrustForwardedFor lets the per-IP rate limiter key on the first
	// X-Forwarded-For hop. It defaults to true because the shipped topology puts
	// the service behind the bundled nginx proxy. Set it to false when the
	// service can be reached directly, so a client cannot spoof the header to
	// evade the per-IP limit; the limiter then keys on the socket address.
	TrustForwardedFor bool
}

type Tenant struct {
	ID       string
	Slug     string
	Name     string
	Timezone string
	Currency string
}

// Tenancy holds platform-wide, non-business configuration. Each business owns
// its own channel settings in PostgreSQL; only the host suffix and encryption
// key remain deployment secrets.
type Tenancy struct {
	Enabled              bool
	HostSuffix           string
	ChannelEncryptionKey []byte
}

type Demo struct {
	OwnerEmail    string
	OwnerPassword string
	WidgetOrigin  string
}

// Telegram holds the former deployment-wide settings only long enough to
// adopt an explicitly named Stage 1-5 tenant. After adoption, bot credentials
// are tenant-owned; APIBaseURL remains a deployment-wide sender endpoint.
type Telegram struct {
	BotToken      string
	WebhookSecret string
	APIBaseURL    string
}

func (t Telegram) Enabled() bool { return t.BotToken != "" && t.WebhookSecret != "" }

type Agent struct {
	MaxIterations           int
	TurnTimeout             time.Duration
	ToolTimeout             time.Duration
	ToolMaxAttempts         int
	ConversationTokenBudget int64
	MaxOutputTokens         int
}

type LLM struct {
	Provider        string
	OpenAIKey       string
	OpenAIURL       string
	OpenAIModel     string
	OpenRouterKey   string
	OpenRouterURL   string
	OpenRouterModel string
	AppURL          string
	AppTitle        string
}

// Operator replaces the Stage 5 shared admin token with durable, database
// backed identities and opaque sessions.
type Operator struct {
	SessionTTL time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		Environment: env("APP_ENV", "development"),
		HTTPAddr:    env("HTTP_ADDR", ":8080"),
		DatabaseURL: env("DATABASE_URL", "postgres://kontor:kontor@localhost:5432/kontor?sslmode=disable"),
		DemoMode:    envBool("DEMO_MODE", true),
		Tenant: Tenant{
			ID:       env("FIXED_TENANT_ID", DefaultTenantID),
			Slug:     env("FIXED_TENANT_SLUG", "salon-nord"),
			Name:     env("FIXED_TENANT_NAME", "Salon Nord"),
			Timezone: env("FIXED_TENANT_TIMEZONE", "Europe/Berlin"),
			Currency: env("FIXED_TENANT_CURRENCY", "EUR"),
		},
		Tenancy: Tenancy{
			Enabled:    envBool("MULTI_TENANT", true),
			HostSuffix: strings.ToLower(strings.Trim(env("TENANT_HOST_SUFFIX", "localhost"), ".")),
		},
		Demo: Demo{
			OwnerEmail:    env("DEMO_OWNER_EMAIL", "owner@salon-nord.test"),
			OwnerPassword: env("DEMO_OWNER_PASSWORD", "demo-operator-password"),
		},
		Telegram: Telegram{
			BotToken:      os.Getenv("TELEGRAM_BOT_TOKEN"),
			WebhookSecret: os.Getenv("TELEGRAM_WEBHOOK_SECRET"),
			APIBaseURL:    env("TELEGRAM_API_BASE_URL", "https://api.telegram.org"),
		},
		Agent: Agent{
			MaxIterations:           envInt("AGENT_MAX_ITERATIONS", 8),
			TurnTimeout:             envDuration("AGENT_TURN_TIMEOUT", 25*time.Second),
			ToolTimeout:             envDuration("AGENT_TOOL_TIMEOUT", 5*time.Second),
			ToolMaxAttempts:         envInt("AGENT_TOOL_MAX_ATTEMPTS", 3),
			ConversationTokenBudget: int64(envInt("AGENT_CONVERSATION_TOKEN_BUDGET", 50_000)),
			MaxOutputTokens:         envInt("AGENT_MAX_OUTPUT_TOKENS", 800),
		},
		LLM: LLM{
			Provider:        strings.ToLower(env("LLM_PROVIDER", "fake")),
			OpenAIKey:       os.Getenv("OPENAI_API_KEY"),
			OpenAIURL:       env("OPENAI_BASE_URL", "https://api.openai.com/v1"),
			OpenAIModel:     os.Getenv("OPENAI_MODEL"),
			OpenRouterKey:   os.Getenv("OPENROUTER_API_KEY"),
			OpenRouterURL:   env("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1"),
			OpenRouterModel: os.Getenv("OPENROUTER_MODEL"),
			AppURL:          os.Getenv("OPENROUTER_APP_URL"),
			AppTitle:        env("OPENROUTER_APP_TITLE", "Kontor"),
		},
		Operator:        Operator{SessionTTL: envDuration("OPERATOR_SESSION_TTL", 12*time.Hour)},
		SlotTokenSecret: env("SLOT_TOKEN_SECRET", demoSlotTokenSecret),
		ShutdownTimeout: envDuration("SHUTDOWN_TIMEOUT", 35*time.Second),
		HTTP: HTTP{
			AllowedOrigin:      env("HTTP_ALLOWED_ORIGIN", "*"),
			RateLimitPerMinute: envInt("HTTP_RATE_LIMIT_PER_MINUTE", 60),
			RateLimitBurst:     envInt("HTTP_RATE_LIMIT_BURST", 20),
			TrustForwardedFor:  envBool("HTTP_TRUST_FORWARDED_FOR", true),
		},
		Metrics: Metrics{
			Enabled: envBool("METRICS_ENABLED", false),
			Token:   os.Getenv("METRICS_TOKEN"),
		},
	}
	legacyBootstrap, err := LoadLegacyTenantBootstrap(cfg.DemoMode)
	if err != nil {
		return Config{}, err
	}
	if (cfg.Telegram.BotToken == "") != (cfg.Telegram.WebhookSecret == "") {
		return Config{}, errors.New("TELEGRAM_BOT_TOKEN and TELEGRAM_WEBHOOK_SECRET must be set together")
	}
	if cfg.Telegram.Enabled() {
		if !legacyBootstrap.Enabled {
			return Config{}, errors.New("legacy Telegram credentials require STAGE6_BOOTSTRAP_ENABLED=true")
		}
		if len(cfg.Telegram.WebhookSecret) < 16 {
			return Config{}, errors.New("TELEGRAM_WEBHOOK_SECRET must contain at least 16 bytes")
		}
	}
	cfg.Demo.WidgetOrigin = env("DEMO_WIDGET_ORIGIN", fmt.Sprintf("http://%s.%s:8080", cfg.Tenant.Slug, cfg.Tenancy.HostSuffix))
	key := os.Getenv("TENANT_CHANNEL_ENCRYPTION_KEY")
	if key == "" && cfg.DemoMode {
		key = demoChannelEncryptionKey
	}
	cfg.Tenancy.ChannelEncryptionKey = []byte(key)

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if cfg.Tenant.ID == "" || cfg.Tenant.Slug == "" || cfg.Tenant.Name == "" {
		return Config{}, errors.New("demo tenant ID, slug, and name must not be empty")
	}
	if _, err := time.LoadLocation(cfg.Tenant.Timezone); err != nil {
		return Config{}, fmt.Errorf("FIXED_TENANT_TIMEZONE is invalid: %w", err)
	}
	if !cfg.Tenancy.Enabled {
		return Config{}, errors.New("MULTI_TENANT=false is not supported after Stage 6")
	}
	if !validDNSSuffix(cfg.Tenancy.HostSuffix) {
		return Config{}, errors.New("TENANT_HOST_SUFFIX must be a DNS suffix")
	}
	if len(cfg.Tenancy.ChannelEncryptionKey) != 32 {
		return Config{}, errors.New("TENANT_CHANNEL_ENCRYPTION_KEY must contain exactly 32 bytes")
	}
	if cfg.Agent.MaxIterations < 1 || cfg.Agent.MaxIterations > 32 {
		return Config{}, fmt.Errorf("AGENT_MAX_ITERATIONS must be between 1 and 32")
	}
	if cfg.Agent.ToolMaxAttempts < 1 || cfg.Agent.ToolMaxAttempts > 16 {
		return Config{}, errors.New("AGENT_TOOL_MAX_ATTEMPTS must be between 1 and 16")
	}
	// The ceiling only guards against a typo turning the per-conversation cap
	// into an unbounded one. A real-model booking spends ~65 000 tokens across
	// two turns, and the conservative estimator reserves ~3x a turn's actual
	// use on top, so 100 000 could not hold one completed booking.
	if cfg.Agent.ConversationTokenBudget < 1 || cfg.Agent.ConversationTokenBudget > 2_000_000 {
		return Config{}, errors.New("AGENT_CONVERSATION_TOKEN_BUDGET must be between 1 and 2000000")
	}
	if cfg.Agent.MaxOutputTokens < 1 {
		return Config{}, errors.New("AGENT_MAX_OUTPUT_TOKENS must be positive")
	}
	if cfg.Agent.TurnTimeout <= 0 {
		return Config{}, errors.New("AGENT_TURN_TIMEOUT must be positive")
	}
	if cfg.ShutdownTimeout < cfg.Agent.TurnTimeout+5*time.Second {
		return Config{}, errors.New("SHUTDOWN_TIMEOUT must be at least AGENT_TURN_TIMEOUT plus 5s")
	}
	if len(cfg.SlotTokenSecret) < 32 {
		return Config{}, errors.New("SLOT_TOKEN_SECRET must contain at least 32 bytes")
	}
	// A production APP_ENV must never run in demo mode. Demo mode seeds the
	// fixed demo tenant/owner and, more importantly, skips the fail-closed
	// demo-secret checks below. Because DEMO_MODE defaults to true, a production
	// deploy that forgets to set DEMO_MODE=false would otherwise silently ship
	// the public demo secrets; force an explicit choice instead.
	if cfg.DemoMode && isProductionEnvironment(cfg.Environment) {
		return Config{}, errors.New("DEMO_MODE must be false when APP_ENV is a production environment")
	}
	// Fail closed on the public demo secrets outside demo mode. These values
	// ship in compose.yaml and .env.example, so accepting them in a real
	// deployment would sign slot tokens and encrypt tenant channel secrets with
	// a globally known key.
	if !cfg.DemoMode {
		if cfg.SlotTokenSecret == demoSlotTokenSecret {
			return Config{}, errors.New("SLOT_TOKEN_SECRET is the demo default; set a real secret when DEMO_MODE=false")
		}
		if string(cfg.Tenancy.ChannelEncryptionKey) == demoChannelEncryptionKey {
			return Config{}, errors.New("TENANT_CHANNEL_ENCRYPTION_KEY is the demo default; set a real 32-byte key when DEMO_MODE=false")
		}
	}
	if cfg.Metrics.Token != "" && len(cfg.Metrics.Token) < 16 {
		return Config{}, errors.New("METRICS_TOKEN must contain at least 16 bytes when set")
	}
	if cfg.Operator.SessionTTL < 5*time.Minute || cfg.Operator.SessionTTL > 30*24*time.Hour {
		return Config{}, errors.New("OPERATOR_SESSION_TTL must be between 5 minutes and 30 days")
	}
	if cfg.HTTP.RateLimitPerMinute < 1 {
		return Config{}, errors.New("HTTP_RATE_LIMIT_PER_MINUTE must be positive")
	}
	if cfg.HTTP.RateLimitBurst < 1 {
		return Config{}, errors.New("HTTP_RATE_LIMIT_BURST must be positive")
	}
	if cfg.HTTP.AllowedOrigin == "" {
		return Config{}, errors.New("HTTP_ALLOWED_ORIGIN must not be empty")
	}
	if cfg.LLM.Provider != "fake" && cfg.LLM.Provider != "openai" && cfg.LLM.Provider != "openrouter" {
		return Config{}, fmt.Errorf("unsupported LLM_PROVIDER %q", cfg.LLM.Provider)
	}
	if cfg.LLM.Provider == "openai" {
		if cfg.LLM.OpenAIKey == "" {
			return Config{}, errors.New("OPENAI_API_KEY is required when LLM_PROVIDER=openai")
		}
		if cfg.LLM.OpenAIModel == "" {
			return Config{}, errors.New("OPENAI_MODEL is required when LLM_PROVIDER=openai")
		}
	}
	if cfg.LLM.Provider == "openrouter" {
		if cfg.LLM.OpenRouterKey == "" {
			return Config{}, errors.New("OPENROUTER_API_KEY is required when LLM_PROVIDER=openrouter")
		}
		if cfg.LLM.OpenRouterModel == "" {
			return Config{}, errors.New("OPENROUTER_MODEL is required when LLM_PROVIDER=openrouter")
		}
	}
	return cfg, nil
}

// isProductionEnvironment reports whether APP_ENV names a production-like
// environment where demo defaults must never apply.
func isProductionEnvironment(environment string) bool {
	switch strings.ToLower(strings.TrimSpace(environment)) {
	case "production", "prod":
		return true
	default:
		return false
	}
}

func validDNSSuffix(value string) bool {
	if len(value) == 0 || len(value) > 253 {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for index := 0; index < len(label); index++ {
			character := label[index]
			if !((character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-') {
				return false
			}
		}
	}
	return true
}

func env(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		if key != "TENANT_HOST_SUFFIX" || value != "" {
			return value
		}
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}

func envBool(key string, fallback bool) bool {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}
	b, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return b
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return d
}
