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

func fieldsOf(t *testing.T, body map[string]interface{}) []interface{} {
	t.Helper()
	spec, ok := body["schema_spec"].(map[string]interface{})
	if !ok {
		t.Fatalf("no schema_spec in %v", body)
	}
	fields, ok := spec["fields"].([]interface{})
	if !ok {
		t.Fatalf("schema_spec has no fields list: %v", spec)
	}
	return fields
}

func TestKolektr_SchemaShorthandIsConverted(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST kolektr/extract":      jsonHandler(200, extractionResponse),
		"POST kolektr/extract/html": jsonHandler(200, extractionResponse),
	})
	defer server.Close()
	k := newTestClient(server).Kolektr
	ctx := context.Background()

	_, err := k.Extract(ctx, "https://example.com", &ExtractOptions{
		SchemaSpec: map[string]interface{}{"title": "css:h3 a", "price": "p.price_color", "odd": "a.css:x"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fields := fieldsOf(t, rs.bodies["POST kolektr/extract"])
	want := []map[string]interface{}{
		{"name": "odd", "selector": "a.css:x"}, // only a leading "css:" is a prefix
		{"name": "price", "selector": "p.price_color"},
		{"name": "title", "selector": "h3 a"},
	}
	if len(fields) != len(want) {
		t.Fatalf("unexpected fields: %v", fields)
	}
	for i, w := range want {
		f := fields[i].(map[string]interface{})
		if f["name"] != w["name"] || f["selector"] != w["selector"] {
			t.Errorf("field %d: %v, want %v", i, f, w)
		}
	}

	// A full spec is sent as given.
	full := map[string]interface{}{"fields": []interface{}{map[string]interface{}{"name": "t", "selector": "h1", "attribute": "title"}}, "pagination": true}
	if _, err := k.ExtractHTML(ctx, "<h1>x</h1>", "https://example.com", &ExtractHTMLOptions{SchemaSpec: full}); err != nil {
		t.Fatal(err)
	}
	spec := rs.bodies["POST kolektr/extract/html"]["schema_spec"].(map[string]interface{})
	if spec["pagination"] != true || fieldsOf(t, rs.bodies["POST kolektr/extract/html"])[0].(map[string]interface{})["attribute"] != "title" {
		t.Errorf("a full spec must be sent as given: %v", spec)
	}

	// The explicit list.
	_, err = k.Extract(ctx, "https://example.com", &ExtractOptions{Fields: []SchemaField{{Name: "img", Selector: "css:img", Attribute: "src", Type: "url"}}})
	if err != nil {
		t.Fatal(err)
	}
	f := fieldsOf(t, rs.bodies["POST kolektr/extract"])[0].(map[string]interface{})
	if f["name"] != "img" || f["selector"] != "img" || f["attribute"] != "src" || f["type"] != "url" {
		t.Errorf("unexpected field: %v", f)
	}
}

func TestKolektr_ExtractionAuthRequired(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"POST kolektr/extract": jsonHandler(200, map[string]interface{}{"success": false, "url": "u", "method": "auth",
			"auth_required": true, "discovery_id": "d1"}),
	})
	defer server.Close()
	r, err := newTestClient(server).Kolektr.Extract(context.Background(), "u", nil)
	if err != nil || !r.AuthRequired || r.DiscoveryID == nil || *r.DiscoveryID != "d1" {
		t.Fatalf("unexpected: %+v %v", r, err)
	}
}

func TestKolektr_PagingParameters(t *testing.T) {
	queries := map[string]string{}
	record := func(w http.ResponseWriter, r *http.Request) {
		queries[r.Method+" "+r.URL.Path] = r.URL.RawQuery
		jsonHandler(200, map[string]interface{}{})(w, r)
	}
	endpoint := "https://api.example.com/items"
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"GET kolektr/jobs":                                   record,
		"GET kolektr/scrapers":                               record,
		"GET kolektr/content":                                record,
		"GET kolektr/api-data/" + endpoint:                   record,
		"GET kolektr/api-data/" + endpoint + "/paginated":    record,
		"POST kolektr/api-data/" + endpoint + "/extract-all": record,
	})
	defer server.Close()
	k := newTestClient(server).Kolektr
	ctx := context.Background()
	isTemplate := false
	calls := []error{}
	_, err := k.ListJobs(ctx, &ListJobsOptions{Page: 2, Limit: 50, StatusFilter: "completed"})
	calls = append(calls, err)
	_, err = k.ListScrapers(ctx, &ListScrapersOptions{Page: 3, PageSize: 10, IsTemplate: &isTemplate})
	calls = append(calls, err)
	_, err = k.ListContent(ctx, &ListContentOptions{Page: 1, PageSize: 5, Search: "books"})
	calls = append(calls, err)
	_, err = k.GetAPIData(ctx, endpoint, &APIDataOptions{EndpointHash: "h1"})
	calls = append(calls, err)
	_, err = k.GetAPIDataPaginated(ctx, endpoint, 10, 20, &APIDataOptions{EndpointHash: "h1"})
	calls = append(calls, err)
	_, err = k.ExtractAllAPIData(ctx, endpoint, &ExtractAllAPIDataOptions{EndpointHash: "h1", MaxRecords: 500})
	calls = append(calls, err)
	for i, err := range calls {
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	p := "/api/v1/organizations/" + testOrgID + "/kolektr/"
	for key, want := range map[string]string{
		"GET " + p + "jobs":                                   "limit=50&page=2&status_filter=completed",
		"GET " + p + "scrapers":                               "is_template=false&page=3&page_size=10",
		"GET " + p + "content":                                "page=1&page_size=5&search=books",
		"GET " + p + "api-data/" + endpoint:                   "endpoint_hash=h1",
		"GET " + p + "api-data/" + endpoint + "/paginated":    "endpoint_hash=h1&limit=20&offset=10",
		"POST " + p + "api-data/" + endpoint + "/extract-all": "endpoint_hash=h1&max_records=500",
	} {
		if got, ok := queries[key]; !ok || got != want {
			t.Errorf("%s: query %q, want %q", key, got, want)
		}
	}
}
