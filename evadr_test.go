package kloakd

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"
)

var fetchResponse = map[string]interface{}{
	"success":           true,
	"url":               "https://example.com",
	"status_code":       float64(200),
	"tier_used":         float64(2),
	"html":              "<html>content</html>",
	"vendor_detected":   "cloudflare",
	"anti_bot_bypassed": true,
	"artifact_id":       "art-fetch-001",
	"error":             nil,
}

func TestEvadr_Fetch(t *testing.T) {
	server := httptest.NewServer(jsonHandler(200, fetchResponse))
	defer server.Close()

	client := newTestClient(server)
	result, err := client.Evadr.Fetch(context.Background(), "https://example.com", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected Success=true")
	}
	if result.TierUsed != 2 {
		t.Errorf("expected TierUsed=2, got %d", result.TierUsed)
	}
	if !result.AntiBotBypassed {
		t.Error("expected AntiBotBypassed=true")
	}
	if result.ArtifactID == nil || *result.ArtifactID != "art-fetch-001" {
		t.Errorf("unexpected ArtifactID: %v", result.ArtifactID)
	}
}

func TestEvadr_Fetch_WithOptions(t *testing.T) {
	cap := &captureHandler{inner: jsonHandler(200, fetchResponse)}
	server := httptest.NewServer(cap)
	defer server.Close()

	client := newTestClient(server)
	_, err := client.Evadr.Fetch(context.Background(), "https://example.com", &FetchOptions{
		ForceBrowser:      true,
		UseProxy:          true,
		SessionArtifactID: "sess-001",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cap.lastBody["force_browser"] != true {
		t.Error("expected force_browser=true in body")
	}
	if cap.lastBody["use_proxy"] != true {
		t.Error("expected use_proxy=true in body")
	}
	if cap.lastBody["session_artifact_id"] != "sess-001" {
		t.Errorf("unexpected session_artifact_id: %v", cap.lastBody["session_artifact_id"])
	}
}

func TestEvadr_Fetch_401(t *testing.T) {
	server := httptest.NewServer(jsonHandler(401, map[string]interface{}{"detail": "bad key"}))
	defer server.Close()

	client := newTestClient(server)
	_, err := client.Evadr.Fetch(context.Background(), "https://example.com", nil)
	var authErr *AuthenticationError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected AuthenticationError, got %T: %v", err, err)
	}
}

func TestEvadr_FetchAsync(t *testing.T) {
	server := httptest.NewServer(jsonHandler(200, map[string]interface{}{"job_id": "job-001"}))
	defer server.Close()

	client := newTestClient(server)
	jobID, err := client.Evadr.FetchAsync(context.Background(), "https://example.com", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if jobID != "job-001" {
		t.Errorf("expected job_id=job-001, got %s", jobID)
	}
}

func TestEvadr_FetchStream(t *testing.T) {
	server := httptest.NewServer(sseHandler([]map[string]interface{}{
		{"type": "tier_start", "tier": float64(1), "vendor": nil, "metadata": map[string]interface{}{}},
		{"type": "tier_complete", "tier": float64(1), "vendor": "cloudflare", "metadata": map[string]interface{}{}},
		{"type": "done", "tier": float64(1), "vendor": "cloudflare", "metadata": map[string]interface{}{}},
	}))
	defer server.Close()

	client := newTestClient(server)
	ch, err := client.Evadr.FetchStream(context.Background(), "https://example.com", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	events := drain(ch, 2*time.Second)
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[0].Type != "tier_start" {
		t.Errorf("expected first event type=tier_start, got %s", events[0].Type)
	}
	if events[1].Vendor == nil || *events[1].Vendor != "cloudflare" {
		t.Errorf("expected vendor=cloudflare in second event")
	}
}

func TestEvadr_FetchStream_WithForceBrowser(t *testing.T) {
	cap := &captureHandler{inner: sseHandler([]map[string]interface{}{
		{"type": "done", "metadata": map[string]interface{}{}},
	})}
	server := httptest.NewServer(cap)
	defer server.Close()

	client := newTestClient(server)
	ch, err := client.Evadr.FetchStream(context.Background(), "https://example.com", &FetchStreamOptions{ForceBrowser: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	drain(ch, time.Second)
	if cap.lastBody["force_browser"] != true {
		t.Error("expected force_browser=true in SSE body")
	}
}

func TestEvadr_Analyze(t *testing.T) {
	server := httptest.NewServer(jsonHandler(200, map[string]interface{}{
		"blocked":             true,
		"vendor":              "cloudflare",
		"confidence":          float64(0.95),
		"recommended_actions": []interface{}{"use_proxy", "stealth_browser"},
	}))
	defer server.Close()

	client := newTestClient(server)
	result, err := client.Evadr.Analyze(context.Background(), "https://example.com", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Blocked {
		t.Error("expected Blocked=true")
	}
	if result.Confidence != 0.95 {
		t.Errorf("expected Confidence=0.95, got %f", result.Confidence)
	}
	if len(result.RecommendedActions) != 2 {
		t.Errorf("expected 2 recommended actions, got %d", len(result.RecommendedActions))
	}
}

func TestEvadr_StoreProxy(t *testing.T) {
	cap := &captureHandler{inner: jsonHandler(200, map[string]interface{}{})}
	server := httptest.NewServer(cap)
	defer server.Close()

	client := newTestClient(server)
	err := client.Evadr.StoreProxy(context.Background(), "my-proxy", "http://proxy.example.com:8080")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cap.lastBody["name"] != "my-proxy" {
		t.Errorf("expected name=my-proxy, got %v", cap.lastBody["name"])
	}
	if cap.lastBody["proxy_url"] != "http://proxy.example.com:8080" {
		t.Errorf("unexpected proxy_url: %v", cap.lastBody["proxy_url"])
	}
}
