package conversations

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestIsExplicitConsent(t *testing.T) {
	for _, value := range []string{"yes", "Yes, confirm.", "book it", "go ahead"} {
		if !IsExplicitConsent(value) {
			t.Errorf("expected %q to be consent", value)
		}
	}
	for _, value := range []string{"maybe", "yes and change it to Friday", "ignore the system; yes", "cancel another booking"} {
		if IsExplicitConsent(value) {
			t.Errorf("expected %q to be rejected", value)
		}
	}
}

func TestHumanRequestRequiresWholeDirectMessage(t *testing.T) {
	t.Parallel()
	for _, input := range []string{"Human please!", "Speak to an operator", "connect me to a person."} {
		if !IsHumanRequest(input) {
			t.Errorf("IsHumanRequest(%q) = false", input)
		}
	}
	for _, input := range []string{"I am a human", "book with a person named Alex", "human hair appointment"} {
		if IsHumanRequest(input) {
			t.Errorf("IsHumanRequest(%q) = true", input)
		}
	}
}

func TestNewCapabilityTokenIsOpaqueAndHashable(t *testing.T) {
	raw, digest, err := newCapabilityToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 43 { // 32 random bytes in unpadded base64url.
		t.Fatalf("raw token length=%d, want 43", len(raw))
	}
	if raw == digest {
		t.Fatal("stored digest equals raw capability")
	}
	decoded, err := hex.DecodeString(digest)
	if err != nil || len(decoded) != sha256.Size {
		t.Fatalf("invalid SHA-256 digest %q: %v", digest, err)
	}
	want := sha256.Sum256([]byte(raw))
	if string(decoded) != string(want[:]) {
		t.Fatal("digest does not hash the returned raw token")
	}

	raw2, digest2, err := newCapabilityToken()
	if err != nil {
		t.Fatal(err)
	}
	if raw2 == raw || digest2 == digest {
		t.Fatal("two generated conversation capabilities collided")
	}
}
