package widget

import (
	"strings"
	"testing"
)

func TestScriptIncludesSafeDesignComponents(t *testing.T) {
	script := string(Script)
	for _, want := range []string{
		"function addWorkingIndicator", "function addConfirmationCard",
		"Nothing is booked until you confirm.", "Kontor will book this",
		"role\", \"status\"",
		"pending_confirmation_active", "data-pending-confirmation",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("embedded widget script is missing %q", want)
		}
	}
	if strings.Contains(script, "addSlotPicker(turn.message)") {
		t.Fatal("widget must not infer available slots from times mentioned in assistant prose")
	}
}
