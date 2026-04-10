package kloakd

import (
	"fmt"
	"net/http"
	"time"
)

// Config holds all configuration for the KLOAKD client.
type Config struct {
	// APIKey is the KLOAKD API key (required). Starts with "sk-".
	APIKey string

	// OrganizationID is the UUID of the organization (required).
	OrganizationID string

	// BaseURL overrides the API base URL. Defaults to https://api.kloakd.dev.
	BaseURL string

	// Timeout is the per-request timeout. Defaults to 30s.
	Timeout time.Duration

	// MaxRetries sets the number of retry attempts for retryable errors.
	// 0 means one attempt (no retries). Defaults to 3.
	MaxRetries int

	// HTTPClient allows injecting a custom *http.Client (e.g. for testing).
	HTTPClient *http.Client
}

func (c Config) effectiveBaseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return defaultBaseURL
}

func (c Config) effectiveTimeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return defaultTimeout
}

func (c Config) effectiveMaxRetries() int {
	if c.MaxRetries > 0 {
		return c.MaxRetries
	}
	return defaultRetries
}

// Client is the KLOAKD SDK client. Create one with New().
//
//	client := kloakd.New(kloakd.Config{
//	    APIKey:         "sk-live-...",
//	    OrganizationID: "your-org-uuid",
//	})
type Client struct {
	Evadr   *EvadrNamespace
	Webgrph *WebgrphNamespace
	Skanyr  *SkanyrNamespace
	Nexus   *NexusNamespace
	Parlyr  *ParlyrNamespace
	Fetchyr *FetchyrNamespace
	Kolektr *KolektrNamespace

	t *transport
}

// New creates a new KLOAKD client with the given configuration.
// Returns an error if APIKey or OrganizationID are missing.
func New(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("kloakd: APIKey is required")
	}
	if cfg.OrganizationID == "" {
		return nil, fmt.Errorf("kloakd: OrganizationID is required")
	}

	t := newTransport(cfg)
	c := &Client{t: t}
	c.Evadr = &EvadrNamespace{t: t}
	c.Webgrph = &WebgrphNamespace{t: t}
	c.Skanyr = &SkanyrNamespace{t: t}
	c.Nexus = &NexusNamespace{t: t}
	c.Parlyr = &ParlyrNamespace{t: t}
	c.Fetchyr = &FetchyrNamespace{t: t}
	c.Kolektr = &KolektrNamespace{t: t}
	return c, nil
}

// MustNew is like New but panics on configuration error.
func MustNew(cfg Config) *Client {
	c, err := New(cfg)
	if err != nil {
		panic(err)
	}
	return c
}
