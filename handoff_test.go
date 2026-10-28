package kloakd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Routes wrapped by @hands_off_after_limit (kobe-scraper kernel/sync_handoff.py)
// answer 202 {job_id, poll_url} after the synchronous limit; the result is at
// GET /api/v1/organizations/{org}/handoff/{job_id}.

const handoffPoll = "GET handoff/job-9"

func handoff202() map[string]interface{} {
	return map[string]interface{}{
		"job_id": "job-9", "status": "running", "reason": "sync_limit_reached", "waited_seconds": float64(85),
		"poll_url": "/api/v1/organizations/" + testOrgID + "/handoff/job-9",
		"message":  "Still running; it continues as a job. Poll poll_url for the result.",
	}
}

func newPollingClient(server *httptest.Server) *Client {
	client, err := New(Config{
		APIKey: testAPIKey, OrganizationID: testOrgID, BaseURL: server.URL, MaxRetries: -1,
		HTTPClient: server.Client(), PollInterval: time.Millisecond, PollTimeout: time.Second,
	})
	if err != nil {
		panic(err)
	}
	return client
}

// handOffCase is one method that can be handed off, the route it posts to,
// the route's 200 body and a check that the typed result came from it.
type handOffCase struct {
	route  string
	result map[string]interface{}
	call   func(c *Client) (interface{}, error)
	check  func(t *testing.T, v interface{})
}

func handOffCases() map[string]handOffCase {
	ctx := context.Background()
	return map[string]handOffCase{
		"Evadr.Fetch": {"evadr/fetch", map[string]interface{}{"success": true, "url": "https://x", "tier_used": float64(3)},
			func(c *Client) (interface{}, error) { return c.Evadr.Fetch(ctx, "https://x", nil) },
			func(t *testing.T, v interface{}) {
				if r := v.(*FetchResult); !r.Success || r.TierUsed != 3 {
					t.Errorf("got %+v", r)
				}
			}},
		"Kolektr.Extract": {"kolektr/extract", map[string]interface{}{"success": true, "url": "https://x", "total_records": float64(2)},
			func(c *Client) (interface{}, error) { return c.Kolektr.Extract(ctx, "https://x", nil) },
			func(t *testing.T, v interface{}) {
				if r := v.(*ExtractionResult); !r.Success || r.TotalRecords != 2 {
					t.Errorf("got %+v", r)
				}
			}},
		"Fetchyr.Authenticate": {"fetchyr/authenticate", map[string]interface{}{"success": true, "artifact_id": "sess-1"},
			func(c *Client) (interface{}, error) { return c.Fetchyr.Authenticate(ctx, "acme", "https://x/login") },
			func(t *testing.T, v interface{}) {
				if r := v.(*SessionResult); !r.Success || r.ArtifactID == nil {
					t.Errorf("got %+v", r)
				}
			}},
		"Fetchyr.DetectForms": {"fetchyr/form/detect", map[string]interface{}{"url": "https://x", "forms_found": float64(1), "login_form_detected": true},
			func(c *Client) (interface{}, error) { return c.Fetchyr.DetectForms(ctx, "https://x", "") },
			func(t *testing.T, v interface{}) {
				if r := v.(*FormDetectionResult); r.FormsFound != 1 || !r.LoginFormDetected {
					t.Errorf("got %+v", r)
				}
			}},
		"Fetchyr.FillForm": {"fetchyr/form/fill", map[string]interface{}{"success": true, "submitted": true},
			func(c *Client) (interface{}, error) {
				return c.Fetchyr.FillForm(ctx, "https://x", map[string]string{"q": "1"}, "")
			},
			func(t *testing.T, v interface{}) {
				if r := v.(map[string]interface{}); r["submitted"] != true {
					t.Errorf("got %+v", r)
				}
			}},
		"Fetchyr.DetectMFA": {"fetchyr/mfa/detect", map[string]interface{}{"mfa_detected": true, "mfa_type": "totp"},
			func(c *Client) (interface{}, error) { return c.Fetchyr.DetectMFA(ctx, "https://x", "") },
			func(t *testing.T, v interface{}) {
				if r := v.(*MfaDetectionResult); !r.MfaDetected || r.MfaType == nil {
					t.Errorf("got %+v", r)
				}
			}},
		"Nexus.Analyze": {"nexus/analyze", map[string]interface{}{"url": "https://x", "page_type": "listing", "confidence": 0.9},
			func(c *Client) (interface{}, error) { return c.Nexus.Analyze(ctx, "https://x", "<html/>", nil) },
			func(t *testing.T, v interface{}) {
				if r := v.(*NexusAnalyzeResult); r.PageType != "listing" {
					t.Errorf("got %+v", r)
				}
			}},
		"Nexus.Synthesize": {"nexus/synthesize", map[string]interface{}{"strategy_id": "s-1", "program": "p"},
			func(c *Client) (interface{}, error) { return c.Nexus.Synthesize(ctx, "https://x", "<html/>", "prices") },
			func(t *testing.T, v interface{}) {
				if r := v.(*NexusSynthesisResult); r.StrategyID != "s-1" {
					t.Errorf("got %+v", r)
				}
			}},
		"Nexus.Execute": {"nexus/execute", map[string]interface{}{"strategy_id": "s-1", "success": true, "execution_result_id": "er-1"},
			func(c *Client) (interface{}, error) { return c.Nexus.Execute(ctx, "s-1", "https://x", nil) },
			func(t *testing.T, v interface{}) {
				if r := v.(*NexusExecuteResult); !r.Success || r.ExecutionResultID == nil || *r.ExecutionResultID != "er-1" {
					t.Errorf("got %+v", r)
				}
			}},
	}
}

