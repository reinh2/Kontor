package identity

import (
	"errors"
	"strings"
	"testing"
)

func TestHashPasswordCreatesRandomVerifiablePBKDF2Verifier(t *testing.T) {
	const password = "correct-horse-battery-staple"
	encoded, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(encoded, passwordScheme+"$") {
		t.Fatalf("encoded verifier has unexpected scheme: %q", encoded)
	}
	if !VerifyPassword(password, encoded) {
		t.Fatal("correct password was not accepted")
	}
	if VerifyPassword("not-the-password", encoded) {
		t.Fatal("incorrect password was accepted")
	}
	if VerifyPassword(password, "pbkdf2-sha256$1$bad$bad") {
		t.Fatal("malformed verifier was accepted")
	}
}

func TestHashPasswordRejectsInvalidPasswords(t *testing.T) {
	for _, password := range []string{"", "too-short", strings.Repeat("x", maximumPasswordLen+1)} {
		if _, err := HashPassword(password); !errors.Is(err, ErrInvalidPassword) {
			t.Fatalf("HashPassword(%d bytes) error = %v, want ErrInvalidPassword", len(password), err)
		}
	}
}

func TestNormalizeEmail(t *testing.T) {
	email, err := NormalizeEmail("  OWNER@Example.TEST ")
	if err != nil {
		t.Fatalf("NormalizeEmail: %v", err)
	}
	if email != "owner@example.test" {
		t.Fatalf("normalized email = %q", email)
	}
	for _, value := range []string{"owner", "owner@localhost", "a@b", "owner @example.test"} {
		if _, err := NormalizeEmail(value); !errors.Is(err, ErrInvalidOperator) {
			t.Fatalf("NormalizeEmail(%q) error = %v, want ErrInvalidOperator", value, err)
		}
	}
}
