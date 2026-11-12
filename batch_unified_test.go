package kloakd

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Tests for the batch job and unified scrape methods, against kobe-scraper
// integration/2026-11-12b (228857dc): kernel/jobs_runs_router.py (BatchJob,
// create_batch_job, pause/resume/cancel_batch_job, get_batch_job_results),
// application/jobs/batch_control.py and
// api/v1/endpoints/unified_scraping.py.

const pausedText = "paused: items that were already running finish; the rest wait until the batch is resumed."

func batchJob(status string, message interface{}) map[string]interface{} {
	return map[string]interface{}{
		"id": "b1", "name": "Batch 3 URLs", "status": status, "total_items": float64(3),
		"completed_items": float64(1), "created_at": "2026-11-12T10:00:00+00:00", "message": message,
	}
}

func refusedTransition(action, status, message string) map[string]interface{} {
	return detail("invalid_batch_transition", message, map[string]interface{}{"action": action, "status": status})
}

// retryingClient retries as a default client does, without the waits.
func retryingClient(server *httptest.Server) *Client {
	c, err := New(Config{APIKey: testAPIKey, OrganizationID: testOrgID, BaseURL: server.URL, MaxRetries: 3, HTTPClient: server.Client()})
	if err != nil {
		panic(err)
	}
	return c
}

// ─── create, read, list, results ─────────────────────────────────────────────

func TestBatch_Create_PostsTheURLsAndOnlyTheOptionsGiven(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST batch-jobs": jsonHandler(201, batchJob("pending", nil)),
	})
	defer server.Close()

	job, err := newTestClient(server).CreateBatchJob(context.Background(), []string{"https://a.example", "https://b.example"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]interface{}{"urls": []interface{}{"https://a.example", "https://b.example"}}
	if !reflect.DeepEqual(rs.bodies["POST batch-jobs"], want) {
		t.Errorf("unexpected body: %v", rs.bodies["POST batch-jobs"])
	}
	if job.ID != "b1" || job.Name != "Batch 3 URLs" || job.Status != "pending" || job.TotalItems != 3 ||
		job.CompletedItems != 1 || job.CreatedAt != "2026-11-12T10:00:00+00:00" || job.Message != nil {
		t.Errorf("unexpected batch: %+v", job)
	}
}

func TestBatch_Create_SendsTheOptionsUnderTheAPINames(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST batch-jobs": jsonHandler(201, batchJob("pending", nil)),
	})
	defer server.Close()

	_, err := newTestClient(server).CreateBatchJob(context.Background(), []string{"https://a.example"}, &BatchJobOptions{
		ExtractionConfig: map[string]interface{}{"selectors": map[string]interface{}{"title": "h1"}},
		CrawlID:          "c1", EntityType: "products", Fields: []string{"title", "price"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]interface{}{
		"urls":              []interface{}{"https://a.example"},
		"extraction_config": map[string]interface{}{"selectors": map[string]interface{}{"title": "h1"}},
		"crawl_id":          "c1", "entity_type": "products", "fields": []interface{}{"title", "price"},
	}
	if !reflect.DeepEqual(rs.bodies["POST batch-jobs"], want) {
		t.Errorf("unexpected body: %v", rs.bodies["POST batch-jobs"])
	}
}

func TestBatch_Create_OverTheRunningBatchesLimitIsConcurrencyLimitErrorAndNotRetried(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST batch-jobs": jsonHandler(409, detail("concurrent_limit", "Your plan runs 2 batches at once.",
			map[string]interface{}{"tier": "free", "concurrent_limit": float64(2), "active_batches": float64(2)})),
	})
	defer server.Close()

	_, err := retryingClient(server).CreateBatchJob(context.Background(), []string{"https://a.example"}, nil)
	var e *ConcurrencyLimitError
	if !errorsAs(err, &e) || e.Code != "concurrent_limit" || e.ConcurrentLimit != 2 || e.Tier != "free" {
		t.Fatalf("expected ConcurrencyLimitError, got %T %v", err, err)
	}
	// detail.active_batches (kernel/jobs_runs_router.py _check_batch_concurrency)
	if e.ActiveCrawls != 2 {
		t.Errorf("ActiveCrawls = %d, want 2 (detail.active_batches)", e.ActiveCrawls)
	}
	if len(rs.seen) != 1 {
		t.Errorf("expected one call, saw %v", rs.seen)
	}
}

