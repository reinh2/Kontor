// Package onboardinghttp exposes the Stage 6 tenant provisioning and operator
// identity endpoints. Public endpoints are intentionally limited to tenant
// creation and login; every existing-tenant mutation is session- and role-bound.
package onboardinghttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/reinhlord/kontor/internal/identity"
	"github.com/reinhlord/kontor/internal/tenants"
)

type tenantStore interface {
	Provision(context.Context, tenants.ProvisionInput) (tenants.Tenant, error)
	CreateService(context.Context, string, tenants.ServiceInput) (tenants.Service, error)
	CreateStaff(context.Context, string, tenants.StaffInput) (tenants.Staff, error)
	AddAvailability(context.Context, string, string, []tenants.AvailabilityRuleInput) error
	UpdateChannels(context.Context, string, tenants.ChannelConfig) error
	ChannelConfig(context.Context, string) (tenants.ChannelConfig, error)
}

type identityStore interface {
	Authenticate(context.Context, string, string, string) (identity.LoginResult, error)
	RevokeSession(context.Context, string) error
	CreateOperator(context.Context, identity.CreateOperatorInput) (identity.Operator, error)
	ValidateSession(context.Context, string) (identity.Principal, error)
}

type Handler struct {
	tenants    tenantStore
	identities identityStore
}

func New(tenantStore tenantStore, identities identityStore) (http.Handler, error) {
	if tenantStore == nil || identities == nil {
		return nil, errors.New("onboarding HTTP: tenant and identity stores are required")
	}
	h := &Handler{tenants: tenantStore, identities: identities}
	public := http.NewServeMux()
	public.HandleFunc("POST /api/v1/tenants", h.provision)
	public.HandleFunc("POST /api/v1/operator/login", h.login)

	protected := http.NewServeMux()
	protected.HandleFunc("POST /api/v1/operator/logout", h.logout)
	owners := http.NewServeMux()
	owners.HandleFunc("GET /api/v1/operator/channels", h.getChannels)
	owners.HandleFunc("PUT /api/v1/operator/channels", h.updateChannels)
	owners.HandleFunc("POST /api/v1/operator/operators", h.createOperator)
	owners.HandleFunc("POST /api/v1/operator/catalog/services", h.createService)
	owners.HandleFunc("POST /api/v1/operator/staff", h.createStaff)
	owners.HandleFunc("POST /api/v1/operator/staff/{staffID}/availability", h.addAvailability)
	protected.Handle("/api/v1/operator/", identity.RequireRole(identity.RoleOwner, owners))

	public.Handle("/api/v1/operator/", identity.Authenticate(identities, protected))
	return noStore(public), nil
}

