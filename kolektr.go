package kloakd

import (
	"context"
	"fmt"
)

// KolektrNamespace exposes the Data Extraction module.
// Access via client.Kolektr.
type KolektrNamespace struct {
	t *transport
}

// PageOptions configures optional parameters for Kolektr.Page.
type PageOptions struct {
	Schema                  map[string]interface{}
	FetchArtifactID         string
	SessionArtifactID       string
	ApiMapArtifactID        string
	Limit                   int
	Offset                  int
}

// Page extracts structured data from a URL using the full Kolektr pipeline.
func (k *KolektrNamespace) Page(ctx context.Context, targetURL string, opts *PageOptions) (*ExtractionResult, error) {
	body := map[string]interface{}{"url": targetURL}
	if opts != nil {
		if len(opts.Schema) > 0 {
			body["schema"] = opts.Schema
		}
		if opts.FetchArtifactID != "" {
			body["fetch_artifact_id"] = opts.FetchArtifactID
		}
		if opts.SessionArtifactID != "" {
			body["session_artifact_id"] = opts.SessionArtifactID
		}
		if opts.ApiMapArtifactID != "" {
			body["api_map_artifact_id"] = opts.ApiMapArtifactID
		}
		if opts.Limit > 0 {
			body["limit"] = opts.Limit
		}
		if opts.Offset > 0 {
			body["offset"] = opts.Offset
		}
	}

	raw, err := k.t.post(ctx, "kolektr/page", body)
	if err != nil {
		return nil, err
	}
	return parseExtractionResult(raw), nil
}

// PageAll auto-paginates until all records are retrieved.
func (k *KolektrNamespace) PageAll(ctx context.Context, targetURL string, opts *PageOptions) ([]map[string]interface{}, error) {
	limit := 100
	offset := 0
	var all []map[string]interface{}

	for {
		o := &PageOptions{}
		if opts != nil {
			*o = *opts
		}
		o.Limit = limit
		o.Offset = offset

		result, err := k.Page(ctx, targetURL, o)
		if err != nil {
			return nil, err
		}
		all = append(all, result.Records...)
		if !result.HasMore {
			break
		}
		offset += len(result.Records)
	}
	return all, nil
}

// ExtractHTMLOptions configures optional parameters for Kolektr.ExtractHTML.
type ExtractHTMLOptions struct {
	Schema map[string]interface{}
}

// ExtractHTML extracts structured data from raw HTML without making a network request.
func (k *KolektrNamespace) ExtractHTML(ctx context.Context, html, targetURL string, opts *ExtractHTMLOptions) (*ExtractionResult, error) {
	body := map[string]interface{}{
		"html": html,
		"url":  targetURL,
	}
	if opts != nil && len(opts.Schema) > 0 {
		body["schema"] = opts.Schema
	}

	raw, err := k.t.post(ctx, "kolektr/extract-html", body)
	if err != nil {
		return nil, err
	}
	return parseExtractionResult(raw), nil
}

// GetAPIData gets all extracted data for a discovered API endpoint.
func (k *KolektrNamespace) GetAPIData(ctx context.Context, apiEndpoint string) (map[string]interface{}, error) {
	return k.t.get(ctx, fmt.Sprintf("kolektr/api-data/%s", apiEndpoint), nil)
}

// GetAPIDataPaginated gets API data with pagination.
func (k *KolektrNamespace) GetAPIDataPaginated(ctx context.Context, apiEndpoint string, offset, limit int) (map[string]interface{}, error) {
	params := map[string]string{
		"offset": fmt.Sprintf("%d", offset),
		"limit":  fmt.Sprintf("%d", limit),
	}
	return k.t.get(ctx, fmt.Sprintf("kolektr/api-data/%s/paginated", apiEndpoint), params)
}

// ExtractAllAPIData extracts all data from a discovered API endpoint.
func (k *KolektrNamespace) ExtractAllAPIData(ctx context.Context, apiEndpoint string) (map[string]interface{}, error) {
	return k.t.post(ctx, fmt.Sprintf("kolektr/api-data/%s/extract-all", apiEndpoint), map[string]interface{}{})
}

// ListContent lists content items.
func (k *KolektrNamespace) ListContent(ctx context.Context) (map[string]interface{}, error) {
	return k.t.get(ctx, "kolektr/content", nil)
}

// GetContent gets a content item by ID.
func (k *KolektrNamespace) GetContent(ctx context.Context, itemID string) (map[string]interface{}, error) {
	return k.t.get(ctx, fmt.Sprintf("kolektr/content/%s", itemID), nil)
}

// DeleteContent deletes a content item.
func (k *KolektrNamespace) DeleteContent(ctx context.Context, itemID string) error {
	return k.t.delete(ctx, fmt.Sprintf("kolektr/content/%s", itemID))
}

// ListJobs lists extraction jobs.
func (k *KolektrNamespace) ListJobs(ctx context.Context) (map[string]interface{}, error) {
	return k.t.get(ctx, "kolektr/jobs", nil)
}

// CreateJob creates an extraction job.
func (k *KolektrNamespace) CreateJob(ctx context.Context, config map[string]interface{}) (map[string]interface{}, error) {
	return k.t.post(ctx, "kolektr/jobs", config)
}