func TestBatch_Create_ThatCouldNotBeQueuedIsThe503DispatchFailedAndNotRetried(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST batch-jobs": jsonHandler(503, detail("dispatch_failed", "The batch could not be queued. Nothing was started.",
			map[string]interface{}{"batch_job_id": "b1"})),
	})
	defer server.Close()

	_, err := retryingClient(server).CreateBatchJob(context.Background(), []string{"https://a.example"}, nil)
	var e *ApiError
	if !errorsAs(err, &e) || e.StatusCode != 503 || e.Code != "dispatch_failed" ||
		e.Message != "The batch could not be queued. Nothing was started." {
		t.Fatalf("expected the API's 503 dispatch_failed, got %T %v", err, err)
	}
	if len(rs.seen) != 1 {
		t.Errorf("a POST is not sent again: %v", rs.seen)
	}
}

func TestBatch_Create_ATestKeyAndTooManyURLsKeepTheAPICodes(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"POST batch-jobs": jsonHandler(403, detail("test_key_not_allowed", "Batch jobs need a live API key.", nil)),
	})
	_, err := newTestClient(server).CreateBatchJob(context.Background(), []string{"https://a.example"}, nil)
	server.Close()
	var forbidden *ForbiddenError
	if !errorsAs(err, &forbidden) || forbidden.Code != "test_key_not_allowed" {
		t.Fatalf("expected ForbiddenError test_key_not_allowed, got %T %v", err, err)
	}

	_, server = newRouteServer(map[string]http.HandlerFunc{
		"POST batch-jobs": jsonHandler(413, detail("batch_too_large", "Your plan allows 100 URLs in a batch.",
			map[string]interface{}{"tier": "free", "max_items": float64(100), "requested_items": float64(101)})),
	})
	defer server.Close()
	_, err = newTestClient(server).CreateBatchJob(context.Background(), []string{"https://a.example"}, nil)
	var apiErr *ApiError
	if !errorsAs(err, &apiErr) || apiErr.StatusCode != 413 || apiErr.Code != "batch_too_large" {
		t.Fatalf("expected the API's 413 batch_too_large, got %T %v", err, err)
	}
}

func TestBatch_Get_APausedBatchCarriesTheAPIMessage(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"GET batch-jobs/b1": jsonHandler(200, batchJob("paused", pausedText)),
	})
	defer server.Close()

	job, err := newTestClient(server).GetBatchJob(context.Background(), "b1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job.Status != "paused" || job.Message == nil || *job.Message != pausedText {
		t.Errorf("unexpected batch: %+v", job)
	}
	if len(rs.seen) != 1 || rs.seen[0] != "GET batch-jobs/b1" {
		t.Errorf("unexpected calls: %v", rs.seen)
	}
}

func TestBatch_Get_NotYoursIsThe404(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"GET batch-jobs/nope": jsonHandler(404, map[string]interface{}{"detail": "Batch job not found"}),
	})
	defer server.Close()

	_, err := newTestClient(server).GetBatchJob(context.Background(), "nope")
	var e *ApiError
	if !errorsAs(err, &e) || e.StatusCode != 404 || e.Message != "Batch job not found" {
		t.Fatalf("expected the API's 404, got %T %v", err, err)
	}
}

