package demohttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/reinhlord/kontor/internal/agenttrace"
	"github.com/reinhlord/kontor/internal/app"
	"github.com/reinhlord/kontor/internal/conversations"
	"github.com/reinhlord/kontor/internal/tenants"
)

// tenantRuntime is implemented by bootstrap.Runtime. The request context is
// already host-resolved before this boundary is reached.
type tenantRuntime interface {
	ApplicationFor(context.Context, string) (*app.Service, *conversations.Store, *agenttrace.Store, error)
}

// NewMultiTenant serves the familiar widget endpoints but resolves their
// application, conversations, and trace store from the immutable tenant in
// request context. Existing handler code therefore cannot accidentally retain
// the prior request's tenant under concurrent traffic.
func NewMultiTenant(runtime tenantRuntime, pool readinessChecker, logger *slog.Logger) (http.Handler, error) {
	if runtime == nil || pool == nil {
		return nil, errors.New("demo HTTP: tenant runtime and pool are required")
	}
	proxy := tenantApplicationProxy{runtime: runtime}
	return New(proxy, tenantTraceProxy{runtime: runtime}, pool, logger), nil
}

type tenantApplicationProxy struct{ runtime tenantRuntime }

func (p tenantApplicationProxy) components(ctx context.Context) (*app.Service, *conversations.Store, *agenttrace.Store, error) {
	tenant, ok := tenants.FromContext(ctx)
	if !ok {
		return nil, nil, nil, errors.New("demo HTTP: host tenant was not resolved")
	}
	return p.runtime.ApplicationFor(ctx, tenant.ID)
}

func (p tenantApplicationProxy) CreateConversation(ctx context.Context, profile conversations.Profile) (conversations.Conversation, error) {
	application, _, _, err := p.components(ctx)
	if err != nil { return conversations.Conversation{}, err }
	return application.CreateConversation(ctx, profile)
}

func (p tenantApplicationProxy) VerifyConversationCapability(ctx context.Context, conversationID, token string) error {
	application, _, _, err := p.components(ctx)
	if err != nil { return err }
	return application.VerifyConversationCapability(ctx, conversationID, token)
}

func (p tenantApplicationProxy) SendMessage(ctx context.Context, conversationID, text, clientMessageID string) (app.TurnResult, error) {
	application, _, _, err := p.components(ctx)
	if err != nil { return app.TurnResult{}, err }
	return application.SendMessage(ctx, conversationID, text, clientMessageID)
}

func (p tenantApplicationProxy) ConversationEvents(ctx context.Context, conversationID string, afterID int64, limit int) ([]conversations.Event, error) {
	application, _, _, err := p.components(ctx)
	if err != nil { return nil, err }
	return application.ConversationEvents(ctx, conversationID, afterID, limit)
}

type tenantTraceProxy struct{ runtime tenantRuntime }

func (p tenantTraceProxy) GetRun(ctx context.Context, runID string) (agenttrace.RunTrace, error) {
	tenant, ok := tenants.FromContext(ctx)
	if !ok { return agenttrace.RunTrace{}, errors.New("demo HTTP: host tenant was not resolved") }
	_, _, trace, err := p.runtime.ApplicationFor(ctx, tenant.ID)
	if err != nil { return agenttrace.RunTrace{}, err }
	return trace.GetRun(ctx, runID)
}

var _ applicationService = tenantApplicationProxy{}
var _ traceReader = tenantTraceProxy{}
