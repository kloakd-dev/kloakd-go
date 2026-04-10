package kloakd

import (
	"testing"
)

func TestNew_RequiresAPIKey(t *testing.T) {
	_, err := New(Config{OrganizationID: "org-001"})
	if err == nil {
		t.Fatal("expected error for missing APIKey")
	}
}

func TestNew_RequiresOrganizationID(t *testing.T) {
	_, err := New(Config{APIKey: "sk-test"})
	if err == nil {
		t.Fatal("expected error for missing OrganizationID")
	}
}

func TestNew_Success(t *testing.T) {
	client, err := New(Config{APIKey: "sk-test", OrganizationID: "org-001"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.Evadr == nil {
		t.Error("Evadr namespace is nil")
	}
	if client.Webgrph == nil {
		t.Error("Webgrph namespace is nil")
	}
	if client.Skanyr == nil {
		t.Error("Skanyr namespace is nil")
	}
	if client.Nexus == nil {
		t.Error("Nexus namespace is nil")
	}
	if client.Parlyr == nil {
		t.Error("Parlyr namespace is nil")
	}
	if client.Fetchyr == nil {
		t.Error("Fetchyr namespace is nil")
	}
	if client.Kolektr == nil {
		t.Error("Kolektr namespace is nil")
	}
}

func TestMustNew_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected MustNew to panic on invalid config")
		}
	}()
	MustNew(Config{})
}

func TestMustNew_Success(t *testing.T) {
	client := MustNew(Config{APIKey: "sk-test", OrganizationID: "org-001"})
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestConfig_Defaults(t *testing.T) {
	cfg := Config{}
	if cfg.effectiveBaseURL() != defaultBaseURL {
		t.Errorf("expected default base URL %s, got %s", defaultBaseURL, cfg.effectiveBaseURL())
	}
	if cfg.effectiveTimeout() != defaultTimeout {
		t.Errorf("expected default timeout %v, got %v", defaultTimeout, cfg.effectiveTimeout())
	}
	if cfg.effectiveMaxRetries() != defaultRetries {
		t.Errorf("expected default retries %d, got %d", defaultRetries, cfg.effectiveMaxRetries())
	}
}

func TestConfig_Overrides(t *testing.T) {
	cfg := Config{
		BaseURL:    "https://custom.api",
		Timeout:    60 * defaultTimeout,
		MaxRetries: 5,
	}
	if cfg.effectiveBaseURL() != "https://custom.api" {
		t.Errorf("unexpected base URL: %s", cfg.effectiveBaseURL())
	}
	if cfg.effectiveMaxRetries() != 5 {
		t.Errorf("expected maxRetries=5, got %d", cfg.effectiveMaxRetries())
	}
}
