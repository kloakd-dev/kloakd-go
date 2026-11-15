package kloakd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Tests for cost control (COST-CONTROL-CONTRACT.md, 15 Nov 2026): the new
// request fields, the estimate's data fields, the status fields, MapSite,
// the artifact routes and the 409 cost_acknowledgement_required error.

func costEstimateBody() map[string]interface{} {
	b := estimateBody("crawl")
	b["include_resources"] = []interface{}{"images"}
	b["data"] = map[string]interface{}{
		"source":                 "root_page",
		"per_page_mb":            map[string]interface{}{"typical": 1.2, "high": 4.0},
		"total_mb":               map[string]interface{}{"low": 3.0, "typical": 12.0, "high": 40.0},
		"total_mb_all_resources": map[string]interface{}{"typical": 30.0, "high": 120.0},
		"saving_mb":              map[string]interface{}{"typical": 18.0, "high": 80.0},
		"by_category_mb_per_page": map[string]interface{}{"document": 0.3, "script": 0.6, "data": 0.1, "images": 1.1,
			"media": 0.0, "fonts": 0.1, "stylesheets": 0.08, "tracking": 0.05},
		"saving_by_category_mb": map[string]interface{}{"images": 11.0, "media": 0.0, "fonts": 1.0,
			"stylesheets": 0.8, "tracking": 0.5},
	}
	b["share_of_remaining"] = map[string]interface{}{"typical": 0.01, "high": 0.04}
	b["consent_required"] = true
	b["risks"] = []interface{}{"Some sites block automated visitors."}
	b["suggested_max_data_mb"] = 40.0
	b["estimate_id"] = "est.abc"
	return b
}

