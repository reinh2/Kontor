package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const DefaultTenantID = "00000000-0000-4000-8000-000000000001"

type Config struct {
	Environment string
	HTTPAddr    string
	DatabaseURL string
	DemoMode    bool

	Tenant   Tenant
	Agent    Agent
	LLM      LLM
	Telegram Telegram

	SlotTokenSecret string
	ShutdownTimeout time.Duration

	HTTP HTTP
}

type HTTP struct {
	// AllowedOrigin is the single origin the widget CORS policy accepts, or
	// "*" for the zero-key demo default. A wildcard cannot carry credentials;
	// the demo API authorizes via a bearer token, not cookies, so this is safe.
	AllowedOrigin string
	// RateLimitPerMinute and RateLimitBurst bound requests from one client IP
	// using a token bucket, protecting the bounded admission queue upstream.
	RateLimitPerMinute int
	RateLimitBurst     int
}

type Tenant struct {
	ID       string
	Slug     string
	Name     string
	Timezone string
	Currency string
}

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
	OpenRouterKey   string
	OpenRouterURL   string
	OpenRouterModel string
	AppURL          string
	AppTitle        string
}

// Telegram enables the webhook channel only when both the bot token and the
// webhook secret are present; the zero-key demo runs without either.
type Telegram struct {
	BotToken      string
	WebhookSecret string
	APIBaseURL    string
}

func (t Telegram) Enabled() bool { return t.BotToken != "" && t.WebhookSecret != "" }

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
			OpenRouterKey:   os.Getenv("OPENROUTER_API_KEY"),
			OpenRouterURL:   env("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1"),
			OpenRouterModel: os.Getenv("OPENROUTER_MODEL"),
			AppURL:          os.Getenv("OPENROUTER_APP_URL"),
			AppTitle:        env("OPENROUTER_APP_TITLE", "Kontor"),
		},
		Telegram: Telegram{
			BotToken:      os.Getenv("TELEGRAM_BOT_TOKEN"),
			WebhookSecret: os.Getenv("TELEGRAM_WEBHOOK_SECRET"),
			APIBaseURL:    env("TELEGRAM_API_BASE_URL", "https://api.telegram.org"),
		},
		SlotTokenSecret: env("SLOT_TOKEN_SECRET", "demo-only-change-me-32-bytes-minimum"),
		ShutdownTimeout: envDuration("SHUTDOWN_TIMEOUT", 35*time.Second),
		HTTP: HTTP{
			AllowedOrigin:      env("HTTP_ALLOWED_ORIGIN", "*"),
			RateLimitPerMinute: envInt("HTTP_RATE_LIMIT_PER_MINUTE", 60),
			RateLimitBurst:     envInt("HTTP_RATE_LIMIT_BURST", 20),
		},
	}

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if cfg.Tenant.ID != DefaultTenantID {
		return Config{}, fmt.Errorf("FIXED_TENANT_ID is fixed to %s in the single-tenant build", DefaultTenantID)
	}
	if cfg.Agent.MaxIterations < 1 || cfg.Agent.MaxIterations > 32 {
		return Config{}, fmt.Errorf("AGENT_MAX_ITERATIONS must be between 1 and 32")
	}
	if cfg.Agent.ToolMaxAttempts < 1 || cfg.Agent.ToolMaxAttempts > 16 {
		return Config{}, errors.New("AGENT_TOOL_MAX_ATTEMPTS must be between 1 and 16")
	}
	if cfg.Agent.ConversationTokenBudget < 1 || cfg.Agent.ConversationTokenBudget > 100_000 {
		return Config{}, errors.New("AGENT_CONVERSATION_TOKEN_BUDGET must be between 1 and 100000")
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
	if cfg.HTTP.RateLimitPerMinute < 1 {
		return Config{}, errors.New("HTTP_RATE_LIMIT_PER_MINUTE must be positive")
	}
	if cfg.HTTP.RateLimitBurst < 1 {
		return Config{}, errors.New("HTTP_RATE_LIMIT_BURST must be positive")
	}
	if cfg.HTTP.AllowedOrigin == "" {
		return Config{}, errors.New("HTTP_ALLOWED_ORIGIN must not be empty")
	}
	if (cfg.Telegram.BotToken == "") != (cfg.Telegram.WebhookSecret == "") {
		return Config{}, errors.New("TELEGRAM_BOT_TOKEN and TELEGRAM_WEBHOOK_SECRET must be set together")
	}
	if cfg.Telegram.Enabled() && len(cfg.Telegram.WebhookSecret) < 16 {
		return Config{}, errors.New("TELEGRAM_WEBHOOK_SECRET must contain at least 16 bytes")
	}
	if cfg.LLM.Provider != "fake" && cfg.LLM.Provider != "openrouter" {
		return Config{}, fmt.Errorf("unsupported LLM_PROVIDER %q", cfg.LLM.Provider)
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

func env(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
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
