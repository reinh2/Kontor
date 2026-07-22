// Package bootstrap wires Stage 1 ports to concrete PostgreSQL and provider
// adapters. Keeping this outside cmd makes the same graph usable in tests.
package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/reinhlord/kontor/internal/agent"
	"github.com/reinhlord/kontor/internal/agentbudget"
	"github.com/reinhlord/kontor/internal/agenttools"
	"github.com/reinhlord/kontor/internal/agenttrace"
	"github.com/reinhlord/kontor/internal/app"
	"github.com/reinhlord/kontor/internal/confirmations"
	"github.com/reinhlord/kontor/internal/conversations"
	"github.com/reinhlord/kontor/internal/llm"
	"github.com/reinhlord/kontor/internal/platform/config"
	"github.com/reinhlord/kontor/internal/scheduling"
	"github.com/reinhlord/kontor/internal/tools"
)

const stage1OpenRouterAttempts = 3

type Components struct {
	Application   *app.Service
	Conversations *conversations.Store
	Trace         *agenttrace.Store
	Confirmations tools.ConfirmationStore
	Runner        *agent.Runner
}

func Build(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, logger *slog.Logger) (*Components, error) {
	model, modelName, err := modelAdapter(cfg)
	if err != nil {
		return nil, err
	}

	scheduleRepository := scheduling.NewPGXRepository(pool, cfg.Tenant.ID)
	scheduleBackend := scheduling.NewToolBackend(scheduleRepository)
	confirmationStore := confirmations.NewPostgreSQL(pool)
	gateway, err := tools.NewGateway(tools.Config{
		Backend: scheduleBackend, SlotSigningKey: []byte(cfg.SlotTokenSecret),
		Confirmations: confirmationStore,
	})
	if err != nil {
		return nil, fmt.Errorf("build tool gateway: %w", err)
	}
	conversationStore := conversations.NewStore(pool)
	traceStore := agenttrace.NewStore(pool, cfg.Tenant.ID)
	executor := agenttools.NewExecutor(
		pool, gateway, cfg.Tenant.ID, cfg.Agent.ToolMaxAttempts, cfg.Agent.ToolTimeout,
	)
	runner, err := agent.NewRunner(agent.Config{
		MaxIterations:          cfg.Agent.MaxIterations,
		TurnTimeout:            cfg.Agent.TurnTimeout,
		MaxOutputTokensPerCall: cfg.Agent.MaxOutputTokens,
		ConversationTokenLimit: int(cfg.Agent.ConversationTokenBudget),
	}, agent.Dependencies{
		Model: model, ToolExecutor: executor, Trace: traceStore,
		Budget: agentbudget.NewPostgreSQL(conversationStore, cfg.Tenant.ID),
		TokenEstimator: agent.ConservativeTokenEstimator{
			ProviderAttempts: providerAttemptLimit(cfg.LLM.Provider),
		},
	}, modelToolDefinitions())
	if err != nil {
		return nil, fmt.Errorf("build agent runner: %w", err)
	}
	application, err := app.New(app.Config{
		TenantID: cfg.Tenant.ID, TenantName: cfg.Tenant.Name, TenantTimezone: cfg.Tenant.Timezone,
		Provider: cfg.LLM.Provider, Model: modelName, TokenBudget: int(cfg.Agent.ConversationTokenBudget),
		TurnTimeout: cfg.Agent.TurnTimeout,
	}, pool, conversationStore, runner, traceStore, confirmationStore)
	if err != nil {
		return nil, err
	}
	logger.InfoContext(ctx, "stage 1 runtime ready",
		"tenant_id", cfg.Tenant.ID, "tenant_mode", "fixed", "llm_provider", cfg.LLM.Provider,
		"max_iterations", cfg.Agent.MaxIterations, "conversation_token_budget", cfg.Agent.ConversationTokenBudget)
	return &Components{
		Application: application, Conversations: conversationStore, Trace: traceStore,
		Confirmations: confirmationStore, Runner: runner,
	}, nil
}

func modelAdapter(cfg config.Config) (llm.Adapter, string, error) {
	if cfg.LLM.Provider == "fake" {
		adapter, err := llm.NewDemoAdapter(llm.DemoConfig{Timezone: cfg.Tenant.Timezone})
		return adapter, "kontor/demo-v1", err
	}
	endpoint := strings.TrimRight(cfg.LLM.OpenRouterURL, "/")
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint += "/chat/completions"
	}
	adapter, err := llm.NewOpenRouterAdapter(llm.OpenRouterConfig{
		APIKey: cfg.LLM.OpenRouterKey, Model: cfg.LLM.OpenRouterModel, Endpoint: endpoint,
		HTTPReferer: cfg.LLM.AppURL, AppTitle: cfg.LLM.AppTitle,
		Timeout: cfg.Agent.TurnTimeout, MaxAttempts: stage1OpenRouterAttempts,
	})
	return adapter, cfg.LLM.OpenRouterModel, err
}

func providerAttemptLimit(provider string) int {
	if provider == "openrouter" {
		return stage1OpenRouterAttempts
	}
	return 1
}

func modelToolDefinitions() []llm.ToolDefinition {
	definitions := tools.Definitions()
	result := make([]llm.ToolDefinition, len(definitions))
	for i, definition := range definitions {
		result[i] = llm.ToolDefinition{
			Name: definition.Name, Version: definition.Version, Description: definition.Description,
			Parameters: append([]byte(nil), definition.Parameters...),
		}
	}
	return result
}