func TestWebgrph_EstimateCrawl_ReadsTheCostFieldsAndSendsIncludeResources(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST webgrph/crawl/estimate":         jsonHandler(200, costEstimateBody()),
		"POST webgrph/crawl-extract/estimate": jsonHandler(200, costEstimateBody()),
	})
	defer server.Close()
	w := newTestClient(server).Webgrph

	est, err := w.EstimateCrawl(context.Background(), "https://example.com/",
		&CrawlEstimateOptions{IncludeResources: []string{ResourceImages}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body := rs.bodies["POST webgrph/crawl/estimate"]
	if !reflect.DeepEqual(body["include_resources"], []interface{}{"images"}) || len(body) != 2 {
		t.Errorf("unexpected body: %v", body)
	}
	if !reflect.DeepEqual(est.IncludeResources, []string{"images"}) || !est.ConsentRequired ||
		!reflect.DeepEqual(est.Risks, []string{"Some sites block automated visitors."}) ||
		est.SuggestedMaxDataMB == nil || *est.SuggestedMaxDataMB != 40 || est.EstimateID != "est.abc" {
		t.Errorf("unexpected cost fields: %+v", est)
	}
	if est.ShareOfRemaining == nil || est.ShareOfRemaining.Typical != 0.01 || est.ShareOfRemaining.High != 0.04 ||
		est.ShareOfRemaining.Low != nil {
		t.Errorf("unexpected share: %+v", est.ShareOfRemaining)
	}
	d := est.Data
	if d == nil || d.Source != "root_page" {
		t.Fatalf("unexpected data: %+v", d)
	}
	if d.TotalMB.Low == nil || *d.TotalMB.Low != 3 || d.TotalMB.Typical != 12 || d.TotalMB.High != 40 {
		t.Errorf("unexpected total: %+v", d.TotalMB)
	}
	if d.PerPageMB.Low != nil || d.PerPageMB.Typical != 1.2 || d.PerPageMB.High != 4 ||
		d.TotalMBAllResources.Typical != 30 || d.TotalMBAllResources.High != 120 ||
		d.SavingMB.Typical != 18 || d.SavingMB.High != 80 {
		t.Errorf("unexpected ranges: %+v", d)
	}
	if len(d.ByCategoryMBPerPage) != 8 || d.ByCategoryMBPerPage["images"] != 1.1 || d.ByCategoryMBPerPage["stylesheets"] != 0.08 {
		t.Errorf("unexpected by category: %v", d.ByCategoryMBPerPage)
	}
	if len(d.SavingByCategoryMB) != 5 || d.SavingByCategoryMB["images"] != 11 || d.SavingByCategoryMB["tracking"] != 0.5 {
		t.Errorf("unexpected saving by category: %v", d.SavingByCategoryMB)
	}

	if _, err := w.EstimateCrawlExtract(context.Background(), "https://example.com/",
		&CrawlExtractEstimateOptions{IncludeResources: []string{ResourceFonts, ResourceTracking}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := rs.bodies["POST webgrph/crawl-extract/estimate"]["include_resources"]; !reflect.DeepEqual(got, []interface{}{"fonts", "tracking"}) {
		t.Errorf("unexpected include_resources: %v", got)
	}
}

func TestWebgrph_EstimateCrawl_WithoutTheCostFieldsTheyAreZero(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"POST webgrph/crawl/estimate": jsonHandler(200, estimateBody("crawl")),
	})
	defer server.Close()

	est, err := newTestClient(server).Webgrph.EstimateCrawl(context.Background(), "https://example.com/",
		&CrawlEstimateOptions{IncludeResources: []string{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.Data != nil || est.ShareOfRemaining != nil || est.ConsentRequired || est.Risks != nil ||
		est.IncludeResources != nil || est.SuggestedMaxDataMB != nil || est.EstimateID != "" {
		t.Errorf("absent cost fields must be zero: %+v", est)
	}
}

var costKeys = []string{"include_resources", "max_data_mb", "estimate_id", "cost_acknowledged", "urls", "include_paths"}

func TestWebgrph_StartCrawl_SendsTheCostFieldsOnlyWhenSet(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST webgrph/crawl": jsonHandler(202, map[string]interface{}{"success": true, "crawl_id": "c1"}),
	})
	defer server.Close()
	w := newTestClient(server).Webgrph

	if _, err := w.StartCrawl(context.Background(), "https://example.com/", &CrawlOptions{MaxPages: 10}); err != nil {
		t.Fatal(err)
	}
	for _, k := range costKeys {
		if _, ok := rs.bodies["POST webgrph/crawl"][k]; ok {
			t.Errorf("%s sent without being set", k)
		}
	}

	_, err := w.StartCrawl(context.Background(), "https://example.com/", &CrawlOptions{
		IncludeResources: []string{ResourceImages, ResourceMedia}, MaxDataMB: 40.5, EstimateID: "est.abc",
		CostAcknowledged: true, URLs: []string{"https://example.com/a", "https://example.com/b"},
		IncludePaths: []string{"/products/"},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := rs.bodies["POST webgrph/crawl"]
	want := map[string]interface{}{
		"url":               "https://example.com/",
		"include_resources": []interface{}{"images", "media"},
		"max_data_mb":       40.5,
		"estimate_id":       "est.abc",
		"cost_acknowledged": true,
		"urls":              []interface{}{"https://example.com/a", "https://example.com/b"},
		"include_paths":     []interface{}{"/products/"},
	}
	if !reflect.DeepEqual(body, want) {
		t.Errorf("unexpected body: %v", body)
	}
}

func TestWebgrph_StartCrawlExtract_SendsTheCostFieldsOnlyWhenSet(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST webgrph/crawl-extract": jsonHandler(202, map[string]interface{}{"success": true, "job_id": "j1"}),
	})
	defer server.Close()
	w := newTestClient(server).Webgrph

	if _, err := w.StartCrawlExtract(context.Background(), "https://example.com/", nil); err != nil {
		t.Fatal(err)
	}
	if body := rs.bodies["POST webgrph/crawl-extract"]; len(body) != 1 {
		t.Errorf("an old call must send only the url: %v", body)
	}
	_, err := w.StartCrawlExtract(context.Background(), "https://example.com/", &CrawlExtractOptions{
		MaxDataMB: 12, EstimateID: "est.x", URLs: []string{"https://example.com/"},
		IncludePaths: []string{"/blog/"}, IncludeResources: []string{ResourceStylesheets},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := rs.bodies["POST webgrph/crawl-extract"]
	if body["max_data_mb"] != float64(12) || body["estimate_id"] != "est.x" ||
		!reflect.DeepEqual(body["urls"], []interface{}{"https://example.com/"}) ||
		!reflect.DeepEqual(body["include_paths"], []interface{}{"/blog/"}) ||
		!reflect.DeepEqual(body["include_resources"], []interface{}{"stylesheets"}) {
		t.Errorf("unexpected body: %v", body)
	}
	if _, ok := body["cost_acknowledged"]; ok {
		t.Errorf("cost_acknowledged false must not be sent: %v", body)
	}
}

func TestWebgrph_Continue_SendsMaxDataMB(t *testing.T) {
	var got []byte
	var seen bool
	var rs *routeServer
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST webgrph/crawl/c1/continue": rawBodyHandler(&rs, "POST webgrph/crawl/c1/continue", &got, &seen, 202,
			map[string]interface{}{"success": true, "crawl_id": "c1", "status": "queued"}),
		"POST webgrph/crawl-extract/j1/continue": rawBodyHandler(&rs, "POST webgrph/crawl-extract/j1/continue", &got, &seen, 202,
			map[string]interface{}{"success": true, "job_id": "j1", "status": "queued"}),
	})
	defer server.Close()
	w := newTestClient(server).Webgrph

	if _, err := w.ContinueCrawl(context.Background(), "c1", &ContinueOptions{MaxDataMB: 200}); err != nil || string(got) != `{"max_data_mb":200}` {
		t.Errorf("unexpected: err=%v body=%s", err, got)
	}
	if _, err := w.ContinueCrawlExtract(context.Background(), "j1", &ContinueOptions{MaxPages: 50, MaxDataMB: 99.5}); err != nil ||
		string(got) != `{"max_data_mb":99.5,"max_pages":50}` {
		t.Errorf("unexpected: err=%v body=%s", err, got)
	}
	if _, err := w.ContinueCrawl(context.Background(), "c1"); err != nil || len(got) != 0 {
		t.Errorf("no options must still send no body: err=%v body=%s", err, got)
	}
}

func TestCrawlStatus_DataFields(t *testing.T) {
	r := crawlResultFromStatus("c1", map[string]interface{}{
		"status": "completed", "is_partial": true, "partial_reason": "data_limit",
		"data_used_mb": 40.2, "max_data_mb": 40.0, "include_resources": []interface{}{},
		"message": "Stopped at your data limit of 40 MB. Continue with a higher limit to fetch more.",
	})
	if r.DataUsedMB == nil || *r.DataUsedMB != 40.2 || r.MaxDataMB == nil || *r.MaxDataMB != 40 ||
		r.IncludeResources == nil || len(r.IncludeResources) != 0 {
		t.Errorf("unexpected data fields: %+v", r)
	}
	if r.PartialReason == nil || *r.PartialReason != PartialReasonDataLimit {
		t.Errorf("unexpected partial reason: %v", r.PartialReason)
	}
	r = crawlResultFromStatus("c1", map[string]interface{}{"status": "running", "max_data_mb": nil})
	if r.DataUsedMB != nil || r.MaxDataMB != nil || r.IncludeResources != nil {
		t.Errorf("null or absent must be nil: %+v", r)
	}
}

func TestCrawlExtractStatus_DataFields(t *testing.T) {
	raw := ceStatus("completed")
	raw["data_used_mb"], raw["max_data_mb"] = 3.5, 10.0
	raw["include_resources"] = []interface{}{"images", "fonts"}
	s := parseCrawlExtractStatus(raw)
	if s.DataUsedMB == nil || *s.DataUsedMB != 3.5 || s.MaxDataMB == nil || *s.MaxDataMB != 10 ||
		!reflect.DeepEqual(s.IncludeResources, []string{"images", "fonts"}) {
		t.Errorf("unexpected data fields: %+v", s)
	}
	s = parseCrawlExtractStatus(ceStatus("crawling"))
	if s.DataUsedMB != nil || s.MaxDataMB != nil || s.IncludeResources != nil {
		t.Errorf("absent must be nil: %+v", s)
	}
}

func TestWebgrph_MapSite_SendsTheBodyAndReadsTheTree(t *testing.T) {
	answer := map[string]interface{}{
		"url": "https://example.com/", "source": "both", "total_urls": float64(1234), "truncated": true,
		"data_used_mb": 0.8,
		"branches": []interface{}{
			map[string]interface{}{"path": "/products/", "url_count": float64(800),
				"sample_urls": []interface{}{"https://example.com/products/a"},
				"children": []interface{}{
					map[string]interface{}{"path": "/products/shoes/", "url_count": float64(500),
						"sample_urls": []interface{}{}, "children": []interface{}{}},
				}},
			map[string]interface{}{"path": "other", "url_count": float64(34), "sample_urls": []interface{}{}},
		},
	}
	rs, server := newRouteServer(map[string]http.HandlerFunc{"POST webgrph/map": jsonHandler(200, answer)})
	defer server.Close()
	w := newTestClient(server).Webgrph

	m, err := w.MapSite(context.Background(), "https://example.com/", &MapSiteOptions{MaxURLs: 5000})
	if err != nil {
		t.Fatal(err)
	}
	body := rs.bodies["POST webgrph/map"]
	if body["url"] != "https://example.com/" || body["max_urls"] != float64(5000) || len(body) != 2 {
		t.Errorf("unexpected body: %v", body)
	}
	if m.URL != "https://example.com/" || m.Source != "both" || m.TotalURLs != 1234 || !m.Truncated ||
		m.DataUsedMB != 0.8 || m.Raw["source"] != "both" {
		t.Errorf("unexpected map: %+v", m)
	}
	want := []SiteMapBranch{
		{Path: "/products/", URLCount: 800, SampleURLs: []string{"https://example.com/products/a"},
			Children: []SiteMapBranch{{Path: "/products/shoes/", URLCount: 500, SampleURLs: []string{}, Children: []SiteMapBranch{}}}},
		{Path: "other", URLCount: 34, SampleURLs: []string{}, Children: []SiteMapBranch{}},
	}
	if !reflect.DeepEqual(m.Branches, want) {
		t.Errorf("unexpected branches: %+v", m.Branches)
	}

	if _, err := w.MapSite(context.Background(), "https://example.com/", nil); err != nil {
		t.Fatal(err)
	}
	if body := rs.bodies["POST webgrph/map"]; len(body) != 1 {
		t.Errorf("no options must send only the url: %v", body)
	}
}

const artID = "5f0c1d2e-3a4b-4c5d-8e9f-0a1b2c3d4e5f"

func TestClient_GetArtifact(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"GET artifacts/" + artID: jsonHandler(200, map[string]interface{}{
			"id": artID, "type": "FETCHED_CONTENT", "url": "https://example.com/", "created_at": "2026-11-15T10:00:00Z",
			"size_bytes": float64(5120), "content_type": "text/html; charset=utf-8",
			"final_url": "https://www.example.com/", "status_code": float64(200), "verified": true,
			"content_url": "/api/v1/organizations/org/artifacts/" + artID + "/content",
		}),
		"GET artifacts/" + artID[:35] + "0": jsonHandler(200, map[string]interface{}{
			"id": artID[:35] + "0", "type": "DEGRADED_FETCHED_CONTENT", "url": "https://example.com/",
			"created_at": "2026-11-15T10:00:00Z", "size_bytes": float64(10), "content_type": "text/html; charset=utf-8",
			"final_url": nil, "status_code": nil, "verified": false, "content_url": "/x/content",
		}),
	})
	defer server.Close()
	client := newTestClient(server)

	a, err := client.GetArtifact(context.Background(), artID)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != artID || a.Type != ArtifactFetchedContent || *a.URL != "https://example.com/" ||
		*a.CreatedAt != "2026-11-15T10:00:00Z" || *a.SizeBytes != 5120 || *a.ContentType != "text/html; charset=utf-8" ||
		*a.FinalURL != "https://www.example.com/" || *a.StatusCode != 200 || !a.Verified ||
		a.ContentURL != "/api/v1/organizations/org/artifacts/"+artID+"/content" {
		t.Errorf("unexpected artifact: %+v", a)
	}
	d, err := client.GetArtifact(context.Background(), artID[:35]+"0")
	if err != nil {
		t.Fatal(err)
	}
	if d.Type != ArtifactDegradedFetchedContent || d.FinalURL != nil || d.StatusCode != nil || d.Verified || d.ContentURL != "/x/content" {
		t.Errorf("unexpected degraded artifact: %+v", d)
	}
	if len(rs.seen) != 2 {
		t.Errorf("unexpected calls: %v", rs.seen)
	}
}

