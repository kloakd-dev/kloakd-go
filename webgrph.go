package kloakd

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// WebgrphNamespace exposes the Site Mapping & Discovery module.
// Access via client.Webgrph.
type WebgrphNamespace struct {
	t *transport
}

// Defaults for waiting on a crawl (crawls run asynchronously on the crawlers).
const (
	DefaultCrawlPollInterval = 3 * time.Second
	DefaultCrawlPollTimeout  = 60 * time.Minute
)

// CrawlOptions configures optional parameters for Webgrph.Crawl.
type CrawlOptions struct {
	MaxDepth             int
	MaxPages             int
	IncludeExternalLinks bool
	SessionArtifactID    string
	FastMode             bool
	CaptureMode          string
	// PollInterval and PollTimeout govern waiting for the crawl to finish
	// (defaults 3 s and 60 min; ctx cancellation also stops the wait).
	PollInterval time.Duration
	PollTimeout  time.Duration
}

// StartCrawl starts a site crawl and returns at once (HTTP 202) with its
// crawl ID; follow it with GetCrawlStatus, CrawlEvents or Crawl.
func (w *WebgrphNamespace) StartCrawl(ctx context.Context, targetURL string, opts *CrawlOptions) (*CrawlResult, error) {
	body := map[string]interface{}{"url": targetURL}
	if opts != nil {
		if opts.MaxDepth > 0 {
			body["max_depth"] = opts.MaxDepth
		}
		if opts.MaxPages > 0 {
			body["max_pages"] = opts.MaxPages
		}
		if opts.IncludeExternalLinks {
			body["include_external_links"] = true
		}
		if opts.SessionArtifactID != "" {
			body["session_artifact_id"] = opts.SessionArtifactID
		}
		if opts.FastMode {
			body["fast_mode"] = true
		}
		if opts.CaptureMode != "" {
			body["capture_mode"] = opts.CaptureMode
		}
	}
	raw, err := w.t.post(ctx, "webgrph/crawl", body)
	if err != nil {
		return nil, err
	}
	return parseCrawlResult(raw), nil
}

