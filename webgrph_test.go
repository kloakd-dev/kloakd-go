package kloakd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

var crawlResponse = map[string]interface{}{
	"success":          true,
	"crawl_id":         "crawl-001",
	"url":              "https://example.com",
	"total_pages":      float64(10),
	"max_depth_reached": float64(2),
	"pages": []interface{}{
		map[string]interface{}{
			"url":         "https://example.com",
			"depth":       float64(0),
			"title":       "Home",
			"status_code": float64(200),
			"children":    []interface{}{"https://example.com/about"},
		},
	},
	"has_more":    false,
	"total":       float64(10),
	"artifact_id": "art-crawl-001",
	"error":       nil,
}

func TestWebgrph_Crawl(t *testing.T) {
	server := httptest.NewServer(jsonHandler(200, crawlResponse))
	defer server.Close()

	client := newTestClient(server)
	result, err := client.Webgrph.Crawl(context.Background(), "https://example.com", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected Success=true")
	}
	if result.CrawlID != "crawl-001" {
		t.Errorf("expected crawl_id=crawl-001, got %s", result.CrawlID)
	}
	if result.TotalPages != 10 {
		t.Errorf("expected total_pages=10, got %d", result.TotalPages)
	}
	if len(result.Pages) != 1 {
		t.Errorf("expected 1 page node, got %d", len(result.Pages))
	}
	if result.ArtifactID == nil || *result.ArtifactID != "art-crawl-001" {
		t.Errorf("unexpected ArtifactID: %v", result.ArtifactID)
	}
}

func TestWebgrph_Crawl_WithOptions(t *testing.T) {
	cap := &captureHandler{inner: jsonHandler(200, crawlResponse)}
	server := httptest.NewServer(cap)
	defer server.Close()

	client := newTestClient(server)
	_, err := client.Webgrph.Crawl(context.Background(), "https://example.com", &CrawlOptions{
		MaxDepth:          3,
		MaxPages:          100,
		SessionArtifactID: "sess-001",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cap.lastBody["max_depth"] != float64(3) {
		t.Errorf("unexpected max_depth: %v", cap.lastBody["max_depth"])
	}
	if cap.lastBody["session_artifact_id"] != "sess-001" {
		t.Errorf("unexpected session_artifact_id: %v", cap.lastBody["session_artifact_id"])
	}
}

func TestWebgrph_CrawlAll_SinglePage(t *testing.T) {
	server := httptest.NewServer(jsonHandler(200, crawlResponse))
	defer server.Close()

	client := newTestClient(server)
	pages, err := client.Webgrph.CrawlAll(context.Background(), "https://example.com", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pages) != 1 {
		t.Errorf("expected 1 page, got %d", len(pages))
	}
}

func TestWebgrph_CrawlAll_Paginates(t *testing.T) {
	calls := 0
	handler := jsonHandlerSeq([]interface{}{
		map[string]interface{}{
			"success": true, "crawl_id": "c1", "url": "https://example.com",
			"total_pages": float64(2), "max_depth_reached": float64(1),
			"pages": []interface{}{
				map[string]interface{}{"url": "https://example.com/p1", "depth": float64(0), "children": []interface{}{}},
			},
			"has_more": true, "total": float64(2),
		},
		map[string]interface{}{
			"success": true, "crawl_id": "c1", "url": "https://example.com",
			"total_pages": float64(2), "max_depth_reached": float64(1),
			"pages": []interface{}{
				map[string]interface{}{"url": "https://example.com/p2", "depth": float64(1), "children": []interface{}{}},
			},
			"has_more": false, "total": float64(2),
		},
	}, &calls)
	server := httptest.NewServer(handler)
	defer server.Close()

	client := newTestClient(server)
	pages, err := client.Webgrph.CrawlAll(context.Background(), "https://example.com", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pages) != 2 {
		t.Errorf("expected 2 pages, got %d", len(pages))
	}
	if calls != 2 {
		t.Errorf("expected 2 API calls, got %d", calls)
	}
}

func TestWebgrph_CrawlStream(t *testing.T) {
	server := httptest.NewServer(sseHandler([]map[string]interface{}{
		{"type": "page_discovered", "url": "https://example.com/a", "depth": float64(1), "metadata": map[string]interface{}{}},
		{"type": "complete", "pages_found": float64(5), "metadata": map[string]interface{}{}},
	}))
	defer server.Close()

	client := newTestClient(server)
	ch, err := client.Webgrph.CrawlStream(context.Background(), "https://example.com", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	events := drain(ch, 2*time.Second)
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Type != "page_discovered" {
		t.Errorf("expected page_discovered, got %s", events[0].Type)
	}
	if events[0].URL == nil || *events[0].URL != "https://example.com/a" {
		t.Errorf("unexpected URL in event: %v", events[0].URL)
	}
}

func TestWebgrph_GetHierarchy(t *testing.T) {
	cap := &captureHandler{inner: jsonHandler(200, map[string]interface{}{"artifact_id": "art-001"})}
	server := httptest.NewServer(cap)
	defer server.Close()

	client := newTestClient(server)
	result, err := client.Webgrph.GetHierarchy(context.Background(), "art-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["artifact_id"] != "art-001" {
		t.Errorf("unexpected artifact_id: %v", result["artifact_id"])
	}
}

func TestWebgrph_GetJob(t *testing.T) {
	server := httptest.NewServer(jsonHandler(200, map[string]interface{}{"job_id": "job-001", "status": "running"}))
	defer server.Close()

	client := newTestClient(server)
	result, err := client.Webgrph.GetJob(context.Background(), "job-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["status"] != "running" {
		t.Errorf("unexpected status: %v", result["status"])
	}
}

// jsonHandlerSeq returns sequential responses from a slice, advancing on each call.
func jsonHandlerSeq(responses []interface{}, calls *int) *captureHandler {
	return &captureHandler{
		inner: func(w http.ResponseWriter, r *http.Request) {
			idx := *calls
			if idx >= len(responses) {
				idx = len(responses) - 1
			}
			*calls = idx + 1
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			b, _ := json.Marshal(responses[idx])
			w.Write(b)
		},
	}
}
