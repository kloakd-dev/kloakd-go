package kloakd

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// Tests for the crawl estimate, continuing with options, and the time
// budget on the status bodies (CRAWL-ESTIMATE-CONTRACT.md, 14 Nov 2026).

func estimateBody(mode string) map[string]interface{} {
	return map[string]interface{}{
		"mode": mode, "url": "https://example.com/", "max_depth": float64(2), "max_pages": float64(10),
		"plan_limits": map[string]interface{}{"max_depth": float64(5), "max_pages": float64(500), "plan_tier": "free"},
		"clamped":     true,
		"site": map[string]interface{}{
			"reachable": true, "sitemap_pages": float64(1234), "homepage_links": nil, "probe_error": nil,
		},
		"est_pages":         map[string]interface{}{"low": float64(10), "high": float64(10), "source": "sitemap"},
		"will_hit_page_cap": true, "time_budget_s": float64(120),
		"est_duration_s": map[string]interface{}{"low": float64(40), "high": 120.5},
		"est_gb":         0.03, "gb_remaining": 4.2, "will_exhaust_quota": false,
		"warning": "This site has more pages than your limit.", "explanation": "A crawl maps the site.",
	}
}

func TestWebgrph_EstimateCrawl_SendsTheBodyAndReadsTheEstimate(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST webgrph/crawl/estimate": jsonHandler(200, estimateBody("crawl")),
	})
	defer server.Close()

	est, err := newTestClient(server).Webgrph.EstimateCrawl(context.Background(), "https://example.com/",
		&CrawlEstimateOptions{MaxDepth: 9, MaxPages: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body := rs.bodies["POST webgrph/crawl/estimate"]
	if body["url"] != "https://example.com/" || body["max_depth"] != float64(9) || body["max_pages"] != float64(10) || len(body) != 3 {
		t.Errorf("unexpected body: %v", body)
	}
	if est.Mode != "crawl" || est.URL != "https://example.com/" || est.MaxDepth != 2 || est.MaxPages != 10 || !est.Clamped {
		t.Errorf("unexpected estimate: %+v", est)
	}
	if est.PlanLimits != (CrawlEstimatePlanLimits{MaxDepth: 5, MaxPages: 500, PlanTier: "free"}) {
		t.Errorf("unexpected plan limits: %+v", est.PlanLimits)
	}
	if !est.Site.Reachable || est.Site.SitemapPages == nil || *est.Site.SitemapPages != 1234 ||
		est.Site.HomepageLinks != nil || est.Site.ProbeError != nil {
		t.Errorf("unexpected site: %+v", est.Site)
	}
	if est.EstPages != (CrawlEstimatePages{Low: 10, High: 10, Source: "sitemap"}) || !est.WillHitPageCap || est.TimeBudgetS != 120 {
		t.Errorf("unexpected pages or budget: %+v", est)
	}
	if est.EstDurationS != (CrawlEstimateDuration{Low: 40, High: 120.5}) || est.EstGB != 0.03 ||
		est.GBRemaining == nil || *est.GBRemaining != 4.2 || est.WillExhaustQuota {
		t.Errorf("unexpected duration or data: %+v", est)
	}
	if est.Warning == nil || *est.Warning != "This site has more pages than your limit." || est.Explanation != "A crawl maps the site." {
		t.Errorf("unexpected warning or explanation: %+v", est)
	}
	if est.Raw["mode"] != "crawl" {
		t.Errorf("Raw not kept")
	}
}

func TestWebgrph_EstimateCrawl_NoOptionsSendsOnlyTheURL(t *testing.T) {
	b := estimateBody("crawl")
	b["gb_remaining"], b["warning"] = nil, nil
	b["est_pages"] = map[string]interface{}{"low": float64(1), "high": float64(10), "source": "limit"}
	rs, server := newRouteServer(map[string]http.HandlerFunc{"POST webgrph/crawl/estimate": jsonHandler(200, b)})
	defer server.Close()

	est, err := newTestClient(server).Webgrph.EstimateCrawl(context.Background(), "https://example.com/", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body := rs.bodies["POST webgrph/crawl/estimate"]; len(body) != 1 || body["url"] != "https://example.com/" {
		t.Errorf("unexpected body: %v", body)
	}
	if est.GBRemaining != nil || est.Warning != nil || est.EstPages.Source != "limit" {
		t.Errorf("nulls must stay nil: %+v", est)
	}
}

func TestWebgrph_EstimateCrawlExtract_SendsTheExtractSpec(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST webgrph/crawl-extract/estimate": jsonHandler(200, estimateBody("crawl_extract")),
	})
	defer server.Close()

	est, err := newTestClient(server).Webgrph.EstimateCrawlExtract(context.Background(), "https://example.com/",
		&CrawlExtractEstimateOptions{MaxPages: 10, Extract: &CrawlExtractSpec{EntityType: "product"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body := rs.bodies["POST webgrph/crawl-extract/estimate"]
	extract, _ := body["extract"].(map[string]interface{})
	if body["max_pages"] != float64(10) || body["max_depth"] != nil || extract["entity_type"] != "product" {
		t.Errorf("unexpected body: %v", body)
	}
	if est.Mode != "crawl_extract" {
		t.Errorf("unexpected mode: %q", est.Mode)
	}
}

func TestWebgrph_EstimateCrawl_AnErrorIsAnApiError(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"POST webgrph/crawl/estimate": jsonHandler(400, map[string]interface{}{"detail": "bad url"}),
	})
	defer server.Close()

	_, err := newTestClient(server).Webgrph.EstimateCrawl(context.Background(), "nope", nil)
	if err == nil {
		t.Fatal("expected an error")
	}
}

// rawBodyHandler records the request's body length, to tell no body from
// {} (the routeServer has already decoded the body into rs.bodies), and
// the body re-encoded as JSON.
func rawBodyHandler(rs **routeServer, key string, got *[]byte, seen *bool, status int, answer map[string]interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		*seen = true
		*got = nil
		if r.ContentLength > 0 {
			*got, _ = json.Marshal((*rs).bodies[key])
		}
		jsonHandler(status, answer)(w, r)
	}
}

