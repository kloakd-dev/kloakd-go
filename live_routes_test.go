package kloakd

import (
	"bufio"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// ─── 202: the synchronous fetch hands off to a job ──────────────────────────

var handedOff = map[string]interface{}{
	"job_id": "job-1", "status": "running", "reason": "sync_limit_reached", "waited_seconds": float64(85),
	"poll_url": "/api/v1/organizations/" + testOrgID + "/evadr/jobs/job-1", "message": "still running",
}

func TestEvadr_Fetch_FollowsAHandedOffJob(t *testing.T) {
	polls := 0
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST evadr/fetch": jsonHandler(202, handedOff),
		"GET evadr/jobs/job-1": func(w http.ResponseWriter, r *http.Request) {
			polls++
			if polls < 2 {
				jsonHandler(200, map[string]interface{}{"job_id": "job-1", "status": "running"})(w, r)
				return
			}
			jsonHandler(200, map[string]interface{}{"job_id": "job-1", "status": "completed", "result": fetchResponse})(w, r)
		},
	})
	defer server.Close()

	result, err := newTestClient(server).Evadr.Fetch(context.Background(), "https://example.com",
		&FetchOptions{PollInterval: time.Millisecond})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success || result.TierUsed != 2 || result.HTML == nil {
		t.Errorf("expected the job's result, got %+v", result)
	}
	if polls != 2 || rs.seen[0] != "POST evadr/fetch" {
		t.Errorf("unexpected calls: %v", rs.seen)
	}
}

func TestEvadr_Fetch_AFailedJobIsAnError(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"POST evadr/fetch":     jsonHandler(202, handedOff),
		"GET evadr/jobs/job-1": jsonHandler(200, map[string]interface{}{"job_id": "job-1", "status": "failed", "error": "blocked"}),
	})
	defer server.Close()

	_, err := newTestClient(server).Evadr.Fetch(context.Background(), "https://example.com",
		&FetchOptions{PollInterval: time.Millisecond})
	var apiErr *ApiError
	if !errorsAs(err, &apiErr) || apiErr.Code != "job_failed" || apiErr.Message != "blocked" {
		t.Fatalf("expected job_failed, got %v", err)
	}
}

func TestEvadr_Fetch_PollTimeout(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"POST evadr/fetch":     jsonHandler(202, handedOff),
		"GET evadr/jobs/job-1": jsonHandler(200, map[string]interface{}{"job_id": "job-1", "status": "running"}),
	})
	defer server.Close()

	_, err := newTestClient(server).Evadr.Fetch(context.Background(), "https://example.com",
		&FetchOptions{PollInterval: time.Millisecond, PollTimeout: 10 * time.Millisecond})
	var apiErr *ApiError
	if !errorsAs(err, &apiErr) || apiErr.Code != "job_timeout" {
		t.Fatalf("expected job_timeout, got %v", err)
	}
}

func TestEvadr_Fetch_ContextCancelStopsTheWait(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"POST evadr/fetch":     jsonHandler(202, handedOff),
		"GET evadr/jobs/job-1": jsonHandler(200, map[string]interface{}{"job_id": "job-1", "status": "running"}),
	})
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := newTestClient(server).Evadr.Fetch(ctx, "https://example.com", &FetchOptions{PollInterval: 5 * time.Millisecond})
	if err == nil || ctx.Err() == nil {
		t.Fatalf("expected the context's error, got %v", err)
	}
}

// ─── typed billing errors ───────────────────────────────────────────────────

func detail(code, message string, extra map[string]interface{}) map[string]interface{} {
	d := map[string]interface{}{"error": code, "message": message}
	for k, v := range extra {
		d[k] = v
	}
	return map[string]interface{}{"detail": d}
}

func TestErrors_402PaymentRequired(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"POST evadr/fetch": jsonHandler(402, detail("payment_required", "Your last payment did not go through.",
			map[string]interface{}{"upgrade_url": "/settings/billing"})),
	})
	defer server.Close()

	_, err := newTestClient(server).Evadr.Fetch(context.Background(), "https://example.com", nil)
	var e *PaymentRequiredError
	if !errorsAs(err, &e) || e.Code != "payment_required" || e.UpgradeURL != "/settings/billing" {
		t.Fatalf("expected PaymentRequiredError, got %T %v", err, err)
	}
}

