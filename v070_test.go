package kloakd

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// Tests for the calls and fields added after 0.6.0 against kobe-scraper
// 6bd19e9b: held crawls, what a crawl really did, ContinueCrawl, login
// sessions, the export's X-Crawl-* headers, and the new result fields.

const heldText = "Your plan's quota has run out. This crawl is paused and will resume when quota returns."

// heldCrawlStatus is GET webgrph/crawl/{id} for a held crawl
// (modules/webgrph/router.py get_crawl_status): no error, the held text in
// message.
func heldCrawlStatus() map[string]interface{} {
	return map[string]interface{}{
		"crawl_id": "c1", "status": "held_quota", "organization_id": testOrgID,
		"total_pages": float64(7), "max_depth_reached": float64(2), "artifact_id": nil,
		"error": nil, "message": heldText,
		"pages_discovered": float64(7), "pages_fetched": float64(4), "pages_unfetched": float64(0),
		"is_partial": nil, "partial_reason": nil, "resumable": nil, "error_code": nil,
	}
}

func heldCrawlRoutes(t *testing.T, statusCalls *int) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"POST webgrph/crawl": jsonHandler(202, map[string]interface{}{
			"success": true, "crawl_id": "c1", "url": "https://example.com", "total_pages": float64(0), "max_depth_reached": float64(0),
		}),
		"GET webgrph/crawl/c1": func(w http.ResponseWriter, r *http.Request) {
			*statusCalls++
			if *statusCalls == 1 {
				jsonHandler(200, map[string]interface{}{"crawl_id": "c1", "status": "running", "organization_id": testOrgID})(w, r)
				return
			}
			jsonHandler(200, heldCrawlStatus())(w, r)
		},
		"GET webgrph/crawl/c1/pages": func(w http.ResponseWriter, r *http.Request) {
			t.Errorf("the pages of a held crawl must not be loaded")
			w.WriteHeader(500)
		},
	}
}

func assertHeld(t *testing.T, result *CrawlResult) {
	t.Helper()
	if !result.Held || result.Status != "held_quota" || result.Success || result.Error != nil {
		t.Fatalf("expected a held crawl without an error, got %+v", result)
	}
	if result.Message == nil || *result.Message != heldText {
		t.Errorf("expected the API's message, got %v", result.Message)
	}
	if len(result.Pages) != 0 || result.CrawlID != "c1" || result.TotalPages != 7 {
		t.Errorf("unexpected held result: %+v", result)
	}
	if result.PagesFetched == nil || *result.PagesFetched != 4 || result.IsPartial != nil || result.Resumable {
		t.Errorf("unexpected outcome fields: %+v", result)
	}
}

func TestWebgrph_Crawl_HeldQuotaStopsTheWaitWithoutAnError(t *testing.T) {
	calls := 0
	_, server := newRouteServer(heldCrawlRoutes(t, &calls))
	defer server.Close()

	// A long timeout: the test passes only if held_quota ends the wait.
	result, err := newTestClient(server).Webgrph.Crawl(context.Background(), "https://example.com",
		&CrawlOptions{PollInterval: time.Millisecond, PollTimeout: time.Minute})
	if err != nil {
		t.Fatalf("a held crawl is not an error: %v", err)
	}
	assertHeld(t, result)
	if result.URL != "https://example.com" {
		t.Errorf("URL should be the caller's, got %q", result.URL)
	}
	if calls != 2 {
		t.Errorf("expected the wait to stop at the first held_quota (2 polls), got %d", calls)
	}
}

func TestWebgrph_WaitForCrawl_Held(t *testing.T) {
	calls := 1 // skip the "running" answer
	_, server := newRouteServer(heldCrawlRoutes(t, &calls))
	defer server.Close()

	result, err := newTestClient(server).Webgrph.WaitForCrawl(context.Background(), "c1",
		&PollOptions{Interval: time.Millisecond, Timeout: time.Minute})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertHeld(t, result)
	if result.URL != "" {
		t.Errorf("the status body has no url; got %q", result.URL)
	}
}

func TestWebgrph_CrawlAll_HeldReturnsNoPagesAndNoError(t *testing.T) {
	calls := 0
	_, server := newRouteServer(heldCrawlRoutes(t, &calls))
	defer server.Close()

	pages, err := newTestClient(server).Webgrph.CrawlAll(context.Background(), "https://example.com",
		&CrawlOptions{PollInterval: time.Millisecond, PollTimeout: time.Minute})
	if err != nil || len(pages) != 0 {
		t.Fatalf("expected no pages and no error, got %v, %v", pages, err)
	}
}

