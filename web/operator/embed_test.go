package operator

import (
	"strings"
	"testing"
)

func TestIndexIncludesOperatorDestinationsAndDashboardAliases(t *testing.T) {
	page := string(IndexPage)
	for _, want := range []string{
		"#/overview", "#/dashboard", "#/runs", "#/calendar", "#/inbox", "#/analytics", "#/settings",
		"function InboxScreen", "function AnalyticsScreen", "function SettingsScreen",
		"Loading inbox", "Could not load the inbox", "Inbox zero",
		"Loading analytics", "Could not load analytics",
		"Loading settings", "Could not load settings",
		"aria-label': 'Operator navigation'", "focus-visible", "@media (max-width: 759px)",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("embedded operator page is missing %q", want)
		}
	}
}
