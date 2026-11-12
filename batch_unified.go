package kloakd

import (
	"context"
	"time"
)

// ─── batch jobs (batch-jobs) ─────────────────────────────────────────────────

// BatchJob is a batch job (kernel/jobs_runs_router.py BatchJob).
type BatchJob struct {
	ID   string
	Name string
	// Status is pending, running, paused, held_quota, completed, failed or
	// cancelled. A paused batch starts no further item until it is resumed;
	// items that were already running finish.
	Status     string
	TotalItems int
	// CompletedItems counts the items that completed or failed.
	CompletedItems int
	CreatedAt      string
	// Message is the API's note for a paused batch or one held for quota;
	// nil otherwise.
	Message *string
	Raw     map[string]interface{}
}

// BatchJobsPage is one page of the organisation's batch jobs, newest first
// (BatchJobsList).
type BatchJobsPage struct {
	// Total counts all of the organisation's batch jobs, not only this page.
	Total     int
	BatchJobs []BatchJob
}

// BatchJobItem is one item (URL) of a batch, as batch-jobs/{id}/results
// lists it.
type BatchJobItem struct {
	JobID string
	URL   *string
	// Status is pending, running, held_quota, completed, failed or cancelled.
	Status string
	// ExtractedData is what the item extracted; nil until it completes.
	ExtractedData interface{}
	// APIData holds the records of a detected API the item's URL names; nil
	// otherwise.
	APIData map[string]interface{}
}

// BatchJobResults is a batch's items and their results (BatchJobResults).
type BatchJobResults struct {
	BatchJobID string
	// Results are the items asked for: all of them, or Limit from Offset, in
	// creation order.
	Results []BatchJobItem
	// TotalJobs, CompletedJobs and FailedJobs count the whole batch when
	// Limit was given; without Limit (and with an Offset) they count the
	// items returned.
	TotalJobs     int
	CompletedJobs int
	FailedJobs    int
}

// BatchJobOptions are the optional fields of Client.CreateBatchJob (the body
// of POST batch-jobs).
type BatchJobOptions struct {
	// ExtractionConfig says how each URL is extracted, for example
	// {"selectors": {"title": "h1"}}.
	ExtractionConfig map[string]interface{}
	// CrawlID is the crawl the URLs came from: its detected-API records are
	// added to the results.
	CrawlID string
	// EntityType is what each page holds, for example "products".
	EntityType string
	// Fields are the names of the fields to extract.
	Fields []string
}

// ListBatchJobsOptions pages Client.ListBatchJobs. Zero values mean the
// API's defaults (page 1, 20 a page; PageSize is 1 to 100).
type ListBatchJobsOptions struct {
	Page     int
	PageSize int
}

// BatchJobResultsOptions pages Client.GetBatchJobResults. A zero Limit
// means every item; Limit is 1 to 1000.
type BatchJobResultsOptions struct {
	Limit  int
	Offset int
}

func parseBatchJob(raw map[string]interface{}) *BatchJob {
	return &BatchJob{
		ID:             strField(raw, "id"),
		Name:           strField(raw, "name"),
		Status:         strField(raw, "status"),
		TotalItems:     intField(raw, "total_items"),
		CompletedItems: intField(raw, "completed_items"),
		CreatedAt:      strField(raw, "created_at"),
		Message:        optStrField(raw, "message"),
		Raw:            raw,
	}
}

func rawMaps(v interface{}) []map[string]interface{} {
	items, _ := v.([]interface{})
	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]interface{}); ok {
			out = append(out, m)
		}
	}
	return out
}

// CreateBatchJob creates a batch job: one extraction per URL (POST
// batch-jobs, answered 201). The batch holds one work slot until it ends.
// It is refused with 403 "test_key_not_allowed" for a test key
// (*ForbiddenError), 413 "batch_too_large" over the tier's URL limit, 422
// for an empty list, *ConcurrencyLimitError (409) at the tier's running
// batches, and 503 "dispatch_failed" when it could not be queued (nothing
// was started). It is never retried.
func (c *Client) CreateBatchJob(ctx context.Context, urls []string, opts *BatchJobOptions) (*BatchJob, error) {
	if urls == nil {
		urls = []string{}
	}
	body := map[string]interface{}{"urls": urls}
	if opts != nil {
		if opts.ExtractionConfig != nil {
			body["extraction_config"] = opts.ExtractionConfig
		}
		if opts.CrawlID != "" {
			body["crawl_id"] = opts.CrawlID
		}
		if opts.EntityType != "" {
			body["entity_type"] = opts.EntityType
		}
		if opts.Fields != nil {
			body["fields"] = opts.Fields
		}
	}
	raw, err := c.t.post(ctx, "batch-jobs", body)
	if err != nil {
		return nil, err
	}
	return parseBatchJob(raw), nil
}