// Crawl starts a site crawl, waits for it to finish, and returns every
// discovered page (GET webgrph/crawl/{id}/pages, followed to the end).
func (w *WebgrphNamespace) Crawl(ctx context.Context, targetURL string, opts *CrawlOptions) (*CrawlResult, error) {
	started, err := w.StartCrawl(ctx, targetURL, opts)
	if err != nil {
		return nil, err
	}
	if started.CrawlID == "" {
		return started, nil
	}
	interval, limit := DefaultCrawlPollInterval, DefaultCrawlPollTimeout
	if opts != nil {
		if opts.PollInterval > 0 {
			interval = opts.PollInterval
		}
		if opts.PollTimeout > 0 {
			limit = opts.PollTimeout
		}
	}
	deadline := time.Now().Add(limit)
	var status map[string]interface{}
	for {
		status, err = w.GetCrawlStatus(ctx, started.CrawlID)
		if err != nil {
			return nil, err
		}
		state := strField(status, "status")
		if state == "completed" || state == "failed" || state == "cancelled" {
			break
		}
		if time.Now().After(deadline) {
			return nil, &ApiError{KloakdError: KloakdError{
				Message: fmt.Sprintf("crawl %s still %s after %s", started.CrawlID, state, limit), Code: "crawl_timeout",
			}}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}

	result := &CrawlResult{
		Success:         strField(status, "status") == "completed",
		CrawlID:         started.CrawlID,
		URL:             targetURL,
		Status:          strField(status, "status"),
		TotalPages:      intField(status, "total_pages"),
		MaxDepthReached: intField(status, "max_depth_reached"),
		ArtifactID:      optStrField(status, "artifact_id"),
		Error:           optStrField(status, "error"),
		Pages:           []PageNode{},
	}
	if !result.Success {
		return result, nil
	}
	cursor := ""
	for {
		params := map[string]string{"limit": "500"}
		if cursor != "" {
			params["cursor"] = cursor
		}
		page, err := w.t.get(ctx, fmt.Sprintf("webgrph/crawl/%s/pages", started.CrawlID), params)
		if err != nil {
			return nil, err
		}
		for _, n := range sliceMapField(page, "nodes") {
			result.Pages = append(result.Pages, parsePageNode(n))
		}
		result.Total = intField(page, "total")
		cursor = strField(page, "next_cursor")
		if cursor == "" {
			break
		}
	}
	return result, nil
}

// CrawlAll is Crawl, returning only the pages.
func (w *WebgrphNamespace) CrawlAll(ctx context.Context, targetURL string, opts *CrawlOptions) ([]PageNode, error) {
	result, err := w.Crawl(ctx, targetURL, opts)
	if err != nil {
		return nil, err
	}
	return result.Pages, nil
}

// CrawlStreamOptions configures optional parameters for Webgrph.CrawlStream.
type CrawlStreamOptions struct {
	MaxDepth          int
	MaxPages          int
	SessionArtifactID string
}

// CrawlStream starts a crawl and streams its events
// (GET webgrph/crawl/{crawl_id}/events). The channel closes when the crawl
// ends or ctx is cancelled.
func (w *WebgrphNamespace) CrawlStream(ctx context.Context, targetURL string, opts *CrawlStreamOptions) (<-chan CrawlEvent, error) {
	crawlOpts := &CrawlOptions{}
	if opts != nil {
		crawlOpts.MaxDepth = opts.MaxDepth
		crawlOpts.MaxPages = opts.MaxPages
		crawlOpts.SessionArtifactID = opts.SessionArtifactID
	}
	started, err := w.StartCrawl(ctx, targetURL, crawlOpts)
	if err != nil {
		return nil, err
	}
	raw, err := w.CrawlEvents(ctx, started.CrawlID)
	if err != nil {
		return nil, err
	}

	ch := make(chan CrawlEvent, 16)
	go func() {
		defer close(ch)
		for payload := range raw {
			ch <- parseCrawlEvent(payload)
		}
	}()
	return ch, nil
}

// GetCrawlStatus polls crawl status and stats.
func (w *WebgrphNamespace) GetCrawlStatus(ctx context.Context, crawlID string) (map[string]interface{}, error) {
	return w.t.get(ctx, fmt.Sprintf("webgrph/crawl/%s", crawlID), nil)
}

// CrawlEvents streams a crawl's events (SSE).
func (w *WebgrphNamespace) CrawlEvents(ctx context.Context, crawlID string) (<-chan map[string]interface{}, error) {
	return w.t.getStream(ctx, fmt.Sprintf("webgrph/crawl/%s/events", crawlID), nil)
}

// GetCrawlPages gets one page of discovered pages (cursor "" for the first).
func (w *WebgrphNamespace) GetCrawlPages(ctx context.Context, crawlID, cursor string, limit int) (map[string]interface{}, error) {
	params := map[string]string{}
	if cursor != "" {
		params["cursor"] = cursor
	}
	if limit > 0 {
		params["limit"] = fmt.Sprintf("%d", limit)
	}
	return w.t.get(ctx, fmt.Sprintf("webgrph/crawl/%s/pages", crawlID), params)
}

// CancelCrawl cancels a running crawl.
func (w *WebgrphNamespace) CancelCrawl(ctx context.Context, crawlID string) (map[string]interface{}, error) {
	return w.t.post(ctx, fmt.Sprintf("webgrph/crawl/%s/cancel", crawlID), nil)
}

// DetectedAPIs lists the APIs detected on a page.
func (w *WebgrphNamespace) DetectedAPIs(ctx context.Context, pageURL string) (map[string]interface{}, error) {
	return w.t.get(ctx, "webgrph/detected-apis", map[string]string{"page_url": pageURL})
}

// ReaderView returns a page's readable content (from a crawl's cache when
// crawlID is given).
func (w *WebgrphNamespace) ReaderView(ctx context.Context, targetURL, crawlID string) (map[string]interface{}, error) {
	body := map[string]interface{}{"url": targetURL}
	if crawlID != "" {
		body["crawl_id"] = crawlID
	}
	return w.t.post(ctx, "webgrph/reader-view", body)
}

// ListSessions lists discovery sessions (items under "items").
func (w *WebgrphNamespace) ListSessions(ctx context.Context, activeOnly bool) (map[string]interface{}, error) {
	params := map[string]string{}
	if activeOnly {
		params["active_only"] = "true"
	}
	return w.t.get(ctx, "webgrph/sessions", params)
}

// SaveSession creates or updates a discovery session ("url" is required).
func (w *WebgrphNamespace) SaveSession(ctx context.Context, session map[string]interface{}) (map[string]interface{}, error) {
	return w.t.post(ctx, "webgrph/sessions", session)
}

// GetSession gets a discovery session.
func (w *WebgrphNamespace) GetSession(ctx context.Context, sessionID string) (map[string]interface{}, error) {
	return w.t.get(ctx, fmt.Sprintf("webgrph/sessions/%s", sessionID), nil)
}

// DeleteSession deletes a discovery session.
func (w *WebgrphNamespace) DeleteSession(ctx context.Context, sessionID string) error {
	return w.t.delete(ctx, fmt.Sprintf("webgrph/sessions/%s", sessionID))
}

// EndSession ends a discovery session.
func (w *WebgrphNamespace) EndSession(ctx context.Context, sessionID string) (map[string]interface{}, error) {
	return w.t.post(ctx, fmt.Sprintf("webgrph/sessions/%s/end", sessionID), nil)
}

// UpdateSessionJob links a job to a discovery session.
func (w *WebgrphNamespace) UpdateSessionJob(ctx context.Context, sessionID, jobID string) (map[string]interface{}, error) {
	return w.t.request(ctx, http.MethodPatch, fmt.Sprintf("webgrph/sessions/%s/job", sessionID), nil, map[string]string{"job_id": jobID})
}

// GetDashboardSummary gets analytics dashboard summary.
func (w *WebgrphNamespace) GetDashboardSummary(ctx context.Context) (map[string]interface{}, error) {
	return w.t.get(ctx, "webgrph/analytics/dashboard/summary", nil)
}

// GetErrorSummary gets error summary analytics.
func (w *WebgrphNamespace) GetErrorSummary(ctx context.Context) (map[string]interface{}, error) {
	return w.t.get(ctx, "webgrph/analytics/error-summary", nil)
}

// GetJobTrends gets job trends analytics.
func (w *WebgrphNamespace) GetJobTrends(ctx context.Context) (map[string]interface{}, error) {
	return w.t.get(ctx, "webgrph/analytics/job-trends", nil)
}

// GetDiscoveryPatterns gets content discovery pattern analytics.
func (w *WebgrphNamespace) GetDiscoveryPatterns(ctx context.Context) (map[string]interface{}, error) {
	return w.t.get(ctx, "webgrph/analytics/content/discovery-patterns", nil)
}

// GetEfficiencyMetrics gets scraping efficiency metrics.
func (w *WebgrphNamespace) GetEfficiencyMetrics(ctx context.Context) (map[string]interface{}, error) {
	return w.t.get(ctx, "webgrph/analytics/scraping/efficiency-metrics", nil)
}

// GetSiteMappingTrends gets site mapping trends.
func (w *WebgrphNamespace) GetSiteMappingTrends(ctx context.Context) (map[string]interface{}, error) {
	return w.t.get(ctx, "webgrph/analytics/site-maps/trends", nil)
}

// GetUserBehaviorInsights gets user behavior insights.
func (w *WebgrphNamespace) GetUserBehaviorInsights(ctx context.Context) (map[string]interface{}, error) {
	return w.t.get(ctx, "webgrph/analytics/users/behavior-insights", nil)
}

// ─── parsers ─────────────────────────────────────────────────────────────────

func parseCrawlResult(raw map[string]interface{}) *CrawlResult {
	pages := []PageNode{}
	if v, ok := raw["pages"]; ok {
		if arr, ok := v.([]interface{}); ok {
			for _, item := range arr {
				if m, ok := item.(map[string]interface{}); ok {
					pages = append(pages, parsePageNode(m))
				}
			}
		}
	}
	return &CrawlResult{
		Success:         boolField(raw, "success"),
		CrawlID:         strField(raw, "crawl_id"),
		URL:             strField(raw, "url"),
		TotalPages:      intField(raw, "total_pages"),
		MaxDepthReached: intField(raw, "max_depth_reached"),
		Pages:           pages,
		ArtifactID:      nestedArtifactID(raw),
		Error:           optStrField(raw, "error"),
	}
}

func parsePageNode(raw map[string]interface{}) PageNode {
	meta := mapField(raw, "metadata")
	return PageNode{
		ID:        strField(raw, "id"),
		URL:       strField(raw, "url"),
		ParentURL: optStrField(raw, "parent_url"),
		Depth:     intField(raw, "depth"),
		Category:  strField(raw, "category"),
		Title:     optStrField(meta, "title"),
		Children:  strSliceField(raw, "children_urls"),
		Metadata:  meta,
	}
}

func parseCrawlEvent(raw map[string]interface{}) CrawlEvent {
	return CrawlEvent{
		Type:       strField(raw, "type"),
		URL:        optStrField(raw, "url"),
		Depth:      optIntField(raw, "depth"),
		PagesFound: optIntField(raw, "pages_found"),
		Metadata:   mapField(raw, "metadata"),
	}
}
