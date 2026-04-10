package kloakd

import (
	"context"
	"net/http/httptest"
	"testing"
)

var extractionResponse = map[string]interface{}{
	"success":       true,
	"url":           "https://example.com",
	"method":        "l1_css",
	"records":       []interface{}{map[string]interface{}{"title": "Book A", "price": "$10"}},
	"total_records": float64(1),
	"pages_scraped": float64(1),
	"has_more":      false,
	"total":         float64(1),
	"artifact_id":   "art-ext-001",
	"job_id":        nil,
	"error":         nil,
}

func TestKolektr_Page(t *testing.T) {
	server := httptest.NewServer(jsonHandler(200, extractionResponse))
	defer server.Close()

	client := newTestClient(server)
	result, err := client.Kolektr.Page(context.Background(), "https://example.com", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected Success=true")
	}
	if result.Method != "l1_css" {
		t.Errorf("expected method=l1_css, got %s", result.Method)
	}
	if result.TotalRecords != 1 {
		t.Errorf("expected 1 record, got %d", result.TotalRecords)
	}
	if result.ArtifactID == nil || *result.ArtifactID != "art-ext-001" {
		t.Errorf("unexpected ArtifactID: %v", result.ArtifactID)
	}
}

func TestKolektr_Page_WithSchema(t *testing.T) {
	cap := &captureHandler{inner: jsonHandler(200, extractionResponse)}
	server := httptest.NewServer(cap)
	defer server.Close()

	client := newTestClient(server)
	_, err := client.Kolektr.Page(context.Background(), "https://example.com", &PageOptions{
		Schema: map[string]interface{}{
			"title": "css:h3 a",
			"price": "css:p.price_color",
		},
		FetchArtifactID: "art-fetch-001",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := cap.lastBody["schema"]; !ok {
		t.Error("expected schema in request body")
	}
	if cap.lastBody["fetch_artifact_id"] != "art-fetch-001" {
		t.Errorf("unexpected fetch_artifact_id: %v", cap.lastBody["fetch_artifact_id"])
	}
}

func TestKolektr_PageAll_SinglePage(t *testing.T) {
	server := httptest.NewServer(jsonHandler(200, extractionResponse))
	defer server.Close()

	client := newTestClient(server)
	records, err := client.Kolektr.PageAll(context.Background(), "https://example.com", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("expected 1 record, got %d", len(records))
	}
	if records[0]["title"] != "Book A" {
		t.Errorf("unexpected title: %v", records[0]["title"])
	}
}

func TestKolektr_ExtractHTML(t *testing.T) {
	cap := &captureHandler{inner: jsonHandler(200, map[string]interface{}{
		"success":       true,
		"url":           "https://example.com",
		"method":        "l1_css",
		"records":       []interface{}{map[string]interface{}{"price": "$12"}},
		"total_records": float64(1),
		"pages_scraped": float64(0),
		"has_more":      false,
		"total":         float64(1),
	})}
	server := httptest.NewServer(cap)
	defer server.Close()

	client := newTestClient(server)
	result, err := client.Kolektr.ExtractHTML(
		context.Background(),
		"<html><p class='price'>$12</p></html>",
		"https://example.com",
		&ExtractHTMLOptions{Schema: map[string]interface{}{"price": "css:p.price"}},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalRecords != 1 {
		t.Errorf("expected 1 record, got %d", result.TotalRecords)
	}
	if cap.lastBody["html"] == "" {
		t.Error("expected html in request body")
	}
	if cap.lastBody["url"] != "https://example.com" {
		t.Errorf("unexpected url: %v", cap.lastBody["url"])
	}
}
