package kloakd

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTransport_AuthHeaders(t *testing.T) {
	var capturedReq *http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = r
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := newTestClient(server)
	_, err := client.Evadr.Fetch(context.Background(), "https://example.com", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertHeader(t, capturedReq, "Authorization", "Bearer "+testAPIKey)
	assertHeader(t, capturedReq, "X-Kloakd-Organization", testOrgID)
	assertHeaderContains(t, capturedReq, "X-Kloakd-SDK", "go/")
}

func TestTransport_OrgPrefixInURL(t *testing.T) {
	var capturedURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := newTestClient(server)
	_, _ = client.Evadr.Fetch(context.Background(), "https://example.com", nil)

	expected := "/api/v1/organizations/" + testOrgID + "/evadr/fetch"
	if capturedURL != expected {
		t.Errorf("expected URL path %s, got %s", expected, capturedURL)
	}
}

func TestTransport_NonRetryableError(t *testing.T) {
	server := httptest.NewServer(jsonHandler(401, map[string]interface{}{"detail": "invalid key"}))
	defer server.Close()

	client := newTestClient(server)
	_, err := client.Evadr.Fetch(context.Background(), "https://example.com", nil)
	var authErr *AuthenticationError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected AuthenticationError, got %T: %v", err, err)
	}
}

func TestTransport_RetryExhausted(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		w.Write([]byte(`{"detail":"server error"}`))
	}))
	defer server.Close()

	// Use maxRetries=2 for this test
	client, _ := New(Config{
		APIKey:         testAPIKey,
		OrganizationID: testOrgID,
		BaseURL:        server.URL,
		MaxRetries:     2,
		HTTPClient:     server.Client(),
		Timeout:        1 * time.Second,
	})
	// Override backoff to be instant
	_, err := client.Evadr.Fetch(context.Background(), "https://example.com", nil)
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	var apiErr *ApiError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected ApiError, got %T: %v", err, err)
	}
	if calls != 3 { // 1 initial + 2 retries
		t.Errorf("expected 3 calls (1 + 2 retries), got %d", calls)
	}
}

func TestTransport_QueryParams(t *testing.T) {
	var capturedURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := newTestClient(server)
	// GetHierarchy uses GET with path param, no query; test GET with params via skanyr.GetJob
	_, _ = client.Webgrph.GetHierarchy(context.Background(), "art-001")
	if !strings.Contains(capturedURL, "art-001") {
		t.Errorf("expected URL to contain art-001, got %s", capturedURL)
	}
}

func TestBackoff_RetryAfterHeader(t *testing.T) {
	d := backoff(0, "5", map[string]interface{}{})
	if d != 5*time.Second {
		t.Errorf("expected 5s from Retry-After header, got %v", d)
	}
}

func TestBackoff_RetryAfterBody(t *testing.T) {
	d := backoff(0, "", map[string]interface{}{"retry_after": float64(10)})
	if d != 10*time.Second {
		t.Errorf("expected 10s from body, got %v", d)
	}
}

func TestBackoff_Exponential(t *testing.T) {
	d0 := backoff(0, "", map[string]interface{}{})
	d1 := backoff(1, "", map[string]interface{}{})
	if d1 <= d0 {
		t.Errorf("expected d1 > d0, got d0=%v d1=%v", d0, d1)
	}
}

func TestTransport_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	client := newTestClient(server)
	_, err := client.Evadr.Fetch(ctx, "https://example.com", nil)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}
