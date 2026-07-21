// Package agenttrace persists the model/tool causal timeline used by the
// Stage 4 trace UI. Tool attempts are child records of one parent execution.
package agenttrace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/reinhlord/kontor/internal/agent"
	"github.com/reinhlord/kontor/internal/platform/ids"
)

type Store struct {
	pool     *pgxpool.Pool
	tenantID string
}

func NewStore(pool *pgxpool.Pool, tenantID string) *Store {
	return &Store{pool: pool, tenantID: tenantID}
}

func (s *Store) StartRun(ctx context.Context, runID, conversationID, triggerMessageID, provider, model string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO agent_runs
			(tenant_id,id,conversation_id,trigger_message_id,status,provider,model)
		VALUES($1,$2,$3,$4,'running',$5,$6)`,
		s.tenantID, runID, conversationID, triggerMessageID, provider, model)
	if err != nil {
		return fmt.Errorf("start agent run: %w", err)
	}
	return nil
}

func (s *Store) FinishRun(ctx context.Context, runID, status, errorCode, errorMessage string, startedAt time.Time) error {
	if status == "" {
		status = "completed"
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE agent_runs
		SET status=$3,error_code=NULLIF($4,''),error_message=NULLIF($5,''),
		    duration_ms=$6,finished_at=now()
		WHERE tenant_id=$1 AND id=$2`,
		s.tenantID, runID, status, errorCode, errorMessage, nonNegativeMS(time.Since(startedAt)))
	if err != nil {
		return fmt.Errorf("finish agent run: %w", err)
	}
	return nil
}

func (s *Store) RecordModelCall(ctx context.Context, trace agent.ModelCallTrace) error {
	payload, _ := json.Marshal(map[string]any{
		"returned_tool_call_count": trace.ReturnedToolCallCount,
		"reserved_tokens":          trace.ReservedTokens,
		"charged_tokens":           trace.ChargedTokens,
	})
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin model trace: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, `
		INSERT INTO agent_iterations
			(tenant_id,id,agent_run_id,iteration_no,finish_reason,prompt_tokens,
			 completion_tokens,duration_ms,model_response,created_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10)`,
		s.tenantID, ids.New(), trace.RunID, trace.Iteration, trace.FinishReason,
		trace.Usage.InputTokens, trace.Usage.OutputTokens,
		nonNegativeMS(trace.FinishedAt.Sub(trace.StartedAt)), string(payload), trace.StartedAt,
	); err != nil {
		return fmt.Errorf("insert model trace: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE agent_runs
		SET model=CASE WHEN $3='' THEN model ELSE $3 END,
		    prompt_tokens=prompt_tokens+$4,
		    completion_tokens=completion_tokens+$5
		WHERE tenant_id=$1 AND id=$2`,
		s.tenantID, trace.RunID, trace.Model, trace.Usage.InputTokens, trace.Usage.OutputTokens,
	); err != nil {
		return fmt.Errorf("update run usage: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit model trace: %w", err)
	}
	return nil
}

func (s *Store) RecordToolExecutionStarted(ctx context.Context, trace agent.ToolExecutionStartedTrace) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO tool_executions
			(tenant_id,id,agent_run_id,agent_iteration_id,tool_call_id,tool_name,
			 contract_version,arguments_json,status,call_index,call_count,started_at)
		SELECT $1,$2,$3,i.id,$4,$5,$6,$7::jsonb,'running',$8,$9,$10
		FROM agent_iterations i
		WHERE i.tenant_id=$1 AND i.agent_run_id=$3 AND i.iteration_no=$11`,
		s.tenantID, ids.New(), trace.RunID, trace.CallID, trace.ToolName, trace.ContractVersion,
		normalizeJSON(trace.Arguments), trace.CallIndex, trace.CallCount, trace.StartedAt, trace.Iteration)
	if err != nil {
		return fmt.Errorf("insert tool parent trace: %w", err)
	}
	return nil
}

func (s *Store) RecordToolAttempt(ctx context.Context, trace agent.ToolAttemptTrace) error {
	status := "succeeded"
	if trace.Status != agent.ToolStatusSuccess {
		status = "failed"
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO tool_execution_attempts
			(tenant_id,id,tool_execution_id,attempt_no,status,result_json,duration_ms,started_at,finished_at)
		SELECT $1,$2,e.id,$3,$4,$5::jsonb,$6,$7,$8
		FROM tool_executions e
		WHERE e.tenant_id=$1 AND e.agent_run_id=$9 AND e.tool_call_id=$10`,
		s.tenantID, ids.New(), trace.AttemptNo, status, normalizeJSON(trace.Detail),
		nonNegativeMS(trace.FinishedAt.Sub(trace.StartedAt)), trace.StartedAt, trace.FinishedAt,
		trace.RunID, trace.CallID)
	if err != nil {
		return fmt.Errorf("insert tool attempt trace: %w", err)
	}
	return nil
}

func (s *Store) RecordToolExecutionCompleted(ctx context.Context, trace agent.ToolExecutionCompletedTrace) error {
	status := string(trace.Status)
	switch trace.Status {
	case agent.ToolStatusSucceeded, agent.ToolStatusFailed, agent.ToolStatusRefused, agent.ToolStatusConfirmationRequired:
	default:
		status = "failed"
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE tool_executions
		SET result_json=$4::jsonb,status=$5,duration_ms=$6,finished_at=$7
		WHERE tenant_id=$1 AND agent_run_id=$2 AND tool_call_id=$3`,
		s.tenantID, trace.RunID, trace.CallID, normalizeJSON(trace.Result), status,
		nonNegativeMS(trace.FinishedAt.Sub(trace.StartedAt)), trace.FinishedAt)
	if err != nil {
		return fmt.Errorf("complete tool parent trace: %w", err)
	}
	return nil
}

