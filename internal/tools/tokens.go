package tools

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidSlotToken = errors.New("invalid slot token")
	ErrExpiredSlotToken = errors.New("expired slot token")
	ErrSlotTokenScope   = errors.New("slot token belongs to another security scope")
)

type SlotClaims struct {
	Version        int       `json:"v"`
	TenantID       string    `json:"tenant_id"`
	ConversationID string    `json:"conversation_id"`
	ServiceID      string    `json:"service_id"`
	ServiceName    string    `json:"service_name"`
	StaffID        string    `json:"staff_id"`
	StaffName      string    `json:"staff_name"`
	StartAt        time.Time `json:"start_at"`
	EndAt          time.Time `json:"end_at"`
	Timezone       string    `json:"timezone"`
	ExpiresAt      time.Time `json:"expires_at"`
}

// SlotSigner authenticates model-visible slot claims. Availability must still
// be checked by the backend inside the booking transaction.
type SlotSigner struct {
	key []byte
}

func NewSlotSigner(key []byte) (*SlotSigner, error) {
	if len(key) < 32 {
		return nil, errors.New("slot signing key must contain at least 32 bytes")
	}
	return &SlotSigner{key: append([]byte(nil), key...)}, nil
}

func (s *SlotSigner) Sign(claims SlotClaims) (string, error) {
	if claims.Version == 0 {
		claims.Version = 1
	}
	if claims.Version != 1 || claims.TenantID == "" || claims.ConversationID == "" ||
		claims.ServiceID == "" || claims.StaffID == "" || claims.StartAt.IsZero() ||
		claims.EndAt.IsZero() || !claims.StartAt.Before(claims.EndAt) || claims.ExpiresAt.IsZero() {
		return "", fmt.Errorf("%w: incomplete claims", ErrInvalidSlotToken)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal slot claims: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	signature := s.mac(encoded)
	return "slt_v1_" + encoded + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (s *SlotSigner) Verify(token string, trusted TrustedContext, now time.Time) (SlotClaims, error) {
	var claims SlotClaims
	if len(token) > 1024 || !strings.HasPrefix(token, "slt_v1_") {
		return claims, ErrInvalidSlotToken
	}
	parts := strings.Split(strings.TrimPrefix(token, "slt_v1_"), ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return claims, ErrInvalidSlotToken
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(signature, s.mac(parts[0])) {
		return claims, ErrInvalidSlotToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || json.Unmarshal(payload, &claims) != nil {
		return SlotClaims{}, ErrInvalidSlotToken
	}
	if claims.Version != 1 || claims.TenantID == "" || claims.ConversationID == "" ||
		claims.ServiceID == "" || claims.StaffID == "" || claims.StartAt.IsZero() ||
		claims.EndAt.IsZero() || !claims.StartAt.Before(claims.EndAt) || claims.ExpiresAt.IsZero() {
		return SlotClaims{}, ErrInvalidSlotToken
	}
	if claims.TenantID != trusted.TenantID || claims.ConversationID != trusted.ConversationID {
		return SlotClaims{}, ErrSlotTokenScope
	}
	if !now.Before(claims.ExpiresAt) {
		return SlotClaims{}, ErrExpiredSlotToken
	}
	return claims, nil
}

func (s *SlotSigner) mac(encodedPayload string) []byte {
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write([]byte("kontor.slot.v1\x00"))
	_, _ = mac.Write([]byte(encodedPayload))
	return mac.Sum(nil)
}