func TestErrors_429GBExhaustedIsNotRetried(t *testing.T) {
	calls := 0
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"POST evadr/fetch": func(w http.ResponseWriter, r *http.Request) {
			calls++
			jsonHandler(429, detail("gb_exhausted", "allocation used", map[string]interface{}{
				"residential_gb_used": float64(10), "residential_gb_limit": float64(10),
			}))(w, r)
		},
	})
	defer server.Close()
	client, _ := New(Config{APIKey: testAPIKey, OrganizationID: testOrgID, BaseURL: server.URL, MaxRetries: 3, HTTPClient: server.Client()})

	_, err := client.Evadr.Fetch(context.Background(), "https://example.com", nil)
	var e *QuotaExceededError
	if !errorsAs(err, &e) || e.GBUsed != 10 || e.GBLimit != 10 {
		t.Fatalf("expected QuotaExceededError, got %T %v", err, err)
	}
	if calls != 1 {
		t.Errorf("an exhausted quota must not be retried; calls=%d", calls)
	}
}

func TestErrors_429RateLimited(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"POST evadr/fetch": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "7")
			jsonHandler(429, detail("rate_limited", "slow down", nil))(w, r)
		},
	})
	defer server.Close()

	_, err := newTestClient(server).Evadr.Fetch(context.Background(), "https://example.com", nil)
	var e *RateLimitError
	if !errorsAs(err, &e) || e.RetryAfter != 7 || e.Code != "rate_limited" {
		t.Fatalf("expected RateLimitError(7), got %T %v", err, err)
	}
}

func TestErrors_503BillingUnavailable(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"POST evadr/fetch": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "30")
			jsonHandler(503, detail("billing_unavailable", "Usage could not be checked", nil))(w, r)
		},
	})
	defer server.Close()

	_, err := newTestClient(server).Evadr.Fetch(context.Background(), "https://example.com", nil)
	var e *BillingUnavailableError
	if !errorsAs(err, &e) || e.RetryAfter != 30 {
		t.Fatalf("expected BillingUnavailableError(30), got %T %v", err, err)
	}
}

// ─── contract: every call the SDK makes is a live route ─────────────────────

func liveRoutes(t *testing.T) map[string]bool {
	f, err := os.Open(filepath.Join("testdata", "routes.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	live := map[string]bool{}
	param := regexp.MustCompile(`\{[^}]+\}`)
	prefix := "/api/v1/organizations/{organization_id}/"
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		parts := strings.SplitN(sc.Text(), "\t", 2)
		if len(parts) == 2 && strings.HasPrefix(parts[1], prefix) {
			live[parts[0]+" "+param.ReplaceAllString(strings.TrimPrefix(parts[1], prefix), "{}")] = true
		}
	}
	return live
}

// sdkCalls scans the SDK's source for every route it calls, as
// "METHOD path" with parameters as {}.
func sdkCalls(t *testing.T) map[string]bool {
	call := regexp.MustCompile(`\.t\.(get|getList|getRaw|post|postRaw|postFollowing|patch|delete|postStream|getStream|do|request)\(ctx,\s*(?:http\.Method(\w+),\s*)?(?:fmt\.Sprintf\(|pathf\()?"([^"]+)"`)
	verbs := map[string]string{"get": "GET", "getList": "GET", "getRaw": "GET", "post": "POST", "postRaw": "POST",
		"postFollowing": "POST", "patch": "PATCH", "delete": "DELETE", "postStream": "POST", "getStream": "GET"}
	files, _ := filepath.Glob("*.go")
	calls := map[string]bool{}
	found := 0
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		src, _ := os.ReadFile(file)
		for _, line := range strings.Split(string(src), "\n") {
			for _, m := range call.FindAllStringSubmatch(line, -1) {
				method := verbs[m[1]]
				if m[2] != "" {
					method = strings.ToUpper(m[2])
				}
				path := m[3]
				calls[method+" "+regexp.MustCompile(`%[sdv]`).ReplaceAllString(path, "{}")] = true
				found++
			}
		}
	}
	if found < 80 {
		t.Fatalf("found only %d calls; the scan is broken", found)
	}
	return calls
}

func TestContract_EverySDKCallIsALiveRoute(t *testing.T) {
	live := liveRoutes(t)
	for key := range sdkCalls(t) {
		if !live[key] {
			t.Errorf("the SDK calls %s, which is not a live route", key)
		}
	}
}

