// Package identity owns tenant-local operator accounts, opaque operator
// sessions, and role values. It deliberately does not expose a global user:
// an operator is always authenticated in exactly one tenant.
package identity

import "time"

const (
	RoleOwner = "owner"
	RoleStaff = "staff"
)

// Operator is a tenant-local operator account. PasswordHash is never exposed
// in a JSON payload and is only hydrated for authentication checks.
type Operator struct {
	TenantID     string
	ID           string
	Email        string
	DisplayName  string
	Role         string
	Active       bool
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Principal is the authenticated request identity. It includes immutable
// tenant facts needed by the operator API, avoiding any tenant selector in
// browser-originated data requests.
type Principal struct {
	SessionID    string
	TenantID     string
	TenantSlug   string
	TenantName   string
	Timezone     string
	Currency     string
	OperatorID   string
	Email        string
	DisplayName  string
	Role         string
	ExpiresAt    time.Time
}

// LoginResult contains the raw opaque bearer token exactly once. The database
// stores only its SHA-256 digest.
type LoginResult struct {
	Token     string
	Principal Principal
}

// CreateOperatorInput is used both by onboarding and by owner-managed staff
// invitations. Passwords travel only through this input and are never stored
// on an Operator value returned to callers.
type CreateOperatorInput struct {
	TenantID    string
	Email       string
	DisplayName string
	Password    string
	Role        string
}

func validRole(role string) bool { return role == RoleOwner || role == RoleStaff }
