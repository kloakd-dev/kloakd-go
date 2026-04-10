package kloakd

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParlyr_Parse(t *testing.T) {
	server := httptest.NewServer(jsonHandler(200, map[string]interface{}{
		"intent":           "scrape_site",
		"confidence":       float64(0.97),
		"tier":             float64(1),
		"source":           "fast_match",
		"entities":         map[string]interface{}{"url": "https://example.com"},
		"requires_action":  true,
		"detected_url":     "https://example.com",
	}))
	defer server.Close()

	client := newTestClient(server)
	result, err := client.Parlyr.Parse(context.Background(), "scrape example.com", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Intent != "scrape_site" {
		t.Errorf("expected scrape_site, got %s", result.Intent)
	}
	if result.Confidence != 0.97 {
		t.Errorf("expected 0.97, got %f", result.Confidence)
	}
	if result.DetectedURL == nil || *result.DetectedURL != "https://example.com" {
		t.Errorf("unexpected DetectedURL: %v", result.DetectedURL)
	}
}

func TestParlyr_Parse_WithSessionID(t *testing.T) {
	cap := &captureHandler{inner: jsonHandler(200, map[string]interface{}{
		"intent": "scrape_site", "confidence": float64(0.9), "tier": float64(1),
		"source": "fast_match", "entities": map[string]interface{}{}, "requires_action": false,
	})}
	server := httptest.NewServer(cap)
	defer server.Close()

	client := newTestClient(server)
	_, err := client.Parlyr.Parse(context.Background(), "hello", &ParseOptions{SessionID: "sess-001"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cap.lastBody["session_id"] != "sess-001" {
		t.Errorf("unexpected session_id: %v", cap.lastBody["session_id"])
	}
}

func TestParlyr_ChatStream(t *testing.T) {
	pairs := []struct {
		Event string
		Data  map[string]interface{}
	}{
		{"intent", map[string]interface{}{"intent": "scrape_site", "confidence": float64(0.95), "tier": float64(2), "entities": map[string]interface{}{}, "requires_action": true}},
		{"response", map[string]interface{}{"content": "I will scrape that for you."}},
		{"done", map[string]interface{}{}},
	}
	server := httptest.NewServer(sseEventHandler(pairs))
	defer server.Close()

	client := newTestClient(server)
	ch, err := client.Parlyr.ChatStream(context.Background(), "sess-001", "scrape example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	events := drain(ch, 2*time.Second)
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[0].Event != "intent" {
		t.Errorf("expected intent event, got %s", events[0].Event)
	}
	if events[1].Event != "response" {
		t.Errorf("expected response event, got %s", events[1].Event)
	}
}

func TestParlyr_Chat(t *testing.T) {
	pairs := []struct {
		Event string
		Data  map[string]interface{}
	}{
		{"intent", map[string]interface{}{"intent": "scrape_site", "confidence": float64(0.95), "tier": float64(2), "entities": map[string]interface{}{}, "requires_action": true}},
		{"response", map[string]interface{}{"content": "Scraping now."}},
		{"end", map[string]interface{}{}},
	}
	server := httptest.NewServer(sseEventHandler(pairs))
	defer server.Close()

	client := newTestClient(server)
	turn, err := client.Parlyr.Chat(context.Background(), "sess-001", "scrape example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if turn.Intent != "scrape_site" {
		t.Errorf("expected intent=scrape_site, got %s", turn.Intent)
	}
	if turn.Response != "Scraping now." {
		t.Errorf("unexpected response: %s", turn.Response)
	}
}

func TestParlyr_DeleteSession(t *testing.T) {
	cap := &captureHandler{inner: jsonHandler(200, map[string]interface{}{})}
	server := httptest.NewServer(cap)
	defer server.Close()

	client := newTestClient(server)
	err := client.Parlyr.DeleteSession(context.Background(), "sess-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsPath(cap.lastURL, "sess-001") {
		t.Errorf("expected URL to contain sess-001, got %s", cap.lastURL)
	}
}

func containsPath(url, substr string) bool {
	return len(url) > 0 && len(substr) > 0 &&
		(len(url) >= len(substr) && (url[len(url)-len(substr):] == substr || containsStr(url, substr)))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
