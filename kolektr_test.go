package kloakd

import (
	"context"
	"net/http"
	"testing"
)

var extractionResponse = map[string]interface{}{
	"success":       true,
	"url":           "https://example.com",
	"method":        "l1_css",
	"records":       []interface{}{map[string]interface{}{"title": "Book A", "price": "$10"}},
	"total_records": float64(1),
	"pages_scraped": float64(1),
	"artifact":      map[string]interface{}{"artifact_id": "art-ext-001"},
}

func TestKolektr_Extract(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST kolektr/extract": jsonHandler(200, extractionResponse),
	})
	defer server.Close()

	result, err := newTestClient(server).Kolektr.Extract(context.Background(), "https://example.com", &ExtractOptions{
		SchemaSpec:      map[string]interface{}{"title": "css:h3"},
		FetchArtifactID: "art-fetch-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalRecords != 1 || len(result.Records) != 1 || *result.ArtifactID != "art-ext-001" {
		t.Errorf("unexpected result: %+v", result)
	}
	body := rs.bodies["POST kolektr/extract"]
	if body["schema_spec"] == nil || body["fetch_artifact_id"] != "art-fetch-1" {
		t.Errorf("unexpected body: %v", body)
	}
	if _, ok := body["schema"]; ok {
		t.Error("schema is not a field of the request (schema_spec is)")
	}
}

func TestKolektr_Page_IsExtract(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST kolektr/extract": jsonHandler(200, extractionResponse),
	})
	defer server.Close()

	if _, err := newTestClient(server).Kolektr.Page(context.Background(), "https://example.com", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rs.seen) != 1 || rs.seen[0] != "POST kolektr/extract" {
		t.Errorf("unexpected calls: %v", rs.seen)
	}
}

func TestKolektr_ExtractHTML(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST kolektr/extract/html": jsonHandler(200, extractionResponse),
	})
	defer server.Close()

	result, err := newTestClient(server).Kolektr.ExtractHTML(context.Background(), "<html></html>", "https://example.com",
		&ExtractHTMLOptions{SchemaSpec: map[string]interface{}{"title": "css:h1"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected Success")
	}
	if rs.bodies["POST kolektr/extract/html"]["schema_spec"] == nil {
		t.Error("schema_spec not sent")
	}
}