func TestClient_DownloadAndSaveArtifact(t *testing.T) {
	const html = "<html><body>hi</body></html>"
	const filename = "example.com-5f0c1d2e.html"
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"GET artifacts/" + artID + "/content": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
			w.Header().Set("X-Content-Type-Options", "nosniff")
			_, _ = w.Write([]byte(html))
		},
	})
	defer server.Close()
	client := newTestClient(server)

	dl, err := client.DownloadArtifact(context.Background(), artID)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(dl.Body)
	dl.Body.Close()
	if string(got) != html || dl.Filename != filename || dl.ContentType != "text/html; charset=utf-8" ||
		dl.JobStatus != "" || dl.Crawl != nil {
		t.Errorf("unexpected download: %+v %q", dl, got)
	}

	target := filepath.Join(t.TempDir(), "page.html")
	saved, err := client.SaveArtifact(context.Background(), artID, target)
	if err != nil {
		t.Fatal(err)
	}
	file, _ := os.ReadFile(target)
	if string(file) != html || saved.Bytes != int64(len(html)) || saved.Filename != filename {
		t.Errorf("unexpected save: %+v %q", saved, file)
	}
}

func TestClient_Artifact_IdIsOnePathSegment(t *testing.T) {
	var rawPath string
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"GET artifacts/a/1/content": func(w http.ResponseWriter, r *http.Request) {
			rawPath = r.URL.RawPath
			_, _ = w.Write([]byte("x"))
		},
	})
	defer server.Close()
	dl, err := newTestClient(server).DownloadArtifact(context.Background(), "a/1")
	if err != nil {
		t.Fatal(err)
	}
	dl.Body.Close()
	if rawPath != "/api/v1/organizations/"+testOrgID+"/artifacts/a%2F1/content" {
		t.Errorf("the id must be escaped as one path segment: %q", rawPath)
	}
}

