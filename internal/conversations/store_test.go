package conversations

import "testing"

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
