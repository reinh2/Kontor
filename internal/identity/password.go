package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"crypto/pbkdf2"
)

const (
	passwordScheme     = "pbkdf2-sha256"
	passwordIterations = 600_000
	passwordSaltBytes  = 16
	passwordKeyBytes   = 32
	minimumPasswordLen = 12
	maximumPasswordLen = 1024
)

var ErrInvalidPassword = errors.New("identity: password does not meet the security policy")

// HashPassword derives a one-way PBKDF2-HMAC-SHA-256 verifier with a random
// per-password salt. The standard-library implementation keeps authentication
// self-contained while the encoded work factor leaves room for later upgrades.
func HashPassword(password string) (string, error) {
	if err := validateNewPassword(password); err != nil {
		return "", err
	}
	salt := make([]byte, passwordSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("identity: generate password salt: %w", err)
	}
	derived, err := pbkdf2.Key(sha256.New, password, salt, passwordIterations, passwordKeyBytes)
	if err != nil {
		return "", fmt.Errorf("identity: derive password verifier: %w", err)
	}
	return strings.Join([]string{
		passwordScheme,
		fmt.Sprintf("%d", passwordIterations),
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(derived),
	}, "$"), nil
}

// VerifyPassword compares a plaintext candidate with a stored verifier in
// constant time. Stored malformed hashes are invalid rather than panicking.
func VerifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != passwordScheme || len(password) > maximumPasswordLen {
		return false
	}
	var iterations int
	if _, err := fmt.Sscanf(parts[1], "%d", &iterations); err != nil || iterations < 100_000 || iterations > 10_000_000 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil || len(salt) < passwordSaltBytes {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(want) != passwordKeyBytes {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iterations, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

func validateNewPassword(password string) error {
	if len(password) < minimumPasswordLen || len(password) > maximumPasswordLen {
		return ErrInvalidPassword
	}
	return nil
}