func TestClient_Artifact_Errors(t *testing.T) {
	notFound := jsonHandler(404, map[string]interface{}{"detail": "Artifact not found"})
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"GET artifacts/" + artID:              notFound,
		"GET artifacts/" + artID + "/content": notFound,
		"GET artifacts/not-a-uuid": jsonHandler(422, map[string]interface{}{"detail": []interface{}{
			map[string]interface{}{"loc": []interface{}{"path", "artifact_id"}, "msg": "Input should be a valid UUID",
				"type": "uuid_parsing"}}}),
		"GET artifacts/not-a-uuid/content": jsonHandler(422, map[string]interface{}{"detail": []interface{}{
			map[string]interface{}{"loc": []interface{}{"path", "artifact_id"}, "msg": "Input should be a valid UUID",
				"type": "uuid_parsing"}}}),
		"GET artifacts/other-org": jsonHandler(403, map[string]interface{}{"detail": "Access denied"}),
	})
	defer server.Close()
	client := newTestClient(server)

	var apiErr *ApiError
	if _, err := client.GetArtifact(context.Background(), artID); !errorsAs(err, &apiErr) || apiErr.StatusCode != 404 ||
		apiErr.Message != "Artifact not found" {
		t.Errorf("expected a 404 Artifact not found, got %v", err)
	}
	if _, err := client.DownloadArtifact(context.Background(), artID); !errorsAs(err, &apiErr) || apiErr.StatusCode != 404 {
		t.Errorf("expected a 404, got %v", err)
	}
	for _, call := range []func() error{
		func() error { _, err := client.GetArtifact(context.Background(), "not-a-uuid"); return err },
		func() error { _, err := client.DownloadArtifact(context.Background(), "not-a-uuid"); return err },
	} {
		if err := call(); !errorsAs(err, &apiErr) || apiErr.StatusCode != 422 {
			t.Errorf("expected a 422 ApiError, got %v", err)
		}
	}
	var forbidden *ForbiddenError
	if _, err := client.GetArtifact(context.Background(), "other-org"); !errorsAs(err, &forbidden) || forbidden.StatusCode != 403 {
		t.Errorf("expected a 403 ForbiddenError, got %v", err)
	}
}

