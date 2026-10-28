package kloakd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParlyr_Parse(t *testing.T) {
	server := httptest.NewServer(jsonHandler(200, map[string]interface{}{
		"intent":          "scrape_site",
		"confidence":      float64(0.97),
		"tier":            float64(1),
		"source":          "fast_match",
		"entities":        map[string]interface{}{"url": "https://example.com"},
		"requires_action": true,
		"detected_url":    "https://example.com",
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

type ssePair = struct {
	Event string
	Data  map[string]interface{}
}

// chatEvent is a payload as chat_orchestrator.py's ChatEvent.to_json writes it.
func chatEvent(typ, content string, data map[string]interface{}) ssePair {
	var c interface{}
	if content != "" {
		c = content
	}
	var d interface{}
	if data != nil {
		d = data
	}
	return ssePair{typ, map[string]interface{}{"type": typ, "content": c, "data": d, "actions": nil, "timestamp": "2026-10-28T12:00:00"}}
}

func TestParlyr_ChatStream(t *testing.T) {
	pairs := []ssePair{
		chatEvent("thinking", "Looking at the site", nil),
		chatEvent("interpretation", "I'll extract books from example.com.", map[string]interface{}{"url": "https://example.com", "entity": "books"}),
		{"end", map[string]interface{}{}},
		chatEvent("text", "after end", nil),
	}
	server := httptest.NewServer(sseEventHandler(pairs))
	defer server.Close()

	client := newTestClient(server)
	ch, err := client.Parlyr.ChatStream(context.Background(), "sess-001", "scrape example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	events := drain(ch, 2*time.Second)
	if len(events) != 2 {
		t.Fatalf("expected 2 events (the stream stops at end), got %+v", events)
	}
	if events[0].Type != "thinking" || events[0].Content == nil || *events[0].Content != "Looking at the site" || events[0].Timestamp == "" {
		t.Errorf("unexpected thinking event: %+v", events[0])
	}
	if events[1].Type != "interpretation" || events[1].Data["entity"] != "books" {
		t.Errorf("unexpected interpretation event: %+v", events[1])
	}
}

func TestParlyr_Chat(t *testing.T) {
	results := map[string]interface{}{"items": []interface{}{map[string]interface{}{"title": "A"}}, "count": float64(1), "job_id": "j1"}
	pairs := []ssePair{
		chatEvent("interpretation", "I'll extract books.", map[string]interface{}{"url": "https://example.com", "entity": "books", "summary": "Extract books"}),
		chatEvent("progress", "Fetching", map[string]interface{}{"stage": "fetch"}),
		chatEvent("warning", "Large site", nil),
		chatEvent("results", "Extracted 1 items", results),
		chatEvent("text", "Done.", nil),
		chatEvent("clarification", "Want prices too?", nil),
		{"end", map[string]interface{}{}},
	}
	server := httptest.NewServer(sseEventHandler(pairs))
	defer server.Close()

	turn, err := newTestClient(server).Parlyr.Chat(context.Background(), "sess-001", "scrape example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !turn.OK || turn.Response != "Done.\nWant prices too?" {
		t.Errorf("unexpected response: %q", turn.Response)
	}
	if turn.Intent != "" || turn.Interpretation["summary"] != "Extract books" {
		t.Errorf("unexpected interpretation: %q %v", turn.Intent, turn.Interpretation)
	}
	if len(turn.Results) != 1 || turn.Results[0]["job_id"] != "j1" || len(turn.Warnings) != 1 || len(turn.Events) != 6 {
		t.Errorf("unexpected turn: %+v", turn)
	}
	if turn.ClarificationNeeded == nil || *turn.ClarificationNeeded != "Want prices too?" {
		t.Errorf("unexpected clarification: %v", turn.ClarificationNeeded)
	}
}

func TestParlyr_Chat_ErrorEventsAreCollected(t *testing.T) {
	errEv := chatEvent("error", "The site blocked us", map[string]interface{}{"error": "blocked", "job_id": "j1"})
	errEv.Data["actions"] = []interface{}{map[string]interface{}{"id": "retry", "label": "Try Again"}}
	pairs := []ssePair{chatEvent("text", "Starting", nil), errEv, {"end", map[string]interface{}{}}}
	server := httptest.NewServer(sseEventHandler(pairs))
	defer server.Close()

	turn, err := newTestClient(server).Parlyr.Chat(context.Background(), "s", "m")
	if err != nil {
		t.Fatalf("an error event is not a Go error: %v", err)
	}
	if turn.OK || len(turn.Errors) != 1 || *turn.Errors[0].Content != "The site blocked us" ||
		turn.Errors[0].Data["job_id"] != "j1" || len(turn.Errors[0].Actions) != 1 || turn.Actions == nil {
		t.Fatalf("unexpected turn: %+v", turn)
	}
}

func TestParlyr_Chat_IntentWhenTheAPISendsOne(t *testing.T) {
	pairs := []ssePair{chatEvent("interpretation", "x", map[string]interface{}{"intent": "scrape"}), {"end", map[string]interface{}{}}}
	server := httptest.NewServer(sseEventHandler(pairs))
	defer server.Close()
	turn, err := newTestClient(server).Parlyr.Chat(context.Background(), "s", "m")
	if err != nil || turn.Intent != "scrape" {
		t.Fatalf("unexpected: %+v %v", turn, err)
	}
}

func TestParlyr_DeleteSession_EscapesTheID(t *testing.T) {
	var raw string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw = r.URL.EscapedPath()
		w.WriteHeader(204)
	}))
	defer server.Close()
	if err := newTestClient(server).Parlyr.DeleteSession(context.Background(), "a/b"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(raw, "/parlyr/chat/a%2Fb") {
		t.Errorf("unexpected path %s", raw)
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