func TestClient_Crawl_HeldDiscoveryFetchesNothing(t *testing.T) {
	calls := 1 // the first poll already answers held_quota
	routes := heldCrawlRoutes(t, &calls)
	routes["POST evadr/fetch"] = func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no page may be fetched while the discovery crawl is held")
		w.WriteHeader(500)
	}
	_, server := newRouteServer(routes)
	defer server.Close()
	client := newTestClient(server)

	// The orchestrator has no poll options (60 min by default): the test
	// ends only because held_quota ends the wait.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := client.Crawl(ctx, "https://example.com", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Held || result.Status != "held_quota" || result.Success || result.Error != nil ||
		result.CrawlID != "c1" || result.Message == nil || *result.Message != heldText || len(result.Pages) != 0 {
		t.Errorf("unexpected site result: %+v", result)
	}

	calls = 1
	ch, err := client.CrawlStream(ctx, "https://example.com", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	events := drain(ch, 5*time.Second)
	if len(events) != 2 || events[0].Type != "discovery_complete" || events[1].Type != "crawl_complete" {
		t.Fatalf("unexpected events: %+v", events)
	}
	for _, ev := range events {
		if *ev.Success || ev.Error != nil || ev.Metadata["held"] != true || ev.Metadata["status"] != "held_quota" || ev.Metadata["crawl_id"] != "c1" {
			t.Errorf("unexpected %s event: %+v", ev.Type, ev)
		}
	}
}

func TestWebgrph_WaitForCrawl_CompletedPartialAndResumable(t *testing.T) {
	routes := crawlRoutes([]string{"completed"})
	routes["GET webgrph/crawl/c1"] = jsonHandler(200, map[string]interface{}{
		"crawl_id": "c1", "status": "completed", "organization_id": testOrgID,
		"total_pages": float64(3), "max_depth_reached": float64(1), "artifact_id": "art-c1", "error": nil, "message": nil,
		"pages_discovered": float64(40), "pages_fetched": float64(3), "pages_unfetched": float64(2),
		"is_partial": true, "partial_reason": "time_budget", "resumable": true, "error_code": nil,
	})
	rs, server := newRouteServer(routes)
	defer server.Close()

	result, err := newTestClient(server).Webgrph.WaitForCrawl(context.Background(), "c1", &PollOptions{Interval: time.Millisecond})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success || result.Held || result.Message != nil || len(result.Pages) != 3 || result.Total != 3 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.IsPartial == nil || !*result.IsPartial || result.PartialReason == nil || *result.PartialReason != "time_budget" || !result.Resumable {
		t.Errorf("unexpected partial fields: %+v", result)
	}
	if *result.PagesDiscovered != 40 || *result.PagesFetched != 3 || *result.PagesUnfetched != 2 || result.ErrorCode != nil {
		t.Errorf("unexpected counts: %+v", result)
	}
	if got := rs.seen[len(rs.seen)-1]; got != "GET webgrph/crawl/c1/pages" {
		t.Errorf("expected the pages to be loaded, last request %s", got)
	}
}

func TestWebgrph_WaitForCrawl_HeldQuotaExpiredIsAFailedCrawl(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"GET webgrph/crawl/c1": jsonHandler(200, map[string]interface{}{
			"crawl_id": "c1", "status": "failed", "organization_id": testOrgID, "total_pages": float64(7),
			"error": "The crawl was held for 30 days and has ended.", "error_code": "held_quota_expired",
		}),
	})
	defer server.Close()

	result, err := newTestClient(server).Webgrph.WaitForCrawl(context.Background(), "c1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success || result.Held || result.Status != "failed" || result.Error == nil ||
		result.ErrorCode == nil || *result.ErrorCode != "held_quota_expired" {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestWebgrph_WaitForCrawl_TimesOut(t *testing.T) {
	_, server := newRouteServer(crawlRoutes([]string{"crawling"}))
	defer server.Close()

	_, err := newTestClient(server).Webgrph.WaitForCrawl(context.Background(), "c1",
		&PollOptions{Interval: time.Millisecond, Timeout: 20 * time.Millisecond})
	var apiErr *ApiError
	if !errorsAs(err, &apiErr) || apiErr.Code != "crawl_timeout" || apiErr.JobID != "c1" {
		t.Fatalf("expected crawl_timeout, got %v", err)
	}
}

func TestWebgrph_ContinueCrawl(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST webgrph/crawl/c1/continue": jsonHandler(202, map[string]interface{}{"success": true, "crawl_id": "c1", "status": "queued"}),
		"POST webgrph/crawl/c2/continue": jsonHandler(409, map[string]interface{}{
			"detail": "Crawl cannot be continued: it has not ended, or is already being continued"}),
	})
	defer server.Close()
	w := newTestClient(server).Webgrph

	res, err := w.ContinueCrawl(context.Background(), "c1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success || res.CrawlID != "c1" || res.Status != "queued" {
		t.Errorf("unexpected result: %+v", res)
	}
	if len(rs.bodies["POST webgrph/crawl/c1/continue"]) != 0 {
		t.Errorf("continue takes no body, sent %v", rs.bodies["POST webgrph/crawl/c1/continue"])
	}

	_, err = w.ContinueCrawl(context.Background(), "c2")
	var apiErr *ApiError
	if !errorsAs(err, &apiErr) || apiErr.StatusCode != 409 {
		t.Fatalf("expected a 409 ApiError, got %v", err)
	}
}