func TestErrors_CostAcknowledgementRequired(t *testing.T) {
	calls := 0
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"POST webgrph/crawl": func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(409)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"detail": map[string]interface{}{"error": "cost_acknowledgement_required",
					"message": "Accept the estimate before starting."}})
		},
	})
	defer server.Close()

	_, err := newTestClient(server).Webgrph.StartCrawl(context.Background(), "https://example.com/", nil)
	var ackErr *CostAcknowledgementRequiredError
	if !errorsAs(err, &ackErr) || ackErr.StatusCode != 409 || ackErr.Code != "cost_acknowledgement_required" ||
		ackErr.Message != "Accept the estimate before starting." {
		t.Fatalf("expected a CostAcknowledgementRequiredError, got %v", err)
	}
	if calls != 1 {
		t.Errorf("must not be retried: %d calls", calls)
	}
	var apiErr *ApiError
	if errorsAs(err, &apiErr) {
		t.Errorf("must not also be an ApiError")
	}
	// The flat {"error": ...} shape (no detail) is the same error.
	if e := raiseForStatus(409, map[string]interface{}{"error": "cost_acknowledgement_required"}, nil, nil); e == nil {
		t.Fatal("expected an error")
	} else if !errorsAs(e, &ackErr) {
		t.Errorf("flat body: got %T", e)
	}
}

func TestContract_CostControlRoutesAreCalled(t *testing.T) {
	live, calls := liveRoutes(t), sdkCalls(t)
	for _, key := range []string{"POST webgrph/map", "GET artifacts/{}", "GET artifacts/{}/content"} {
		if !live[key] {
			t.Errorf("%s missing from testdata/routes.tsv", key)
		}
		if !calls[key] {
			t.Errorf("the SDK does not call %s", key)
		}
	}
}