func TestBatch_List_SendsPageAndPageSizeOnlyWhenGiven(t *testing.T) {
	var queries []string
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"GET batch-jobs": func(w http.ResponseWriter, r *http.Request) {
			queries = append(queries, r.URL.RawQuery)
			jsonHandler(200, map[string]interface{}{
				"total": float64(41), "batch_jobs": []interface{}{batchJob("running", nil), batchJob("paused", pausedText)},
			})(w, r)
		},
	})
	defer server.Close()
	client := newTestClient(server)

	page, err := client.ListBatchJobs(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Total != 41 || len(page.BatchJobs) != 2 || page.BatchJobs[1].Status != "paused" || page.BatchJobs[0].Message != nil {
		t.Errorf("unexpected page: %+v", page)
	}
	if _, err := client.ListBatchJobs(context.Background(), &ListBatchJobsOptions{Page: 3, PageSize: 50}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if queries[0] != "" || queries[1] != "page=3&page_size=50" {
		t.Errorf("unexpected queries: %q", queries)
	}
}

func TestBatch_Results_ReadsTheItemsAndTheCountsAndPages(t *testing.T) {
	var queries []string
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"GET batch-jobs/b1/results": func(w http.ResponseWriter, r *http.Request) {
			queries = append(queries, r.URL.RawQuery)
			jsonHandler(200, map[string]interface{}{
				"batch_job_id": "b1",
				"results": []interface{}{
					map[string]interface{}{"job_id": "j1", "url": "https://a.example", "status": "completed",
						"extracted_data": map[string]interface{}{"title": "A"}, "api_data": map[string]interface{}{"records": []interface{}{}}},
					map[string]interface{}{"job_id": "j2", "url": nil, "status": "pending", "extracted_data": nil, "api_data": nil},
				},
				"total_jobs": float64(3), "completed_jobs": float64(1), "failed_jobs": float64(0),
			})(w, r)
		},
	})
	defer server.Close()
	client := newTestClient(server)

	results, err := client.GetBatchJobResults(context.Background(), "b1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results.BatchJobID != "b1" || results.TotalJobs != 3 || results.CompletedJobs != 1 || results.FailedJobs != 0 || len(results.Results) != 2 {
		t.Fatalf("unexpected results: %+v", results)
	}
	first, second := results.Results[0], results.Results[1]
	if first.JobID != "j1" || first.URL == nil || *first.URL != "https://a.example" || first.Status != "completed" ||
		!reflect.DeepEqual(first.ExtractedData, map[string]interface{}{"title": "A"}) || first.APIData == nil {
		t.Errorf("unexpected first item: %+v", first)
	}
	if second.URL != nil || second.ExtractedData != nil || second.APIData != nil || second.Status != "pending" {
		t.Errorf("unexpected second item: %+v", second)
	}
	if _, err := client.GetBatchJobResults(context.Background(), "b1", &BatchJobResultsOptions{Limit: 2, Offset: 4}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if queries[0] != "" || queries[1] != "limit=2&offset=4" {
		t.Errorf("unexpected queries: %q", queries)
	}
}

// ─── pause, resume, cancel ───────────────────────────────────────────────────

type batchControl struct {
	name    string
	call    func(*Client, context.Context, string) (*BatchJob, error)
	after   string      // the status the control leaves
	message interface{} // the message the API sends with it
	refused string      // a status the control is refused for
	reason  string      // the API's message for that refusal
}

var batchControls = []batchControl{
	{"pause", (*Client).PauseBatchJob, "paused", pausedText, "completed",
		"Only a running batch can be paused (current status: completed)."},
	{"resume", (*Client).ResumeBatchJob, "running", nil, "cancelled",
		"Only a paused batch can be resumed (current status: cancelled)."},
	{"cancel", (*Client).CancelBatchJob, "cancelled", nil, "completed",
		"A batch that has completed cannot be cancelled."},
}

func TestBatch_Controls_Succeed(t *testing.T) {
	for _, control := range batchControls {
		t.Run(control.name, func(t *testing.T) {
			rs, server := newRouteServer(map[string]http.HandlerFunc{
				"POST batch-jobs/b1/" + control.name: jsonHandler(200, batchJob(control.after, control.message)),
			})
			defer server.Close()

			job, err := control.call(newTestClient(server), context.Background(), "b1")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if job.ID != "b1" || job.Status != control.after {
				t.Errorf("unexpected batch: %+v", job)
			}
			if control.message == nil != (job.Message == nil) {
				t.Errorf("unexpected message: %v", job.Message)
			}
			if len(rs.seen) != 1 || rs.seen[0] != "POST batch-jobs/b1/"+control.name {
				t.Errorf("unexpected calls: %v", rs.seen)
			}
		})
	}
}