func TestWebgrph_ContinueCrawl_WithAndWithoutOptions(t *testing.T) {
	var got []byte
	var seen bool
	var rs *routeServer
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST webgrph/crawl/c1/continue": rawBodyHandler(&rs, "POST webgrph/crawl/c1/continue", &got, &seen, 202,
			map[string]interface{}{"success": true, "crawl_id": "c1", "status": "queued"}),
	})
	defer server.Close()
	w := newTestClient(server).Webgrph

	if _, err := w.ContinueCrawl(context.Background(), "c1"); err != nil || !seen || len(got) != 0 {
		t.Fatalf("no options must send no body: err=%v body=%q", err, got)
	}
	seen = false
	if _, err := w.ContinueCrawl(context.Background(), "c1", &ContinueOptions{}); err != nil || !seen || len(got) != 0 {
		t.Fatalf("empty options must send no body: err=%v body=%q", err, got)
	}
	res, err := w.ContinueCrawl(context.Background(), "c1", &ContinueOptions{MaxPages: 200, MaxDepth: 4})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != `{"max_depth":4,"max_pages":200}` {
		t.Errorf("unexpected body: %s", got)
	}
	if !res.Success || res.CrawlID != "c1" || res.Status != "queued" {
		t.Errorf("unexpected result: %+v", res)
	}
	if _, err := w.ContinueCrawl(context.Background(), "c1", &ContinueOptions{MaxPages: 50}); err != nil || string(got) != `{"max_pages":50}` {
		t.Errorf("only set options are sent: err=%v body=%s", err, got)
	}
}

func TestWebgrph_ContinueCrawl_LoweringIsA422WithItsCode(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"POST webgrph/crawl/c1/continue": jsonHandler(422, map[string]interface{}{
			"detail": map[string]interface{}{"error": "continue_cannot_lower", "message": "max_pages can only be raised"}}),
	})
	defer server.Close()

	_, err := newTestClient(server).Webgrph.ContinueCrawl(context.Background(), "c1", &ContinueOptions{MaxPages: 5})
	var apiErr *ApiError
	if !errorsAs(err, &apiErr) || apiErr.StatusCode != 422 || apiErr.Code != "continue_cannot_lower" {
		t.Fatalf("expected a 422 continue_cannot_lower, got %v", err)
	}
}