func TestParsePageNode_FetchStatusAndError(t *testing.T) {
	n := parsePageNode(map[string]interface{}{
		"id": "u", "url": "https://example.com/u", "depth": float64(1), "category": "page", "children_urls": []interface{}{},
		"metadata": map[string]interface{}{"title": "U", "fetch_status": "failed", "fetch_error": "timeout"},
	})
	if n.FetchStatus == nil || *n.FetchStatus != "failed" || n.FetchError == nil || *n.FetchError != "timeout" {
		t.Errorf("unexpected node: %+v", n)
	}
	plain := parsePageNode(map[string]interface{}{"id": "v", "url": "https://example.com/v", "metadata": map[string]interface{}{}})
	if plain.FetchStatus != nil || plain.FetchError != nil {
		t.Errorf("expected nil fetch fields, got %+v", plain)
	}
}

// ─── crawl-extract ───────────────────────────────────────────────────────────

func ceStatusWithCrawl(status string, crawl map[string]interface{}) map[string]interface{} {
	body := ceStatus(status)
	body["crawl"] = crawl
	return body
}

func TestCrawlExtract_Wait_ReturnsWhenTheCrawlIsHeld(t *testing.T) {
	calls := 0
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"GET webgrph/crawl-extract/" + ceJob: func(w http.ResponseWriter, r *http.Request) {
			calls++
			crawlStatus := "running"
			if calls > 1 {
				crawlStatus = "held_quota"
			}
			jsonHandler(200, ceStatusWithCrawl("crawling", map[string]interface{}{
				"status": crawlStatus, "is_partial": nil, "partial_reason": nil, "pages_discovered": float64(9),
				"pages_fetched": float64(4), "pages_unfetched": nil, "cancelled": false, "root_unfetched": false,
				"error": nil, "error_code": nil,
			}))(w, r)
		},
	})
	defer server.Close()

	status, err := newTestClient(server).Webgrph.WaitForCrawlExtract(context.Background(), ceJob,
		&PollOptions{Interval: time.Millisecond, Timeout: time.Minute})
	if err != nil {
		t.Fatalf("a held crawl is not an error: %v", err)
	}
	if !status.Held || status.Finished() || status.Status != "crawling" || calls != 2 {
		t.Fatalf("expected the unfinished job with Held true after 2 polls, got %+v (%d polls)", status, calls)
	}
	c := status.Crawl
	if c == nil || c.Status != "held_quota" || c.IsPartial != nil || *c.PagesDiscovered != 9 || *c.PagesFetched != 4 ||
		c.PagesUnfetched != nil || c.Cancelled || c.RootUnfetched || c.Error != nil {
		t.Errorf("unexpected crawl: %+v", c)
	}
}