// The API answers a repeated control with 200 and the unchanged batch
// (application/jobs/batch_control.py): the SDK returns it both times.
func TestBatch_Controls_ARepeatIs200(t *testing.T) {
	for _, control := range batchControls {
		t.Run(control.name, func(t *testing.T) {
			rs, server := newRouteServer(map[string]http.HandlerFunc{
				"POST batch-jobs/b1/" + control.name: jsonHandler(200, batchJob(control.after, control.message)),
			})
			defer server.Close()
			client := newTestClient(server)

			first, err := control.call(client, context.Background(), "b1")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			second, err := control.call(client, context.Background(), "b1")
			if err != nil {
				t.Fatalf("a repeat is not an error: %v", err)
			}
			if first.Status != control.after || second.Status != control.after || len(rs.seen) != 2 {
				t.Errorf("unexpected repeat: %+v %+v %v", first, second, rs.seen)
			}
		})
	}
}

func TestBatch_Controls_ARefusedTransitionIsThe409WithTheAPICode(t *testing.T) {
	for _, control := range batchControls {
		t.Run(control.name, func(t *testing.T) {
			rs, server := newRouteServer(map[string]http.HandlerFunc{
				"POST batch-jobs/b1/" + control.name: jsonHandler(409, refusedTransition(control.name, control.refused, control.reason)),
			})
			defer server.Close()

			_, err := control.call(retryingClient(server), context.Background(), "b1")
			var e *ApiError
			if !errorsAs(err, &e) || e.StatusCode != 409 || e.Code != "invalid_batch_transition" || e.Message != control.reason {
				t.Fatalf("expected the API's 409 invalid_batch_transition, got %T %v", err, err)
			}
			var concurrency *ConcurrencyLimitError
			if errorsAs(err, &concurrency) {
				t.Error("a refused transition is not a ConcurrencyLimitError")
			}
			if len(rs.seen) != 1 {
				t.Errorf("a refusal is not retried: %v", rs.seen)
			}
		})
	}
}

func TestBatch_Resume_ABatchHeldForQuotaIsRefusedWithTheAPIReason(t *testing.T) {
	reason := "This batch is held because your plan's quota ran out; it resumes by itself when quota returns."
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"POST batch-jobs/b1/resume": jsonHandler(409, refusedTransition("resume", "held_quota", reason)),
	})
	defer server.Close()

	_, err := newTestClient(server).ResumeBatchJob(context.Background(), "b1")
	var e *ApiError
	if !errorsAs(err, &e) || e.Code != "invalid_batch_transition" || e.Message != reason {
		t.Fatalf("expected the API's refusal, got %T %v", err, err)
	}
}

func TestBatch_Resume_ThatCannotQueueIsThe503DispatchFailedAndNotRetried(t *testing.T) {
	message := "The batch could not be resumed: its items could not be queued. It is paused again; resume it again."
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST batch-jobs/b1/resume": jsonHandler(503, detail("dispatch_failed", message,
			map[string]interface{}{"batch_job_id": "b1", "items_not_queued": float64(2)})),
	})
	defer server.Close()

	_, err := retryingClient(server).ResumeBatchJob(context.Background(), "b1")
	var e *ApiError
	if !errorsAs(err, &e) || e.StatusCode != 503 || e.Code != "dispatch_failed" || e.Message != message {
		t.Fatalf("expected the API's 503 dispatch_failed, got %T %v", err, err)
	}
	var billing *BillingUnavailableError
	if errorsAs(err, &billing) {
		t.Error("dispatch_failed is not BillingUnavailableError")
	}
	if len(rs.seen) != 1 {
		t.Errorf("a POST is not sent again: %v", rs.seen)
	}
}

