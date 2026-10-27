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

func TestContract_EverySDKCallIsALiveRoute(t *testing.T) {
	live := liveRoutes(t)
	call := regexp.MustCompile(`\.t\.(get|post|put|patch|delete|stream|streamWithEvents|getStream|do|request)\(ctx,\s*(?:http\.Method(\w+),\s*)?(?:fmt\.Sprintf\()?"([^"]+)"`)
	verbs := map[string]string{"get": "GET", "post": "POST", "put": "PUT", "patch": "PATCH", "delete": "DELETE",
		"stream": "POST", "streamWithEvents": "POST", "getStream": "GET"}
	files, _ := filepath.Glob("*.go")
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
				if strings.Contains(line, `"parlyr/chat/"+`) {
					path = "parlyr/chat/%s"
				}
				key := method + " " + regexp.MustCompile(`%[sdv]`).ReplaceAllString(path, "{}")
				found++
				if !live[key] {
					t.Errorf("%s calls %s, which is not a live route", file, key)
				}
			}
		}
	}
	if found < 100 {
		t.Fatalf("found only %d calls; the scan is broken", found)
	}
}