func TestHandOff_EveryHandOffRouteIsFollowedToItsResult(t *testing.T) {
	for name, tc := range handOffCases() {
		t.Run(name, func(t *testing.T) {
			polls := 0
			rs, server := newRouteServer(map[string]http.HandlerFunc{
				"POST " + tc.route: jsonHandler(202, handoff202()),
				handoffPoll: func(w http.ResponseWriter, r *http.Request) {
					polls++
					assertHeader(t, r, "Authorization", "Bearer "+testAPIKey)
					if polls < 3 {
						jsonHandler(200, map[string]interface{}{"job_id": "job-9", "kind": tc.route, "status": "running"})(w, r)
						return
					}
					jsonHandler(200, map[string]interface{}{"job_id": "job-9", "kind": tc.route, "status": "completed", "result": tc.result})(w, r)
				},
			})
			defer server.Close()

			v, err := tc.call(newPollingClient(server))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tc.check(t, v)
			if polls != 3 || rs.seen[0] != "POST "+tc.route {
				t.Errorf("unexpected calls: %v", rs.seen)
			}
		})
	}
}

func TestHandOff_ADirect200IsNotPolled(t *testing.T) {
	for name, tc := range handOffCases() {
		t.Run(name, func(t *testing.T) {
			rs, server := newRouteServer(map[string]http.HandlerFunc{"POST " + tc.route: jsonHandler(200, tc.result)})
			defer server.Close()
			v, err := tc.call(newPollingClient(server))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tc.check(t, v)
			if len(rs.seen) != 1 {
				t.Errorf("unexpected calls: %v", rs.seen)
			}
		})
	}
}

func TestHandOff_AFailedJobIsJobFailed(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"POST nexus/execute": jsonHandler(202, handoff202()),
		handoffPoll:          jsonHandler(200, map[string]interface{}{"job_id": "job-9", "status": "failed", "error": "HTTPException: engine down"}),
	})
	defer server.Close()
	_, err := newPollingClient(server).Nexus.Execute(context.Background(), "s-1", "https://x", nil)
	var apiErr *ApiError
	if !errorsAs(err, &apiErr) || apiErr.Code != "job_failed" || apiErr.JobID != "job-9" || apiErr.Message != "HTTPException: engine down" {
		t.Fatalf("expected job_failed, got %v", err)
	}
}

func TestHandOff_AnExpiredJobIsJobFailed(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"POST kolektr/extract": jsonHandler(202, handoff202()),
		handoffPoll:            jsonHandler(404, map[string]interface{}{"detail": "Job job-9 not found or expired"}),
	})
	defer server.Close()
	_, err := newPollingClient(server).Kolektr.Extract(context.Background(), "https://x", nil)
	var apiErr *ApiError
	if !errorsAs(err, &apiErr) || apiErr.Code != "job_failed" || apiErr.StatusCode != 404 || apiErr.JobID != "job-9" ||
		!strings.Contains(apiErr.Message, "not found or expired") {
		t.Fatalf("expected job_failed (expired), got %v", err)
	}
}