func TestCrawlExtract_Status_ParsesTheCrawlObject(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"GET webgrph/crawl-extract/" + ceJob: jsonHandler(200, ceStatusWithCrawl("completed", map[string]interface{}{
			"status": "failed", "is_partial": true, "partial_reason": "error", "pages_discovered": float64(30),
			"pages_fetched": float64(10), "pages_unfetched": float64(3), "cancelled": false, "root_unfetched": true,
			"error": "root page not fetched", "error_code": "root_unfetched",
		})),
		"GET webgrph/crawl-extract/none": jsonHandler(200, ceStatus("completed")),
	})
	defer server.Close()
	w := newTestClient(server).Webgrph

	status, err := w.GetCrawlExtract(context.Background(), ceJob)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	c := status.Crawl
	if status.Held || c == nil || c.Status != "failed" || !*c.IsPartial || *c.PartialReason != "error" ||
		*c.PagesDiscovered != 30 || *c.PagesFetched != 10 || *c.PagesUnfetched != 3 || !c.RootUnfetched ||
		*c.Error != "root page not fetched" || *c.ErrorCode != "root_unfetched" {
		t.Errorf("unexpected crawl: %+v", c)
	}

	without, err := w.GetCrawlExtract(context.Background(), "none")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if without.Crawl != nil || without.Held {
		t.Errorf("expected no crawl object, got %+v", without.Crawl)
	}
}

func TestCrawlExtract_Export_ReadsTheCrawlHeaders(t *testing.T) {
	inner := exportHandler("text/csv", "crawl-extract-"+ceJob+".csv", "completed", "a,b\n1,2\n")
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"GET webgrph/crawl-extract/" + ceJob + "/export": func(w http.ResponseWriter, r *http.Request) {
			// As crawl_extract_router._crawl_headers writes them: booleans
			// lower-case, a value not reported left out.
			w.Header().Set("X-Crawl-Status", "completed")
			w.Header().Set("X-Crawl-Partial", "true")
			w.Header().Set("X-Crawl-Partial-Reason", "max_pages")
			w.Header().Set("X-Crawl-Pages-Discovered", "120")
			w.Header().Set("X-Crawl-Pages-Fetched", "50")
			w.Header().Set("X-Crawl-Cancelled", "false")
			w.Header().Set("X-Crawl-Root-Unfetched", "false")
			inner(w, r)
		},
	})
	defer server.Close()
	w := newTestClient(server).Webgrph

	dl, err := w.ExportCrawlExtract(context.Background(), ceJob, &CrawlExtractExportOptions{Format: ExportCSV})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, _ = io.Copy(io.Discard, dl.Body)
	dl.Body.Close()
	want := &ExportCrawl{
		Status: ptr("completed"), IsPartial: ptr(true), PartialReason: ptr("max_pages"),
		PagesDiscovered: ptr(120), PagesFetched: ptr(50), PagesUnfetched: nil,
		Cancelled: ptr(false), RootUnfetched: ptr(false),
	}
	if dl.JobStatus != "completed" || !reflect.DeepEqual(dl.Crawl, want) {
		t.Fatalf("unexpected export: status %q crawl %+v", dl.JobStatus, dl.Crawl)
	}

	saved, err := w.DownloadCrawlExtract(context.Background(), ceJob, filepath.Join(t.TempDir(), "rows.csv"),
		&CrawlExtractExportOptions{Format: ExportCSV})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if saved.JobStatus != "completed" || !reflect.DeepEqual(saved.Crawl, want) || saved.Bytes != 8 {
		t.Errorf("unexpected saved export: %+v crawl %+v", saved, saved.Crawl)
	}
}