func (h *Handler) provision(w http.ResponseWriter, r *http.Request) {
	var input tenants.ProvisionInput
	if !decodeJSON(w, r, &input) {
		return
	}
	created, err := h.tenants.Provision(r.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, tenants.ErrConflict):
			writeProblem(w, http.StatusConflict, "tenant already exists", "Choose a different tenant slug")
		case errors.Is(err, tenants.ErrInvalidInput), errors.Is(err, identity.ErrInvalidPassword):
			writeProblem(w, http.StatusBadRequest, "invalid tenant configuration", "Provide a complete tenant, owner, catalogue, staff, schedule, and channel configuration")
		default:
			writeProblem(w, http.StatusInternalServerError, "tenant provisioning failed", "The tenant could not be provisioned")
		}
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		TenantSlug string `json:"tenant_slug"`
		Email      string `json:"email"`
		Password   string `json:"password"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	result, err := h.identities.Authenticate(r.Context(), input.TenantSlug, input.Email, input.Password)
	if err != nil {
		if errors.Is(err, identity.ErrInvalidCredentials) || errors.Is(err, identity.ErrInvalidOperator) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="kontor-operator"`)
			writeProblem(w, http.StatusUnauthorized, "invalid credentials", "The tenant, email, or password was not accepted")
			return
		}
		writeProblem(w, http.StatusInternalServerError, "login failed", "The operator session could not be started")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": result.Token,
		"token_type":   "Bearer",
		"expires_at":   result.Principal.ExpiresAt,
		"session":      sessionResponse(result.Principal),
	})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	token, ok := bearerToken(r.Header.Get("Authorization"))
	if !ok {
		writeProblem(w, http.StatusUnauthorized, "unauthorized", "A valid operator session is required")
		return
	}
	if err := h.identities.RevokeSession(r.Context(), token); err != nil && !errors.Is(err, identity.ErrSessionInvalid) {
		writeProblem(w, http.StatusInternalServerError, "logout failed", "The operator session could not be ended")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) createOperator(w http.ResponseWriter, r *http.Request) {
	principal, ok := identity.PrincipalFromContext(r.Context())
	if !ok {
		writeUnauthorized(w)
		return
	}
	var input struct {
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
		Role        string `json:"role"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	operator, err := h.identities.CreateOperator(r.Context(), identity.CreateOperatorInput{
		TenantID: principal.TenantID, Email: input.Email, DisplayName: input.DisplayName,
		Password: input.Password, Role: input.Role,
	})
	if err != nil {
		switch {
		case errors.Is(err, identity.ErrInvalidOperator), errors.Is(err, identity.ErrInvalidPassword):
			writeProblem(w, http.StatusBadRequest, "invalid operator", "Provide a valid email, display name, password, and role")
		case errors.Is(err, identity.ErrOperatorExists):
			writeProblem(w, http.StatusConflict, "operator already exists", "Choose a different email address")
		default:
			writeProblem(w, http.StatusInternalServerError, "operator creation failed", "The operator could not be created")
		}
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": operator.ID, "email": operator.Email, "display_name": operator.DisplayName, "role": operator.Role,
	})
}

func (h *Handler) getChannels(w http.ResponseWriter, r *http.Request) {
	principal, ok := identity.PrincipalFromContext(r.Context())
	if !ok {
		writeUnauthorized(w)
		return
	}
	config, err := h.tenants.ChannelConfig(r.Context(), principal.TenantID)
	if err != nil {
		handleTenantError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, config)
}

func (h *Handler) updateChannels(w http.ResponseWriter, r *http.Request) {
	principal, ok := identity.PrincipalFromContext(r.Context())
	if !ok {
		writeUnauthorized(w)
		return
	}
	var input tenants.ChannelConfig
	if !decodeJSON(w, r, &input) {
		return
	}
	if err := h.tenants.UpdateChannels(r.Context(), principal.TenantID, input); err != nil {
		handleTenantError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

func (h *Handler) createService(w http.ResponseWriter, r *http.Request) {
	principal, ok := identity.PrincipalFromContext(r.Context())
	if !ok {
		writeUnauthorized(w)
		return
	}
	var input tenants.ServiceInput
	if !decodeJSON(w, r, &input) {
		return
	}
	service, err := h.tenants.CreateService(r.Context(), principal.TenantID, input)
	if err != nil {
		handleTenantError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, service)
}

func (h *Handler) createStaff(w http.ResponseWriter, r *http.Request) {
	principal, ok := identity.PrincipalFromContext(r.Context())
	if !ok {
		writeUnauthorized(w)
		return
	}
	var input tenants.StaffInput
	if !decodeJSON(w, r, &input) {
		return
	}
	member, err := h.tenants.CreateStaff(r.Context(), principal.TenantID, input)
	if err != nil {
		handleTenantError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, member)
}

func (h *Handler) addAvailability(w http.ResponseWriter, r *http.Request) {
	principal, ok := identity.PrincipalFromContext(r.Context())
	if !ok {
		writeUnauthorized(w)
		return
	}
	var input struct {
		Rules []tenants.AvailabilityRuleInput `json:"rules"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if err := h.tenants.AddAvailability(r.Context(), principal.TenantID, strings.TrimSpace(r.PathValue("staffID")), input.Rules); err != nil {
		handleTenantError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]bool{"created": true})
}

func handleTenantError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, tenants.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "not found", "The requested tenant resource does not exist")
	case errors.Is(err, tenants.ErrInvalidInput), errors.Is(err, identity.ErrInvalidPassword):
		writeProblem(w, http.StatusBadRequest, "invalid configuration", "The supplied tenant configuration is invalid")
	case errors.Is(err, tenants.ErrConflict):
		writeProblem(w, http.StatusConflict, "conflict", "A resource with that identifier already exists")
	default:
		writeProblem(w, http.StatusInternalServerError, "tenant update failed", "The tenant configuration could not be updated")
	}
}

func sessionResponse(principal identity.Principal) map[string]any {
	return map[string]any{
		"tenant_id": principal.TenantID, "tenant_name": principal.TenantName,
		"timezone": principal.Timezone, "currency": principal.Currency,
		"operator_id": principal.OperatorID, "operator_email": principal.Email,
		"operator_name": principal.DisplayName, "role": principal.Role,
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	if contentType := r.Header.Get("Content-Type"); contentType != "" && !strings.HasPrefix(contentType, "application/json") {
		writeProblem(w, http.StatusUnsupportedMediaType, "unsupported media type", "Content-Type must be application/json")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid request body", "The request body must be a single valid JSON object")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeProblem(w, http.StatusBadRequest, "invalid request body", "The request body must contain a single JSON object")
		return false
	}
	return true
}

func bearerToken(header string) (string, bool) {
	if len(header) > 1024 {
		return "", false
	}
	fields := strings.Fields(header)
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") || fields[1] == "" {
		return "", false
	}
	return fields[1], true
}

func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="kontor-operator"`)
	writeProblem(w, http.StatusUnauthorized, "unauthorized", "A valid operator session is required")
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeProblem(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type": "about:blank", "title": title, "status": status, "detail": detail,
	})
}