func TestBatch_Resume_OverTheRunningBatchesLimitIsConcurrencyLimitError(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"POST batch-jobs/b1/resume": jsonHandler(409, detail("concurrent_limit", "Your plan runs 2 batches at once.",
			map[string]interface{}{"tier": "free", "concurrent_limit": float64(2), "active_batches": float64(2)})),
	})
	defer server.Close()

	_, err := newTestClient(server).ResumeBatchJob(context.Background(), "b1")
	var e *ConcurrencyLimitError
	if !errorsAs(err, &e) || e.ConcurrentLimit != 2 {
		t.Fatalf("expected ConcurrencyLimitError, got %T %v", err, err)
	}
}

func TestBatch_Controls_NotYoursIsThe404(t *testing.T) {
	for _, control := range batchControls {
		t.Run(control.name, func(t *testing.T) {
			_, server := newRouteServer(map[string]http.HandlerFunc{
				"POST batch-jobs/nope/" + control.name: jsonHandler(404, map[string]interface{}{"detail": "Batch job not found"}),
			})
			defer server.Close()

			_, err := control.call(newTestClient(server), context.Background(), "nope")
			var e *ApiError
			if !errorsAs(err, &e) || e.StatusCode != 404 {
				t.Fatalf("expected the API's 404, got %T %v", err, err)
			}
		})
	}
}

// ─── unified scrape ──────────────────────────────────────────────────────────

func unifiedAnswer() map[string]interface{} {
	return map[string]interface{}{
		"success": true, "job_id": "u1", "level": "level1", "status": "completed",
		"escalation_path": []interface{}{}, "message": "Extraction completed at level 1",
		"metadata": map[string]interface{}{"items": float64(4)}, "auth_required": false, "discovery_id": nil,
	}
}

func TestUnifiedScrape_PostsTheURLAloneWhenNoOptionIsGiven(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{"POST scraping/unified": jsonHandler(200, unifiedAnswer())})
	defer server.Close()

	result, err := newTestClient(server).UnifiedScrape(context.Background(), "https://example.com", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(rs.bodies["POST scraping/unified"], map[string]interface{}{"url": "https://example.com"}) {
		t.Errorf("unexpected body: %v", rs.bodies["POST scraping/unified"])
	}
	if !result.Success || result.JobID != "u1" || result.Level != "level1" || result.Status != "completed" ||
		len(result.EscalationPath) != 0 || result.Message != "Extraction completed at level 1" ||
		result.Metadata["items"] != float64(4) || result.AuthRequired || result.DiscoveryID != nil {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestUnifiedScrape_SendsTheRequestFieldsOfTheRouteAndNothingElse(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{"POST scraping/unified": jsonHandler(200, unifiedAnswer())})
	defer server.Close()

	_, err := newTestClient(server).UnifiedScrape(context.Background(), "https://example.com", &UnifiedScrapeOptions{
		Prompt: "the products", Mode: "level2", Schema: map[string]string{"title": "the product's name"},
		Options: map[string]interface{}{"timeout": float64(30)}, Context: map[string]interface{}{"site": "shop"},
		Constraints: map[string]interface{}{"max_pages": float64(2)}, SessionArtifactID: "s1",
		PollInterval: time.Millisecond, PollTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]interface{}{
		"url": "https://example.com", "prompt": "the products", "mode": "level2",
		"schema":  map[string]interface{}{"title": "the product's name"},
		"options": map[string]interface{}{"timeout": float64(30)}, "context": map[string]interface{}{"site": "shop"},
		"constraints": map[string]interface{}{"max_pages": float64(2)}, "session_artifact_id": "s1",
	}
	if !reflect.DeepEqual(rs.bodies["POST scraping/unified"], want) {
		t.Errorf("unexpected body: %v", rs.bodies["POST scraping/unified"])
	}
}

func TestUnifiedScrape_ALoginPageFoundFirstIsAnAnswerNotAnError(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{"POST scraping/unified": jsonHandler(200, map[string]interface{}{
		"success": true, "job_id": "x", "level": "preflight", "status": "auth_required", "escalation_path": []interface{}{},
		"message": "This URL needs a login.", "metadata": nil, "auth_required": true, "discovery_id": "d1",
	})})
	defer server.Close()

	result, err := newTestClient(server).UnifiedScrape(context.Background(), "https://example.com/account", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.AuthRequired || result.Status != "auth_required" || result.Level != "preflight" ||
		result.DiscoveryID == nil || *result.DiscoveryID != "d1" || result.Metadata != nil {
		t.Errorf("unexpected result: %+v", result)
	}
}

func unifiedHandoff() map[string]interface{} {
	return map[string]interface{}{
		"job_id": "h1", "status": "running", "reason": "sync_limit_reached", "waited_seconds": float64(85),
		"poll_url": "/api/v1/organizations/" + testOrgID + "/handoff/h1", "message": "still running",
	}
}

func TestUnifiedScrape_A202HandOffIsFollowedToTheRoutesOwnAnswer(t *testing.T) {
	polls := 0
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST scraping/unified": jsonHandler(202, unifiedHandoff()),
		"GET handoff/h1": func(w http.ResponseWriter, r *http.Request) {
			polls++
			if polls < 2 {
				jsonHandler(200, map[string]interface{}{"job_id": "h1", "status": "running"})(w, r)
				return
			}
			jsonHandler(200, map[string]interface{}{"job_id": "h1", "status": "completed", "result": unifiedAnswer()})(w, r)
		},
	})
	defer server.Close()

	result, err := newTestClient(server).UnifiedScrape(context.Background(), "https://example.com",
		&UnifiedScrapeOptions{PollInterval: time.Millisecond})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.JobID != "u1" || result.Status != "completed" || polls != 2 || rs.seen[0] != "POST scraping/unified" {
		t.Errorf("unexpected result %+v after %v", result, rs.seen)
	}
}