func TestWebgrph_ContinueCrawlExtract_WithAndWithoutOptions(t *testing.T) {
	var got []byte
	var seen bool
	var rs *routeServer
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST webgrph/crawl-extract/j1/continue": rawBodyHandler(&rs, "POST webgrph/crawl-extract/j1/continue", &got, &seen, 202,
			map[string]interface{}{"success": true, "job_id": "j1", "crawl_id": "c1", "status": "queued"}),
		"POST webgrph/crawl-extract/j2/continue": jsonHandler(409, map[string]interface{}{
			"detail": map[string]interface{}{"error": "not_continuable", "message": "The job cannot be continued."}}),
	})
	defer server.Close()
	w := newTestClient(server).Webgrph

	res, err := w.ContinueCrawlExtract(context.Background(), "j1")
	if err != nil || !seen || len(got) != 0 {
		t.Fatalf("no options must send no body: err=%v body=%q", err, got)
	}
	if !res.Success || res.JobID != "j1" || res.CrawlID != "c1" || res.Status != "queued" || res.Raw["job_id"] != "j1" {
		t.Errorf("unexpected result: %+v", res)
	}
	if _, err := w.ContinueCrawlExtract(context.Background(), "j1", &ContinueOptions{MaxPages: 300}); err != nil || string(got) != `{"max_pages":300}` {
		t.Errorf("unexpected: err=%v body=%s", err, got)
	}
	_, err = w.ContinueCrawlExtract(context.Background(), "j2")
	var apiErr *ApiError
	if !errorsAs(err, &apiErr) || apiErr.StatusCode != 409 || apiErr.Code != "not_continuable" {
		t.Fatalf("expected a 409 ApiError, got %v", err)
	}
}

func TestCrawlStatus_TimeBudgetFields(t *testing.T) {
	r := crawlResultFromStatus("c1", map[string]interface{}{
		"status": "running", "time_budget_s": float64(150), "elapsed_s": 42.5, "est_remaining_s": 107.5,
	})
	if r.TimeBudgetS == nil || *r.TimeBudgetS != 150 || r.ElapsedS == nil || *r.ElapsedS != 42.5 ||
		r.EstRemainingS == nil || *r.EstRemainingS != 107.5 {
		t.Errorf("unexpected time fields: %+v", r)
	}
	r = crawlResultFromStatus("c1", map[string]interface{}{
		"status": "running", "time_budget_s": nil, "elapsed_s": nil,
	})
	if r.TimeBudgetS != nil || r.ElapsedS != nil || r.EstRemainingS != nil {
		t.Errorf("null or absent must be nil: %+v", r)
	}
}

func TestCrawlExtractStatus_TimeBudgetFields(t *testing.T) {
	raw := ceStatus("crawling")
	raw["time_budget_s"], raw["elapsed_s"], raw["est_remaining_s"] = float64(600), float64(30), 570.0
	s := parseCrawlExtractStatus(raw)
	if s.TimeBudgetS == nil || *s.TimeBudgetS != 600 || s.ElapsedS == nil || *s.ElapsedS != 30 ||
		s.EstRemainingS == nil || *s.EstRemainingS != 570 {
		t.Errorf("unexpected time fields: %+v", s)
	}
	s = parseCrawlExtractStatus(ceStatus("crawling"))
	if s.TimeBudgetS != nil || s.ElapsedS != nil || s.EstRemainingS != nil {
		t.Errorf("absent must be nil: %+v", s)
	}
}

func TestContract_EstimateAndContinueRoutesAreCalled(t *testing.T) {
	live, calls := liveRoutes(t), sdkCalls(t)
	for _, key := range []string{
		"POST webgrph/crawl/estimate", "POST webgrph/crawl-extract/estimate",
		"POST webgrph/crawl/{}/continue", "POST webgrph/crawl-extract/{}/continue",
	} {
		if !live[key] {
			t.Errorf("%s missing from testdata/routes.tsv", key)
		}
		if !calls[key] {
			t.Errorf("the SDK does not call %s", key)
		}
	}
}