type RunTrace struct {
	ID               string      `json:"id"`
	ConversationID   string      `json:"conversation_id"`
	Status           string      `json:"status"`
	Provider         string      `json:"provider"`
	Model            string      `json:"model"`
	PromptTokens     int         `json:"prompt_tokens"`
	CompletionTokens int         `json:"completion_tokens"`
	DurationMS       *int        `json:"duration_ms"`
	StartedAt        time.Time   `json:"started_at"`
	FinishedAt       *time.Time  `json:"finished_at"`
	Tools            []ToolTrace `json:"tools"`
}

type ToolTrace struct {
	ID         string          `json:"id"`
	Iteration  int             `json:"iteration"`
	CallIndex  int             `json:"call_index"`
	CallCount  int             `json:"call_count"`
	CallID     string          `json:"call_id"`
	Name       string          `json:"name"`
	Arguments  json.RawMessage `json:"arguments"`
	Result     json.RawMessage `json:"result,omitempty"`
	Status     string          `json:"status"`
	DurationMS *int            `json:"duration_ms"`
	Attempts   []AttemptTrace  `json:"attempts"`
}

type AttemptTrace struct {
	AttemptNo  int             `json:"attempt_no"`
	Status     string          `json:"status"`
	Result     json.RawMessage `json:"result,omitempty"`
	DurationMS *int            `json:"duration_ms"`
}

func (s *Store) GetRun(ctx context.Context, runID string) (RunTrace, error) {
	var run RunTrace
	err := s.pool.QueryRow(ctx, `
		SELECT id::text,conversation_id::text,status,provider,model,prompt_tokens,
		       completion_tokens,duration_ms,started_at,finished_at
		FROM agent_runs WHERE tenant_id=$1 AND id=$2`, s.tenantID, runID).
		Scan(&run.ID, &run.ConversationID, &run.Status, &run.Provider, &run.Model,
			&run.PromptTokens, &run.CompletionTokens, &run.DurationMS, &run.StartedAt, &run.FinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return RunTrace{}, pgx.ErrNoRows
	}
	if err != nil {
		return RunTrace{}, fmt.Errorf("get run: %w", err)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT e.id::text,i.iteration_no,e.call_index,e.call_count,e.tool_call_id,e.tool_name,
		       e.arguments_json,e.result_json,e.status,e.duration_ms
		FROM tool_executions e
		JOIN agent_iterations i ON i.tenant_id=e.tenant_id AND i.id=e.agent_iteration_id
		WHERE e.tenant_id=$1 AND e.agent_run_id=$2
		ORDER BY i.iteration_no,e.call_index`, s.tenantID, runID)
	if err != nil {
		return RunTrace{}, fmt.Errorf("list tool traces: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var tool ToolTrace
		var result []byte
		if err := rows.Scan(&tool.ID, &tool.Iteration, &tool.CallIndex, &tool.CallCount,
			&tool.CallID, &tool.Name, &tool.Arguments, &result, &tool.Status, &tool.DurationMS); err != nil {
			return RunTrace{}, fmt.Errorf("scan tool trace: %w", err)
		}
		tool.Result = result
		tool.Attempts, err = s.getAttempts(ctx, tool.ID)
		if err != nil {
			return RunTrace{}, err
		}
		run.Tools = append(run.Tools, tool)
	}
	if err := rows.Err(); err != nil {
		return RunTrace{}, fmt.Errorf("tool trace rows: %w", err)
	}
	if run.Tools == nil {
		run.Tools = []ToolTrace{}
	}
	return run, nil
}

func (s *Store) getAttempts(ctx context.Context, executionID string) ([]AttemptTrace, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT attempt_no,status,result_json,duration_ms
		FROM tool_execution_attempts
		WHERE tenant_id=$1 AND tool_execution_id=$2
		ORDER BY attempt_no`, s.tenantID, executionID)
	if err != nil {
		return nil, fmt.Errorf("list tool attempts: %w", err)
	}
	defer rows.Close()
	var result []AttemptTrace
	for rows.Next() {
		var item AttemptTrace
		var payload []byte
		if err := rows.Scan(&item.AttemptNo, &item.Status, &payload, &item.DurationMS); err != nil {
			return nil, fmt.Errorf("scan tool attempt: %w", err)
		}
		item.Result = payload
		result = append(result, item)
	}
	if result == nil {
		result = []AttemptTrace{}
	}
	return result, rows.Err()
}

func normalizeJSON(value json.RawMessage) string {
	if len(value) == 0 || !json.Valid(value) {
		return `{}`
	}
	return string(value)
}

func nonNegativeMS(duration time.Duration) int {
	if duration < 0 {
		return 0
	}
	return int(duration.Milliseconds())
}

var _ agent.TraceSink = (*Store)(nil)
