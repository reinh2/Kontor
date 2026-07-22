package tenants

import (
	"errors"
	"testing"
)

func TestSlugFromHostAcceptsOnlyOneTenantLabel(t *testing.T) {
	tests := []struct {
		host   string
		suffix string
		want   string
		ok     bool
	}{
		{host: "salon-nord.kontor.example", suffix: "kontor.example", want: "salon-nord", ok: true},
		{host: "SALON-NORD.KONTOR.EXAMPLE:8080", suffix: "kontor.example", want: "salon-nord", ok: true},
		{host: "salon.nord.kontor.example", suffix: "kontor.example"},
		{host: "kontor.example", suffix: "kontor.example"},
		{host: "salon-nord.other.example", suffix: "kontor.example"},
	}
	for _, test := range tests {
		t.Run(test.host, func(t *testing.T) {
			got, ok := slugFromHost(test.host, test.suffix)
			if got != test.want || ok != test.ok {
				t.Fatalf("slugFromHost(%q, %q) = (%q, %v), want (%q, %v)", test.host, test.suffix, got, ok, test.want, test.ok)
			}
		})
	}
}

func TestSecretCipherRoundTripAndTamperResistance(t *testing.T) {
	cipher, err := newSecretCipher([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("newSecretCipher: %v", err)
	}
	ciphertext, nonce, err := cipher.seal("telegram-bot-token")
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if string(ciphertext) == "telegram-bot-token" {
		t.Fatal("secret was stored as plaintext")
	}
	plaintext, err := cipher.open(ciphertext, nonce)
	if err != nil || plaintext != "telegram-bot-token" {
		t.Fatalf("open = %q, %v", plaintext, err)
	}
	ciphertext[0] ^= 1
	if _, err := cipher.open(ciphertext, nonce); err == nil {
		t.Fatal("tampered ciphertext was accepted")
	}
}

func TestValidateProvisionRejectsCrossTenantStyleConfiguration(t *testing.T) {
	valid := ProvisionInput{
		Slug: "salon-nord", Name: "Salon Nord", Timezone: "Europe/Berlin", Currency: "EUR",
		Owner:    OwnerInput{Email: "owner@salon-nord.test", DisplayName: "Owner", Password: "correct-horse-battery-staple"},
		Channels: ChannelConfig{WidgetOrigin: "https://salon-nord.example"},
		Services: []ServiceInput{{Slug: "cut", Name: "Cut", DurationMinutes: 30, Currency: "EUR"}},
		Staff: []StaffInput{{
			Slug: "ada", DisplayName: "Ada", Timezone: "Europe/Berlin", ServiceSlugs: []string{"cut"},
			Availability: []AvailabilityRuleInput{{RuleType: "working", DayOfWeek: 1, LocalStart: "09:00", LocalEnd: "17:00"}},
		}},
	}
	if err := validateProvision(valid); err != nil {
		t.Fatalf("valid provision input rejected: %v", err)
	}
	valid.Staff[0].ServiceSlugs = []string{"service-owned-by-another-tenant"}
	if err := validateProvision(valid); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("cross-tenant service slug error = %v, want ErrInvalidInput", err)
	}
}

func TestCanonicalWidgetOriginNormalizesBrowserOriginsAndRejectsNonOrigins(t *testing.T) {
	for _, test := range []struct {
		input string
		want  string
	}{
		{input: "HTTPS://Salon.Example:8443/", want: "https://salon.example:8443"},
		{input: "https://salon.example:443", want: "https://salon.example"},
		{input: "https://salon.example:0443", want: "https://salon.example"},
		{input: "http://salon.example:80/", want: "http://salon.example"},
		{input: "http://salon.example:00080/", want: "http://salon.example"},
		{input: "https://salon.example:0008443", want: "https://salon.example:8443"},
	} {
		origin, err := CanonicalWidgetOrigin(test.input)
		if err != nil {
			t.Fatalf("CanonicalWidgetOrigin(%q): %v", test.input, err)
		}
		if origin != test.want {
			t.Fatalf("canonical origin for %q = %q, want %q", test.input, origin, test.want)
		}
	}
	for _, value := range []string{
		"https://salon.example/path",
		"https://salon.example/?source=setup",
		"https://salon.example/#widget",
		"https://user@salon.example",
	} {
		if _, err := CanonicalWidgetOrigin(value); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("CanonicalWidgetOrigin(%q) error = %v, want ErrInvalidInput", value, err)
		}
	}
}