func TestHandOff_StillRunningAtTheTimeoutIsJobTimeout(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"POST fetchyr/authenticate": jsonHandler(202, handoff202()),
		handoffPoll:                 jsonHandler(200, map[string]interface{}{"job_id": "job-9", "status": "running"}),
	})
	defer server.Close()
	client, _ := New(Config{APIKey: testAPIKey, OrganizationID: testOrgID, BaseURL: server.URL, MaxRetries: -1,
		HTTPClient: server.Client(), PollInterval: time.Millisecond, PollTimeout: 10 * time.Millisecond})
	_, err := client.Fetchyr.Authenticate(context.Background(), "acme", "https://x")
	var apiErr *ApiError
	if !errorsAs(err, &apiErr) || apiErr.Code != "job_timeout" || apiErr.JobID != "job-9" {
		t.Fatalf("expected job_timeout, got %v", err)
	}
}

func TestHandOff_A202WithoutPollURLIsAnError(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"POST nexus/analyze": jsonHandler(202, map[string]interface{}{"status": "running"}),
	})
	defer server.Close()
	_, err := newPollingClient(server).Nexus.Analyze(context.Background(), "https://x", "<html/>", nil)
	var apiErr *ApiError
	if !errorsAs(err, &apiErr) || apiErr.Code != "unexpected_handoff" {
		t.Fatalf("expected unexpected_handoff, got %v", err)
	}
}

func TestHandOff_APollURLOffTheAPIHostIsRefused(t *testing.T) {
	body := handoff202()
	body["poll_url"] = "https://elsewhere.example/steal"
	_, server := newRouteServer(map[string]http.HandlerFunc{"POST nexus/synthesize": jsonHandler(202, body)})
	defer server.Close()
	_, err := newPollingClient(server).Nexus.Synthesize(context.Background(), "https://x", "<html/>", "g")
	if err == nil || !strings.Contains(err.Error(), "not on the API host") {
		t.Fatalf("expected a refusal, got %v", err)
	}
}

func TestHandOff_BackoffGrowsAndIsCapped(t *testing.T) {
	waits := []time.Duration{DefaultPollInterval}
	for i := 0; i < 7; i++ {
		waits = append(waits, nextPollInterval(waits[len(waits)-1]))
	}
	if waits[0] != 3*time.Second || waits[1] != 4500*time.Millisecond || waits[7] != 15*time.Second {
		t.Errorf("unexpected schedule %v", waits)
	}
}

func TestHandOff_GetHandoffAndWaitForHandoff(t *testing.T) {
	polls := 0
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"GET handoff/job-7": func(w http.ResponseWriter, r *http.Request) {
			polls++
			if polls < 3 {
				jsonHandler(200, map[string]interface{}{"job_id": "job-7", "kind": "kolektr_extract", "status": "running"})(w, r)
				return
			}
			jsonHandler(200, map[string]interface{}{"job_id": "job-7", "kind": "kolektr_extract", "status": "completed",
				"result": map[string]interface{}{"success": true, "total_records": float64(4)}})(w, r)
		},
	})
	defer server.Close()
	c := newTestClient(server)

	job, err := c.GetHandoff(context.Background(), "job-7")
	if err != nil || job.Status != "running" || job.Kind != "kolektr_extract" || job.Result != nil {
		t.Fatalf("unexpected job: %+v %v", job, err)
	}
	result, err := c.WaitForHandoff(context.Background(), "job-7", &PollOptions{Interval: time.Millisecond})
	if err != nil || result["total_records"] != float64(4) || polls != 3 {
		t.Fatalf("unexpected: %v %v (polls %d)", result, err, polls)
	}
}

func TestHandOff_GetHandoffNotFound(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"GET handoff/gone": jsonHandler(404, map[string]interface{}{"detail": "Job gone not found or expired"}),
	})
	defer server.Close()
	_, err := newTestClient(server).GetHandoff(context.Background(), "gone")
	var apiErr *ApiError
	if !errorsAs(err, &apiErr) || apiErr.StatusCode != 404 {
		t.Fatalf("expected a 404 ApiError, got %v", err)
	}
	_, err = newTestClient(server).WaitForHandoff(context.Background(), "gone", &PollOptions{Interval: time.Millisecond})
	if !errorsAs(err, &apiErr) || apiErr.Code != "job_failed" || apiErr.JobID != "gone" {
		t.Fatalf("expected job_failed with the job id, got %v", err)
	}
}