func TestExport_NoCrawlHeadersMeansNilCrawl(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		// An export whose crawl row is gone: X-Job-Status only.
		"GET webgrph/crawl-extract/" + ceJob + "/export": exportHandler("application/json", "x.json", "failed", "[]"),
		// The batch export sends Content-Disposition and Cache-Control only
		// (kernel/batch_export_router.py).
		"GET batch-jobs/b1/export": exportHandler("application/json", "batch-b1.json", "", "[]"),
	})
	defer server.Close()
	client := newTestClient(server)

	dl, err := client.Webgrph.ExportCrawlExtract(context.Background(), ceJob, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dl.Body.Close()
	if dl.Crawl != nil || dl.JobStatus != "failed" {
		t.Errorf("expected a nil Crawl and the job status, got %+v %q", dl.Crawl, dl.JobStatus)
	}

	batch, err := client.ExportBatchJob(context.Background(), "b1", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	batch.Body.Close()
	if batch.Crawl != nil || batch.JobStatus != "" {
		t.Errorf("a batch export has neither header: %+v %q", batch.Crawl, batch.JobStatus)
	}
}

// ─── Fetchyr login sessions ──────────────────────────────────────────────────

func TestFetchyr_StartLoginSession_Interactive(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST fetchyr/login-sessions": jsonHandler(202, map[string]interface{}{
			"method": "interactive", "status": "waiting_for_login", "target_url": "https://shop.example/account",
			"discovery_id": "d-1", "session_artifact_id": nil,
			"next":         map[string]interface{}{"status": "/fetchyr/auth-chain/d-1", "credentials": "/fetchyr/auth-chain/d-1/resume-credentials"},
			"cookies_kept": nil, "cookies_dropped": []interface{}{},
			"protections": map[string]interface{}{"max_requests_per_hour": float64(120)},
			"message":     "Send your credentials to continue.",
		}),
	})
	defer server.Close()

	session, err := newTestClient(server).Fetchyr.StartLoginSession(context.Background(), "https://shop.example/account", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only what the caller set is sent: the API applies its own defaults.
	if body := rs.bodies["POST fetchyr/login-sessions"]; !reflect.DeepEqual(body, map[string]interface{}{"target_url": "https://shop.example/account"}) {
		t.Errorf("unexpected body: %v", body)
	}
	if session.Method != "interactive" || session.Status != "waiting_for_login" || session.TargetURL != "https://shop.example/account" ||
		session.DiscoveryID == nil || *session.DiscoveryID != "d-1" || session.SessionArtifactID != nil ||
		session.CookiesKept != nil || len(session.CookiesDropped) != 0 ||
		session.Next["credentials"] != "/fetchyr/auth-chain/d-1/resume-credentials" ||
		session.Protections["max_requests_per_hour"] != float64(120) || session.Message != "Send your credentials to continue." {
		t.Errorf("unexpected session: %+v", session)
	}
}

func TestFetchyr_StartLoginSession_Cookies(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST fetchyr/login-sessions": jsonHandler(201, map[string]interface{}{
			"method": "cookies", "status": "ready", "target_url": "https://shop.example/account",
			"discovery_id": nil, "session_artifact_id": "art-s1", "next": map[string]interface{}{"session": "/fetchyr/sessions/art-s1"},
			"cookies_kept": float64(1), "cookies_dropped": []interface{}{"tracker"},
			"protections": map[string]interface{}{}, "message": "The session is ready.",
		}),
	})
	defer server.Close()

	session, err := newTestClient(server).Fetchyr.StartLoginSession(context.Background(), "https://shop.example/account",
		&LoginSessionOptions{
			Method: "cookies",
			Cookies: []LoginCookie{
				{Name: "sid", Value: "v1", Domain: ".shop.example", Path: "/", Expires: 1800000000, Secure: true, HTTPOnly: true, SameSite: "Lax"},
				{Name: "tracker", Value: "v2"},
			},
			UserAgent: "Mozilla/5.0 test", Country: "KE",
		})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]interface{}{
		"target_url": "https://shop.example/account", "method": "cookies", "user_agent": "Mozilla/5.0 test", "country": "KE",
		"cookies": []interface{}{
			map[string]interface{}{"name": "sid", "value": "v1", "domain": ".shop.example", "path": "/",
				"expires": float64(1800000000), "secure": true, "httpOnly": true, "sameSite": "Lax"},
			map[string]interface{}{"name": "tracker", "value": "v2"},
		},
	}
	if body := rs.bodies["POST fetchyr/login-sessions"]; !reflect.DeepEqual(body, want) {
		t.Errorf("unexpected body:\n got %v\nwant %v", body, want)
	}
	if session.Status != "ready" || session.SessionArtifactID == nil || *session.SessionArtifactID != "art-s1" ||
		session.DiscoveryID != nil || session.CookiesKept == nil || *session.CookiesKept != 1 ||
		!reflect.DeepEqual(session.CookiesDropped, []string{"tracker"}) {
		t.Errorf("unexpected session: %+v", session)
	}
}

func TestFetchyr_StartLoginSession_Refused(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"POST fetchyr/login-sessions": jsonHandler(422, map[string]interface{}{
			"detail": "method=cookies needs user_agent: your browser's navigator.userAgent"}),
	})
	defer server.Close()

	_, err := newTestClient(server).Fetchyr.StartLoginSession(context.Background(), "https://shop.example/",
		&LoginSessionOptions{Method: "cookies", Cookies: []LoginCookie{{Name: "sid", Value: "v"}}})
	var apiErr *ApiError
	if !errorsAs(err, &apiErr) || apiErr.StatusCode != 422 {
		t.Fatalf("expected a 422 ApiError, got %v", err)
	}
}