// GetBatchJob reads a batch job's state (GET batch-jobs/{batch_job_id}). A
// batch that is not yours is an *ApiError with StatusCode 404.
func (c *Client) GetBatchJob(ctx context.Context, batchJobID string) (*BatchJob, error) {
	raw, err := c.t.get(ctx, pathf("batch-jobs/%s", batchJobID), nil)
	if err != nil {
		return nil, err
	}
	return parseBatchJob(raw), nil
}

// ListBatchJobs lists the organisation's batch jobs, newest first (GET
// batch-jobs).
func (c *Client) ListBatchJobs(ctx context.Context, opts *ListBatchJobsOptions) (*BatchJobsPage, error) {
	params := map[string]string{}
	if opts != nil {
		setInt(params, "page", opts.Page)
		setInt(params, "page_size", opts.PageSize)
	}
	raw, err := c.t.get(ctx, "batch-jobs", params)
	if err != nil {
		return nil, err
	}
	page := &BatchJobsPage{Total: intField(raw, "total"), BatchJobs: []BatchJob{}}
	for _, item := range rawMaps(raw["batch_jobs"]) {
		page.BatchJobs = append(page.BatchJobs, *parseBatchJob(item))
	}
	return page, nil
}

// GetBatchJobResults lists a batch's items with what each extracted (GET
// batch-jobs/{batch_job_id}/results): every item without a Limit; with a
// Limit, that many from Offset, and the counts are then of the whole batch.
// To download the rows use ExportBatchJob.
func (c *Client) GetBatchJobResults(ctx context.Context, batchJobID string, opts *BatchJobResultsOptions) (*BatchJobResults, error) {
	params := map[string]string{}
	if opts != nil {
		setInt(params, "limit", opts.Limit)
		setInt(params, "offset", opts.Offset)
	}
	raw, err := c.t.get(ctx, pathf("batch-jobs/%s/results", batchJobID), params)
	if err != nil {
		return nil, err
	}
	results := &BatchJobResults{
		BatchJobID:    strField(raw, "batch_job_id"),
		Results:       []BatchJobItem{},
		TotalJobs:     intField(raw, "total_jobs"),
		CompletedJobs: intField(raw, "completed_jobs"),
		FailedJobs:    intField(raw, "failed_jobs"),
	}
	if results.BatchJobID == "" {
		results.BatchJobID = batchJobID
	}
	for _, item := range rawMaps(raw["results"]) {
		results.Results = append(results.Results, BatchJobItem{
			JobID:         strField(item, "job_id"),
			URL:           optStrField(item, "url"),
			Status:        strField(item, "status"),
			ExtractedData: item["extracted_data"],
			APIData:       optMapField(item, "api_data"),
		})
	}
	return results, nil
}

// PauseBatchJob pauses a running batch (POST
// batch-jobs/{batch_job_id}/pause). Items already running finish; no other
// item starts until ResumeBatchJob. Pausing a paused batch changes nothing.
// A batch in any other state is a 409 *ApiError with Code
// "invalid_batch_transition".
func (c *Client) PauseBatchJob(ctx context.Context, batchJobID string) (*BatchJob, error) {
	raw, err := c.t.post(ctx, pathf("batch-jobs/%s/pause", batchJobID), nil)
	if err != nil {
		return nil, err
	}
	return parseBatchJob(raw), nil
}

