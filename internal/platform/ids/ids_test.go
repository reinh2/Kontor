package ids

import (
	"regexp"
	"testing"
)

func TestNewUUIDv4(t *testing.T) {
	value := New()
	pattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !pattern.MatchString(value) {
		t.Fatalf("invalid UUIDv4 %q", value)
	}
	if value == New() {
		t.Fatal("expected distinct identifiers")
	}
}