// 0.4.0: routes deleted from the API because they returned invented results
// (kobe-scraper d7d25134; nexus/recommendations, POST kolektr/jobs, Kolektr's
// progress and pipeline routes, two Webgrph analytics routes, GET
// evadr/profiles), plus the standalone recommendations router, which is not
// mounted. The hand-off poll route is new.
var deletedRoutes = []string{
	"POST kolektr/jobs",
	"GET kolektr/jobs/{}/progress", "GET kolektr/jobs/{}/progress/summary",
	"GET kolektr/jobs/{}/progress/events", "GET kolektr/jobs/{}/progress/latest",
	"GET kolektr/progress/phases", "GET kolektr/progress/phases/{}",
	"GET kolektr/progress/phases/{}/steps", "GET kolektr/progress/summary",
	"GET kolektr/pipeline/{}/stream", "GET kolektr/pipeline/{}/events",
	"GET webgrph/analytics/job-trends", "GET webgrph/analytics/error-summary",
	"GET evadr/profiles",
}

// Webgrph's dashboard analytics routes still exist but are served by the
// dashboard-only sub-router and left out of the public API
// (scripts/generate_public_openapi.py EXCLUDED); the SDK does not call them.
var dashboardOnlyRoutes = []string{
	"GET webgrph/analytics/dashboard/summary", "GET webgrph/analytics/content/discovery-patterns",
	"GET webgrph/analytics/scraping/efficiency-metrics", "GET webgrph/analytics/site-maps/trends",
	"GET webgrph/analytics/users/behavior-insights",
}

func TestContract_DeletedRoutesAreGoneAndTheHandOffPollIsLive(t *testing.T) {
	live := liveRoutes(t)
	if !live["GET handoff/{}"] {
		t.Error("GET handoff/{job_id} missing from testdata/routes.tsv")
	}
	for _, key := range deletedRoutes {
		if live[key] {
			t.Errorf("%s is deleted but still in testdata/routes.tsv", key)
		}
	}
	for key := range live {
		path := strings.SplitN(key, " ", 2)[1]
		if strings.HasPrefix(path, "nexus/recommendations/") || strings.HasPrefix(path, "recommendations/") {
			t.Errorf("%s is deleted or unmounted but still in testdata/routes.tsv", key)
		}
	}
	for _, key := range dashboardOnlyRoutes {
		if !live[key] {
			t.Errorf("%s still exists (dashboard-only) and should stay in testdata/routes.tsv", key)
		}
	}
	calls := sdkCalls(t)
	for _, key := range append(append([]string{}, deletedRoutes...), dashboardOnlyRoutes...) {
		if calls[key] {
			t.Errorf("the SDK still calls %s", key)
		}
	}
	for key := range calls {
		if strings.Contains(key, " nexus/recommendations/") || strings.Contains(key, " webgrph/analytics/") {
			t.Errorf("the SDK still calls %s", key)
		}
	}
}

// 0.5.0: the Skanyr reads and the auth-chain resumes the JS and Python
// SDKs had and Go lacked, and the public hand-off poll.
func TestContract_ParityRoutesAreCalled(t *testing.T) {
	calls := sdkCalls(t)
	for _, key := range []string{
		"GET skanyr/resources", "GET skanyr/resources/{}",
		"GET skanyr/extractions/{}/text", "GET skanyr/extractions/{}/structure",
		"POST skanyr/sites/{}/export", "GET skanyr/sites/{}/diff", "GET skanyr/sites/{}/findings",
		"GET skanyr/sites/{}/graph", "GET skanyr/sites/{}/graph/stats", "GET skanyr/sites/{}/graph/subgraph/{}",
		"GET skanyr/sites/{}/search", "GET skanyr/sites/{}/snapshots", "GET skanyr/sites/{}/technologies",
		"GET skanyr/sites/{}/technology-evidence", "GET skanyr/sites/{}/timeline", "GET skanyr/sites/{}/timeline/{}",
		"POST fetchyr/auth-chain/{}/resume-credentials", "POST fetchyr/auth-chain/{}/resume-mfa",
		"GET handoff/{}",
	} {
		if !calls[key] {
			t.Errorf("the SDK does not call %s", key)
		}
	}
}