// ResumeBatchJob resumes a paused batch (POST
// batch-jobs/{batch_job_id}/resume): the items held while it was paused are
// queued again, each once. Resuming a running batch changes nothing. A
// batch in any other state is a 409 *ApiError with Code
// "invalid_batch_transition". It is gated like creation (quota, payment,
// *ConcurrencyLimitError at the tier's running batches). A 503 *ApiError
// with Code "dispatch_failed" means the items could not be queued and the
// batch is paused again: call it again.
func (c *Client) ResumeBatchJob(ctx context.Context, batchJobID string) (*BatchJob, error) {
	raw, err := c.t.post(ctx, pathf("batch-jobs/%s/resume", batchJobID), nil)
	if err != nil {
		return nil, err
	}
	return parseBatchJob(raw), nil
}

// CancelBatchJob cancels a batch and every item of it that has not ended
// (POST batch-jobs/{batch_job_id}/cancel). Completed and failed items keep
// their results. Cancelling a cancelled batch changes nothing. A batch that
// has completed or failed is a 409 *ApiError with Code
// "invalid_batch_transition".
func (c *Client) CancelBatchJob(ctx context.Context, batchJobID string) (*BatchJob, error) {
	raw, err := c.t.post(ctx, pathf("batch-jobs/%s/cancel", batchJobID), nil)
	if err != nil {
		return nil, err
	}
	return parseBatchJob(raw), nil
}

// ─── unified scrape (scraping/unified) ───────────────────────────────────────

// UnifiedScrapeOptions are the optional fields of Client.UnifiedScrape
// (unified_scraping.py UnifiedScrapeRequest) and how a 202 is followed.
type UnifiedScrapeOptions struct {
	// Prompt says what to extract, in plain words.
	Prompt string
	// Mode is "auto" (the API's default), "level1" or "level2".
	Mode string
	// Schema maps a field name to its description; made from Prompt when
	// omitted.
	Schema map[string]string
	// Options are scraping options (timeout, proxy and so on).
	Options map[string]interface{}
	// Context and Constraints are for the Strategy Engine.
	Context     map[string]interface{}
	Constraints map[string]interface{}
	// SessionArtifactID is a login session's id, to scrape behind a login.
	SessionArtifactID string
	// PollInterval and PollTimeout override Config.PollInterval /
	// PollTimeout for following a scrape the API hands off as a job.
	PollInterval time.Duration
	PollTimeout  time.Duration
}

// UnifiedEscalation is one step of a unified scrape's escalation
// (EscalationEvent).
type UnifiedEscalation struct {
	From      string
	To        string
	Reason    string
	Timestamp string
}

// UnifiedScrapeResult is the answer of Client.UnifiedScrape
// (unified_scraping.py UnifiedScrapeResponse).
type UnifiedScrapeResult struct {
	Success bool
	// JobID names the scrape's job: read it with Client.GetUnifiedScrape.
	JobID string
	// Level is level1, level2, or preflight when a login page was found
	// before any scrape.
	Level          string
	Status         string
	EscalationPath []UnifiedEscalation
	Message        string
	Metadata       map[string]interface{}
	// AuthRequired is true when the URL is behind a login: nothing was
	// scraped, Status is "auth_required" and DiscoveryID names the
	// auth-chain discovery (Fetchyr.GetAuthChainStatus). JobID then names
	// no stored job.
	AuthRequired bool
	DiscoveryID  *string
	Raw          map[string]interface{}
}

// UnifiedScrapeProgress is the progress of a running unified scrape
// (ProgressDetail).
type UnifiedScrapeProgress struct {
	Percentage  int
	CurrentStep string
	ItemsFound  *int
	ItemsTarget *int
	// PartialResults are the first few results, when there are any.
	PartialResults []map[string]interface{}
}

// UnifiedScrapeStatus is the state of a unified scrape job
// (unified_scraping.py JobStatusResponse).
type UnifiedScrapeStatus struct {
	ID       string
	Status   string
	Level    string
	Progress *int
	// ProgressDetail is set while the job is running.
	ProgressDetail *UnifiedScrapeProgress
	CreatedAt      string
	UpdatedAt      *string
	CompletedAt    *string
	Error          *string
	// Result is the job's stored result document; nil until there is one.
	Result         map[string]interface{}
	EscalationPath []UnifiedEscalation
	Raw            map[string]interface{}
}