func sessionStatusBody(status string) map[string]interface{} {
	body := map[string]interface{}{
		"artifact_id": "art-s1", "status": status, "target_domain": "shop.example", "auth_method": "cookies",
		"created_at": "2026-11-12T09:00:00+00:00", "expires_at": "2026-11-19T09:00:00+00:00", "last_used_at": nil,
		"paused_at": nil, "pause_reason": nil, "pause_message": nil,
		"identity":           map[string]interface{}{"user_agent": "Mozilla/5.0 test", "country": "KE"},
		"requests_this_hour": float64(12), "protections": map[string]interface{}{"max_requests_per_hour": float64(120)},
	}
	if status == "paused" {
		body["paused_at"] = "2026-11-12T10:00:00+00:00"
		body["pause_reason"] = "rate_limited"
		body["pause_message"] = "The site slowed this session down."
	}
	return body
}

func TestFetchyr_GetSession(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"GET fetchyr/sessions/art-s1": jsonHandler(200, sessionStatusBody("paused")),
	})
	defer server.Close()
	f := newTestClient(server).Fetchyr

	s, err := f.GetSession(context.Background(), "art-s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.ArtifactID != "art-s1" || s.Status != "paused" || *s.TargetDomain != "shop.example" || *s.AuthMethod != "cookies" ||
		*s.CreatedAt != "2026-11-12T09:00:00+00:00" || *s.ExpiresAt != "2026-11-19T09:00:00+00:00" || s.LastUsedAt != nil ||
		*s.PausedAt != "2026-11-12T10:00:00+00:00" || *s.PauseReason != "rate_limited" ||
		*s.PauseMessage != "The site slowed this session down." || s.Identity["country"] != "KE" ||
		*s.RequestsThisHour != 12 || s.Protections["max_requests_per_hour"] != float64(120) {
		t.Errorf("unexpected status: %+v", s)
	}

	_, err = f.GetSession(context.Background(), "missing")
	var apiErr *ApiError
	if !errorsAs(err, &apiErr) || apiErr.StatusCode != 404 {
		t.Fatalf("expected a 404 ApiError, got %v", err)
	}
}

func TestFetchyr_ResumeSession(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST fetchyr/sessions/art-s1/resume": jsonHandler(200, sessionStatusBody("active")),
		"POST fetchyr/sessions/art-s2/resume": jsonHandler(409, map[string]interface{}{"detail": "The session is not paused"}),
	})
	defer server.Close()
	f := newTestClient(server).Fetchyr

	s, err := f.ResumeSession(context.Background(), "art-s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Status != "active" || s.PausedAt != nil || s.PauseReason != nil || s.ArtifactID != "art-s1" {
		t.Errorf("unexpected status: %+v", s)
	}
	if len(rs.bodies["POST fetchyr/sessions/art-s1/resume"]) != 0 {
		t.Errorf("resume takes no body, sent %v", rs.bodies["POST fetchyr/sessions/art-s1/resume"])
	}

	_, err = f.ResumeSession(context.Background(), "art-s2")
	var apiErr *ApiError
	if !errorsAs(err, &apiErr) || apiErr.StatusCode != 409 {
		t.Fatalf("expected a 409 ApiError, got %v", err)
	}
}

// ─── Kolektr ─────────────────────────────────────────────────────────────────

func TestKolektr_Extract_ReadsLevelAndLoginOptions(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"POST kolektr/extract": jsonHandler(200, map[string]interface{}{
			"success": false, "url": "https://shop.example/account", "method": "l1_css", "records": []interface{}{},
			"total_records": float64(0), "pages_scraped": float64(0),
			"level": "level2", "level_reason": "auto:javascript_required",
			"auth_required": true, "discovery_id": "d-9",
			"login_options": map[string]interface{}{"start": "POST /fetchyr/login-sessions", "methods": []interface{}{"interactive", "cookies"}},
		}),
	})
	defer server.Close()

	res, err := newTestClient(server).Kolektr.Extract(context.Background(), "https://shop.example/account", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Level == nil || *res.Level != "level2" || res.LevelReason == nil || *res.LevelReason != "auto:javascript_required" ||
		!res.AuthRequired || res.LoginOptions["start"] != "POST /fetchyr/login-sessions" {
		t.Errorf("unexpected result: %+v", res)
	}

	plain := parseExtractionResult(map[string]interface{}{"success": true, "url": "u", "method": "m"})
	if plain.Level != nil || plain.LevelReason != nil || plain.LoginOptions != nil {
		t.Errorf("expected nil level and login options, got %+v", plain)
	}
}
