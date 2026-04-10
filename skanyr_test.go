package kloakd

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"
)

var discoverResponse = map[string]interface{}{
	"success":         true,
	"discovery_id":    "disc-001",
	"url":             "https://api.example.com",
	"total_endpoints": float64(3),
	"endpoints": []interface{}{
		map[string]interface{}{
			"url":        "https://api.example.com/users",
			"method":     "GET",
			"api_type":   "rest",
			"confidence": float64(0.98),
			"parameters": map[string]interface{}{},
		},
	},
	"has_more":    false,
	"total":       float64(3),
	"artifact_id": "art-disc-001",
	"error":       nil,
}

func TestSkanyr_Discover(t *testing.T) {
	server := httptest.NewServer(jsonHandler(200, discoverResponse))
	defer server.Close()

	client := newTestClient(server)
	result, err := client.Skanyr.Discover(context.Background(), "https://api.example.com", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected Success=true")
	}
	if result.TotalEndpoints != 3 {
		t.Errorf("expected total_endpoints=3, got %d", result.TotalEndpoints)
	}
	if len(result.Endpoints) != 1 {
		t.Errorf("expected 1 endpoint, got %d", len(result.Endpoints))
	}
	if result.Endpoints[0].ApiType != "rest" {
		t.Errorf("expected api_type=rest, got %s", result.Endpoints[0].ApiType)
	}
}

func TestSkanyr_Discover_WithSiteHierarchyArtifact(t *testing.T) {
	cap := &captureHandler{inner: jsonHandler(200, discoverResponse)}
	server := httptest.NewServer(cap)
	defer server.Close()

	client := newTestClient(server)
	_, err := client.Skanyr.Discover(context.Background(), "https://api.example.com", &DiscoverOptions{
		SiteHierarchyArtifactID: "hier-001",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cap.lastBody["site_hierarchy_artifact_id"] != "hier-001" {
		t.Errorf("unexpected site_hierarchy_artifact_id: %v", cap.lastBody["site_hierarchy_artifact_id"])
	}
}

func TestSkanyr_DiscoverAll_SinglePage(t *testing.T) {
	server := httptest.NewServer(jsonHandler(200, discoverResponse))
	defer server.Close()

	client := newTestClient(server)
	endpoints, err := client.Skanyr.DiscoverAll(context.Background(), "https://api.example.com", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(endpoints) != 1 {
		t.Errorf("expected 1 endpoint, got %d", len(endpoints))
	}
}

func TestSkanyr_DiscoverStream(t *testing.T) {
	server := httptest.NewServer(sseHandler([]map[string]interface{}{
		{"type": "endpoint_found", "endpoint_url": "https://api.example.com/v1/items", "api_type": "rest", "metadata": map[string]interface{}{}},
		{"type": "complete", "metadata": map[string]interface{}{}},
	}))
	defer server.Close()

	client := newTestClient(server)
	ch, err := client.Skanyr.DiscoverStream(context.Background(), "https://api.example.com", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	events := drain(ch, 2*time.Second)
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Type != "endpoint_found" {
		t.Errorf("expected endpoint_found, got %s", events[0].Type)
	}
	if events[0].EndpointURL == nil || *events[0].EndpointURL != "https://api.example.com/v1/items" {
		t.Errorf("unexpected EndpointURL: %v", events[0].EndpointURL)
	}
}

func TestSkanyr_DiscoverStream_WithSiteHierarchy(t *testing.T) {
	cap := &captureHandler{inner: sseHandler([]map[string]interface{}{
		{"type": "complete", "metadata": map[string]interface{}{}},
	})}
	server := httptest.NewServer(cap)
	defer server.Close()

	client := newTestClient(server)
	ch, err := client.Skanyr.DiscoverStream(context.Background(), "https://api.example.com", &DiscoverStreamOptions{
		SiteHierarchyArtifactID: "hier-001",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	drain(ch, time.Second)
	if cap.lastBody["site_hierarchy_artifact_id"] != "hier-001" {
		t.Errorf("unexpected site_hierarchy_artifact_id: %v", cap.lastBody["site_hierarchy_artifact_id"])
	}
}

func TestSkanyr_GetApiMap(t *testing.T) {
	server := httptest.NewServer(jsonHandler(200, map[string]interface{}{"artifact_id": "art-disc-001"}))
	defer server.Close()

	client := newTestClient(server)
	result, err := client.Skanyr.GetApiMap(context.Background(), "art-disc-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["artifact_id"] != "art-disc-001" {
		t.Errorf("unexpected result: %v", result)
	}
}

func TestSkanyr_GetJob(t *testing.T) {
	server := httptest.NewServer(jsonHandler(200, map[string]interface{}{"job_id": "job-disc-001", "status": "completed"}))
	defer server.Close()

	client := newTestClient(server)
	result, err := client.Skanyr.GetJob(context.Background(), "job-disc-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["status"] != "completed" {
		t.Errorf("unexpected status: %v", result["status"])
	}
}
