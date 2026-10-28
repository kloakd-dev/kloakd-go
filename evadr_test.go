package kloakd

import (
	"context"
	"errors"
	"net/http"
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

func TestEvadr_FetchStream_StartsAJobAndStreamsItsEvents(t *testing.T) {
	// The fallback stream of modules/evadr/router.py: {type, job_id, ...}.
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST evadr/fetch/async": jsonHandler(202, map[string]interface{}{"job_id": "job-9", "status": "pending", "message": "queued"}),
		"GET evadr/jobs/job-9/events": sseHandler([]map[string]interface{}{
			{"type": "STATUS", "job_id": "job-9", "status": "running"},
			{"type": "COMPLETE", "job_id": "job-9", "tier_used": float64(2), "artifact_id": "art-1"},
		}),
	})
	defer server.Close()

	ch, err := newTestClient(server).Evadr.FetchStream(context.Background(), "https://example.com", &FetchStreamOptions{ForceBrowser: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	events := drain(ch, 2*time.Second)
	if len(events) != 2 || events[0].Type != "STATUS" || events[0].Status != "running" || events[0].JobID != "job-9" {
		t.Fatalf("unexpected events: %+v", events)
	}
	if events[1].Type != "COMPLETE" || events[1].TierUsed == nil || *events[1].TierUsed != 2 ||
		events[1].ArtifactID == nil || *events[1].ArtifactID != "art-1" || events[1].Pipeline != nil {
		t.Fatalf("unexpected COMPLETE event: %+v", events[1])
	}
	if rs.bodies["POST evadr/fetch/async"]["force_browser"] != true {
		t.Error("expected force_browser=true on the async fetch")
	}
}

func TestEvadr_JobEvents_PipelineEventsAndFailure(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"GET evadr/jobs/job-9/events": sseHandler([]map[string]interface{}{
			{"pipeline_id": "job-9", "event_type": "stage_started", "stage_id": "tier_1", "status": "running",
				"message": "Trying tier 1", "progress_pct": float64(25), "metadata": map[string]interface{}{"tier": float64(1)},
				"error": nil, "sequence": float64(3)},
			{"pipeline_id": "job-9", "event_type": "pipeline_error", "status": "failed", "sequence": float64(4),
				"error": map[string]interface{}{"error_code": "blocked", "user_message": "The site blocked every tier"}},
			{"type": "FAILED", "job_id": "job-9", "error": "blocked"},
		}),
	})
	defer server.Close()

	ch, err := newTestClient(server).Evadr.JobEvents(context.Background(), "job-9")
	if err != nil {
		t.Fatal(err)
	}
	events := drain(ch, 2*time.Second)
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %+v", events)
	}
	p := events[0].Pipeline
	if p == nil || p.EventType != "stage_started" || p.StageID != "tier_1" || p.ProgressPct == nil || *p.ProgressPct != 25 || p.Sequence != 3 || p.Message != "Trying tier 1" {
		t.Fatalf("unexpected pipeline event: %+v", p)
	}
	if events[1].Error == nil || *events[1].Error != "The site blocked every tier" || events[1].Pipeline.Error["error_code"] != "blocked" {
		t.Fatalf("unexpected pipeline error: %+v", events[1])
	}
	if events[2].Type != "FAILED" || events[2].Error == nil || *events[2].Error != "blocked" {
		t.Fatalf("unexpected FAILED event: %+v", events[2])
	}
}

func TestEvadr_Fetch_AuthRequiredAndNullHTML(t *testing.T) {
	server := httptest.NewServer(jsonHandler(200, map[string]interface{}{
		"success": true, "url": "https://example.com/account", "status_code": float64(200), "tier_used": float64(1),
		"html": nil, "auth_required": true, "discovery_id": "disc-1",
	}))
	defer server.Close()

	r, err := newTestClient(server).Evadr.Fetch(context.Background(), "https://example.com/account", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !r.AuthRequired || r.DiscoveryID == nil || *r.DiscoveryID != "disc-1" || r.HTML != nil {
		t.Fatalf("unexpected result: %+v", r)
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