func parseEscalations(v interface{}) []UnifiedEscalation {
	out := []UnifiedEscalation{}
	for _, e := range rawMaps(v) {
		out = append(out, UnifiedEscalation{
			From:      strField(e, "from"),
			To:        strField(e, "to"),
			Reason:    strField(e, "reason"),
			Timestamp: strField(e, "timestamp"),
		})
	}
	return out
}

// UnifiedScrape scrapes a URL and lets the API choose how (POST
// scraping/unified): level 1 (direct extraction) or level 2 (the Strategy
// Engine), with escalation when the first fails. It needs a live API key
// entitled to Kolektr and Nexus (403 "test_key_not_allowed",
// "module_not_entitled": *ForbiddenError). A request the API hands off as a
// job (202) is followed to its result. The answer names the job; read its
// state and data with GetUnifiedScrape. When the URL is behind a login the
// answer has AuthRequired true and nothing was scraped.
func (c *Client) UnifiedScrape(ctx context.Context, targetURL string, opts *UnifiedScrapeOptions) (*UnifiedScrapeResult, error) {
	body := map[string]interface{}{"url": targetURL}
	var poll *PollOptions
	if opts != nil {
		if opts.Prompt != "" {
			body["prompt"] = opts.Prompt
		}
		if opts.Mode != "" {
			body["mode"] = opts.Mode
		}
		if opts.Schema != nil {
			body["schema"] = opts.Schema
		}
		if opts.Options != nil {
			body["options"] = opts.Options
		}
		if opts.Context != nil {
			body["context"] = opts.Context
		}
		if opts.Constraints != nil {
			body["constraints"] = opts.Constraints
		}
		if opts.SessionArtifactID != "" {
			body["session_artifact_id"] = opts.SessionArtifactID
		}
		poll = &PollOptions{Interval: opts.PollInterval, Timeout: opts.PollTimeout}
	}
	raw, err := c.t.postFollowing(ctx, "scraping/unified", body, poll)
	if err != nil {
		return nil, err
	}
	success := true
	if b, ok := raw["success"].(bool); ok {
		success = b
	}
	return &UnifiedScrapeResult{
		Success:        success,
		JobID:          strField(raw, "job_id"),
		Level:          strField(raw, "level"),
		Status:         strField(raw, "status"),
		EscalationPath: parseEscalations(raw["escalation_path"]),
		Message:        strField(raw, "message"),
		Metadata:       optMapField(raw, "metadata"),
		AuthRequired:   boolField(raw, "auth_required"),
		DiscoveryID:    optStrField(raw, "discovery_id"),
		Raw:            raw,
	}, nil
}

// GetUnifiedScrape reads the state of a unified scrape job and, once it
// has one, its result (GET scraping/unified/{job_id}). A job that is not
// yours is an *ApiError with StatusCode 404.
func (c *Client) GetUnifiedScrape(ctx context.Context, jobID string) (*UnifiedScrapeStatus, error) {
	raw, err := c.t.get(ctx, pathf("scraping/unified/%s", jobID), nil)
	if err != nil {
		return nil, err
	}
	status := &UnifiedScrapeStatus{
		ID:             strField(raw, "id"),
		Status:         strField(raw, "status"),
		Level:          strField(raw, "level"),
		Progress:       optIntField(raw, "progress"),
		CreatedAt:      strField(raw, "created_at"),
		UpdatedAt:      optStrField(raw, "updated_at"),
		CompletedAt:    optStrField(raw, "completed_at"),
		Error:          optStrField(raw, "error"),
		Result:         optMapField(raw, "result"),
		EscalationPath: parseEscalations(raw["escalation_path"]),
		Raw:            raw,
	}
	if status.ID == "" {
		status.ID = jobID
	}
	if detail := optMapField(raw, "progress_detail"); detail != nil {
		progress := &UnifiedScrapeProgress{
			Percentage:  intField(detail, "percentage"),
			CurrentStep: strField(detail, "current_step"),
			ItemsFound:  optIntField(detail, "items_found"),
			ItemsTarget: optIntField(detail, "items_target"),
		}
		if _, ok := detail["partial_results"].([]interface{}); ok {
			progress.PartialResults = rawMaps(detail["partial_results"])
		}
		status.ProgressDetail = progress
	}
	return status, nil
}
