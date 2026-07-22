package identity

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// EnsureOperator creates a seed account only when its tenant-local email is
// absent. It never resets an existing operator's password, so demo startup
// cannot unexpectedly invalidate a manually changed login.
func (s *Store) EnsureOperator(ctx context.Context, input CreateOperatorInput) (Operator, error) {
	if s == nil || s.pool == nil {
		return Operator{}, errors.New("identity: nil store")
	}
	email, err := NormalizeEmail(input.Email)
	if err != nil {
		return Operator{}, err
	}
	operator, err := s.operatorByEmail(ctx, input.TenantID, email)
	if err == nil {
		return operator, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Operator{}, err
	}
	operator, err = s.CreateOperator(ctx, input)
	if err == nil {
		return operator, nil
	}
	// Another startup may have inserted the account between the lookup and
	// insert. Read the winner rather than treating the idempotent seed as fatal.
	if loaded, loadErr := s.operatorByEmail(ctx, input.TenantID, email); loadErr == nil {
		return loaded, nil
	}
	return Operator{}, fmt.Errorf("identity: ensure operator: %w", err)
}

func (s *Store) operatorByEmail(ctx context.Context, tenantID, email string) (Operator, error) {
	var operator Operator
	err := s.pool.QueryRow(ctx, `
		SELECT tenant_id::text,id::text,email,display_name,role,active,created_at,updated_at
		FROM operators WHERE tenant_id=$1 AND email=$2`, tenantID, email,
	).Scan(&operator.TenantID, &operator.ID, &operator.Email, &operator.DisplayName,
		&operator.Role, &operator.Active, &operator.CreatedAt, &operator.UpdatedAt)
	if err != nil {
		return Operator{}, err
	}
	return operator, nil
}
