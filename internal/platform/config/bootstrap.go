package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

const stage6BootstrapEnabledKey = "STAGE6_BOOTSTRAP_ENABLED"

var stage6BootstrapFieldKeys = []string{
	"STAGE6_BOOTSTRAP_TENANT_ID",
	"STAGE6_BOOTSTRAP_TENANT_SLUG",
	"STAGE6_BOOTSTRAP_WIDGET_ORIGIN",
	"STAGE6_BOOTSTRAP_OWNER_EMAIL",
	"STAGE6_BOOTSTRAP_OWNER_DISPLAY_NAME",
	"STAGE6_BOOTSTRAP_OWNER_PASSWORD",
}

// LegacyTenantBootstrap is an explicitly enabled, one-time recovery input for
// a tenant that existed before Stage 6. It is deliberately separate from demo
// defaults so production startup cannot silently create an owner account.
type LegacyTenantBootstrap struct {
	Enabled          bool
	TenantID         string
	TenantSlug       string
	WidgetOrigin     string
	OwnerEmail       string
	OwnerDisplayName string
	OwnerPassword    string
}

// LoadLegacyTenantBootstrap validates the all-or-nothing, fail-closed legacy
// tenant recovery configuration. The caller is responsible for consuming it
// only in the API process after migrations have completed.
func LoadLegacyTenantBootstrap(demoMode bool) (LegacyTenantBootstrap, error) {
	var bootstrap LegacyTenantBootstrap
	rawEnabled := strings.TrimSpace(os.Getenv(stage6BootstrapEnabledKey))
	if rawEnabled != "" {
		enabled, err := strconv.ParseBool(rawEnabled)
		if err != nil {
			return LegacyTenantBootstrap{}, errors.New("STAGE6_BOOTSTRAP_ENABLED must be a boolean")
		}
		bootstrap.Enabled = enabled
	}

	hasField := false
	for _, key := range stage6BootstrapFieldKeys {
		if os.Getenv(key) != "" {
			hasField = true
			break
		}
	}
	if !bootstrap.Enabled {
		if hasField {
			return LegacyTenantBootstrap{}, errors.New("STAGE6_BOOTSTRAP_* fields require STAGE6_BOOTSTRAP_ENABLED=true")
		}
		return bootstrap, nil
	}
	if demoMode {
		return LegacyTenantBootstrap{}, errors.New("STAGE6_BOOTSTRAP_ENABLED is not allowed when DEMO_MODE=true")
	}

	bootstrap.TenantID = strings.TrimSpace(os.Getenv("STAGE6_BOOTSTRAP_TENANT_ID"))
	bootstrap.TenantSlug = strings.TrimSpace(os.Getenv("STAGE6_BOOTSTRAP_TENANT_SLUG"))
	bootstrap.WidgetOrigin = strings.TrimSpace(os.Getenv("STAGE6_BOOTSTRAP_WIDGET_ORIGIN"))
	bootstrap.OwnerEmail = strings.TrimSpace(os.Getenv("STAGE6_BOOTSTRAP_OWNER_EMAIL"))
	bootstrap.OwnerDisplayName = strings.TrimSpace(os.Getenv("STAGE6_BOOTSTRAP_OWNER_DISPLAY_NAME"))
	bootstrap.OwnerPassword = os.Getenv("STAGE6_BOOTSTRAP_OWNER_PASSWORD")
	if !validBootstrapTenantID(bootstrap.TenantID) || bootstrap.TenantSlug == "" || bootstrap.WidgetOrigin == "" ||
		bootstrap.OwnerEmail == "" || bootstrap.OwnerDisplayName == "" || bootstrap.OwnerPassword == "" {
		return LegacyTenantBootstrap{}, errors.New("STAGE6_BOOTSTRAP_ENABLED requires a complete tenant, channel, and owner configuration")
	}
	return bootstrap, nil
}

func validBootstrapTenantID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index := 0; index < len(value); index++ {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if value[index] != '-' {
				return false
			}
			continue
		}
		character := value[index]
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f') || (character >= 'A' && character <= 'F')) {
			return false
		}
	}
	return true
}