func TestUnifiedScrape_AHandedOffScrapeThatFailsIsJobFailedWithTheJobID(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"POST scraping/unified": jsonHandler(202, unifiedHandoff()),
		"GET handoff/h1":        jsonHandler(200, map[string]interface{}{"job_id": "h1", "status": "failed", "error": "blocked"}),
	})
	defer server.Close()

	_, err := newTestClient(server).UnifiedScrape(context.Background(), "https://example.com",
		&UnifiedScrapeOptions{PollInterval: time.Millisecond})
	var e *ApiError
	if !errorsAs(err, &e) || e.Code != "job_failed" || e.Message != "blocked" || e.JobID != "h1" {
		t.Fatalf("expected job_failed, got %T %v", err, err)
	}
}

func TestUnifiedScrape_ATestKeyAndAMissingModuleAreForbiddenWithTheAPICode(t *testing.T) {
	for _, code := range []string{"test_key_not_allowed", "module_not_entitled"} {
		t.Run(code, func(t *testing.T) {
			_, server := newRouteServer(map[string]http.HandlerFunc{
				"POST scraping/unified": jsonHandler(403, detail(code, "refused", nil)),
			})
			defer server.Close()

			_, err := newTestClient(server).UnifiedScrape(context.Background(), "https://example.com", nil)
			var e *ForbiddenError
			if !errorsAs(err, &e) || e.Code != code {
				t.Fatalf("expected ForbiddenError %s, got %T %v", code, err, err)
			}
		})
	}
}