// GetJob gets an extraction job by ID.
func (k *KolektrNamespace) GetJob(ctx context.Context, jobID string) (map[string]interface{}, error) {
	return k.t.get(ctx, fmt.Sprintf("kolektr/jobs/%s", jobID), nil)
}

// GetJobStatus gets extraction job status.
func (k *KolektrNamespace) GetJobStatus(ctx context.Context, jobID string) (map[string]interface{}, error) {
	return k.t.get(ctx, fmt.Sprintf("kolektr/extraction-jobs/%s/status", jobID), nil)
}

// GetJobProgress gets job progress.
func (k *KolektrNamespace) GetJobProgress(ctx context.Context, jobID string) (map[string]interface{}, error) {
	return k.t.get(ctx, fmt.Sprintf("kolektr/jobs/%s/progress", jobID), nil)
}

// GetJobProgressEvents gets job progress events.
func (k *KolektrNamespace) GetJobProgressEvents(ctx context.Context, jobID string) (map[string]interface{}, error) {
	return k.t.get(ctx, fmt.Sprintf("kolektr/jobs/%s/progress/events", jobID), nil)
}

// GetJobProgressLatest gets latest progress event for a job.
func (k *KolektrNamespace) GetJobProgressLatest(ctx context.Context, jobID string) (map[string]interface{}, error) {
	return k.t.get(ctx, fmt.Sprintf("kolektr/jobs/%s/progress/latest", jobID), nil)
}

// GetJobProgressSummary gets job progress summary.
func (k *KolektrNamespace) GetJobProgressSummary(ctx context.Context, jobID string) (map[string]interface{}, error) {
	return k.t.get(ctx, fmt.Sprintf("kolektr/jobs/%s/progress/summary", jobID), nil)
}

// GetPipelineEvents gets events for a pipeline run.
func (k *KolektrNamespace) GetPipelineEvents(ctx context.Context, pipelineID string) (map[string]interface{}, error) {
	return k.t.get(ctx, fmt.Sprintf("kolektr/pipeline/%s/events", pipelineID), nil)
}

// GetPipelineStream streams pipeline data.
func (k *KolektrNamespace) GetPipelineStream(ctx context.Context, pipelineID string) (map[string]interface{}, error) {
	return k.t.get(ctx, fmt.Sprintf("kolektr/pipeline/%s/stream", pipelineID), nil)
}

// ListProgressPhases lists all progress phases.
func (k *KolektrNamespace) ListProgressPhases(ctx context.Context) (map[string]interface{}, error) {
	return k.t.get(ctx, "kolektr/progress/phases", nil)
}

// GetProgressPhase gets info for a specific progress phase.
func (k *KolektrNamespace) GetProgressPhase(ctx context.Context, phaseName string) (map[string]interface{}, error) {
	return k.t.get(ctx, fmt.Sprintf("kolektr/progress/phases/%s", phaseName), nil)
}

// GetProgressPhaseSteps gets steps for a progress phase.
func (k *KolektrNamespace) GetProgressPhaseSteps(ctx context.Context, phaseName string) (map[string]interface{}, error) {
	return k.t.get(ctx, fmt.Sprintf("kolektr/progress/phases/%s/steps", phaseName), nil)
}

// GetProgressSummary gets overall progress summary.
func (k *KolektrNamespace) GetProgressSummary(ctx context.Context) (map[string]interface{}, error) {
	return k.t.get(ctx, "kolektr/progress/summary", nil)
}

// ListScrapers lists scraper configurations.
func (k *KolektrNamespace) ListScrapers(ctx context.Context) (map[string]interface{}, error) {
	return k.t.get(ctx, "kolektr/scrapers", nil)
}

// CreateScraper creates a scraper configuration.
func (k *KolektrNamespace) CreateScraper(ctx context.Context, config map[string]interface{}) (map[string]interface{}, error) {
	return k.t.post(ctx, "kolektr/scrapers", config)
}

// GetScraper gets a scraper configuration by ID.
func (k *KolektrNamespace) GetScraper(ctx context.Context, scraperID string) (map[string]interface{}, error) {
	return k.t.get(ctx, fmt.Sprintf("kolektr/scrapers/%s", scraperID), nil)
}

// UpdateScraper updates a scraper configuration.
func (k *KolektrNamespace) UpdateScraper(ctx context.Context, scraperID string, updates map[string]interface{}) (map[string]interface{}, error) {
	return k.t.patch(ctx, fmt.Sprintf("kolektr/scrapers/%s", scraperID), updates)
}

// DeleteScraper deletes a scraper configuration.
func (k *KolektrNamespace) DeleteScraper(ctx context.Context, scraperID string) error {
	return k.t.delete(ctx, fmt.Sprintf("kolektr/scrapers/%s", scraperID))
}

// ─── parsers ─────────────────────────────────────────────────────────────────

func parseExtractionResult(raw map[string]interface{}) *ExtractionResult {
	return &ExtractionResult{
		Success:      boolField(raw, "success"),
		URL:          strField(raw, "url"),
		Method:       strField(raw, "method"),
		Records:      sliceMapField(raw, "records"),
		TotalRecords: intField(raw, "total_records"),
		PagesScraped: intField(raw, "pages_scraped"),
		HasMore:      boolField(raw, "has_more"),
		Total:        intField(raw, "total"),
		ArtifactID:   optStrField(raw, "artifact_id"),
		JobID:        optStrField(raw, "job_id"),
		Error:        optStrField(raw, "error"),
	}
}