func TestUnifiedScrape_Get_ReadsARunningJobWithItsProgress(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{"GET scraping/unified/u1": jsonHandler(200, map[string]interface{}{
		"id": "u1", "status": "running", "level": "level2", "progress": float64(40),
		"progress_detail": map[string]interface{}{"percentage": float64(40), "current_step": "extracting",
			"items_found": float64(3), "items_target": nil, "partial_results": []interface{}{map[string]interface{}{"title": "A"}}},
		"created_at": "2026-11-12T10:00:00", "updated_at": "2026-11-12T10:00:05", "completed_at": nil,
		"error": nil, "result": nil, "escalation_path": []interface{}{},
	})})
	defer server.Close()

	status, err := newTestClient(server).GetUnifiedScrape(context.Background(), "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.ID != "u1" || status.Status != "running" || status.Level != "level2" || status.Progress == nil || *status.Progress != 40 ||
		status.CreatedAt != "2026-11-12T10:00:00" || status.UpdatedAt == nil || status.CompletedAt != nil ||
		status.Error != nil || status.Result != nil || len(status.EscalationPath) != 0 {
		t.Errorf("unexpected status: %+v", status)
	}
	detail := status.ProgressDetail
	if detail == nil || detail.Percentage != 40 || detail.CurrentStep != "extracting" || detail.ItemsFound == nil ||
		*detail.ItemsFound != 3 || detail.ItemsTarget != nil || len(detail.PartialResults) != 1 || detail.PartialResults[0]["title"] != "A" {
		t.Errorf("unexpected progress: %+v", detail)
	}
	if rs.seen[0] != "GET scraping/unified/u1" {
		t.Errorf("unexpected calls: %v", rs.seen)
	}
}

func TestUnifiedScrape_Get_ReadsACompletedJobWithItsResultAndEscalation(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{"GET scraping/unified/u1": jsonHandler(200, map[string]interface{}{
		"id": "u1", "status": "completed", "level": "level2", "progress": float64(100), "progress_detail": nil,
		"created_at": "2026-11-12T10:00:00", "updated_at": "2026-11-12T10:01:00", "completed_at": "2026-11-12T10:01:00",
		"error": nil, "result": map[string]interface{}{"data": []interface{}{map[string]interface{}{"title": "A"}}},
		"escalation_path": []interface{}{map[string]interface{}{"from": "level1", "to": "level2", "reason": "no items", "timestamp": "2026-11-12T10:00:20"}},
	})})
	defer server.Close()

	status, err := newTestClient(server).GetUnifiedScrape(context.Background(), "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Status != "completed" || status.ProgressDetail != nil || status.CompletedAt == nil || status.Result["data"] == nil {
		t.Errorf("unexpected status: %+v", status)
	}
	want := []UnifiedEscalation{{From: "level1", To: "level2", Reason: "no items", Timestamp: "2026-11-12T10:00:20"}}
	if !reflect.DeepEqual(status.EscalationPath, want) {
		t.Errorf("unexpected escalation: %+v", status.EscalationPath)
	}
}

func TestUnifiedScrape_Get_NotYoursIsThe404(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"GET scraping/unified/nope": jsonHandler(404, map[string]interface{}{"detail": "Job nope not found"}),
	})
	defer server.Close()

	_, err := newTestClient(server).GetUnifiedScrape(context.Background(), "nope")
	var e *ApiError
	if !errorsAs(err, &e) || e.StatusCode != 404 || e.Message != "Job nope not found" {
		t.Fatalf("expected the API's 404, got %T %v", err, err)
	}
}

// ─── the route list ──────────────────────────────────────────────────────────

func TestContract_BatchAndUnifiedScrapeRoutesAreLiveAndCalled(t *testing.T) {
	live, calls := liveRoutes(t), sdkCalls(t)
	for _, key := range []string{
		"POST batch-jobs", "GET batch-jobs", "GET batch-jobs/{}", "GET batch-jobs/{}/results",
		"POST batch-jobs/{}/pause", "POST batch-jobs/{}/resume", "POST batch-jobs/{}/cancel",
		"POST scraping/unified", "GET scraping/unified/{}",
	} {
		if !live[key] {
			t.Errorf("%s missing from testdata/routes.tsv", key)
		}
		if !calls[key] {
			t.Errorf("the SDK does not call %s", key)
		}
	}
}

// testdata/routes.tsv is the route list of kobe-scraper
// integration/2026-11-12b (228857dc): 280 routes, without the 12
// performance routes the API removed.
func TestContract_TheRouteListIsTheIntegrationBranchs(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "routes.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		rows++
		if strings.Contains(sc.Text(), "/performance/") {
			t.Errorf("a removed performance route is still listed: %s", sc.Text())
		}
	}
	if rows != 280 {
		t.Errorf("expected 280 routes, found %d", rows)
	}
}
