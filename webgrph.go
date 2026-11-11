package kloakd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
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
	// CaptureMode is "structure", "content" or "visual". Scope is a
	// CrawlScope (same_host, same_domain, allowed_domains,
	// allowed_subdomains, excluded_paths, include_patterns,
	// exclude_patterns); Budget a CrawlBudget (max_resources, max_depth,
	// max_file_size, max_total_bytes, ...). All optional.
	CaptureMode string
	Scope       map[string]interface{}
	Budget      map[string]interface{}
	// PollInterval and PollTimeout govern waiting for the crawl to finish
	// (defaults 3 s and 60 min; ctx cancellation also stops the wait).
	PollInterval time.Duration
	PollTimeout  time.Duration
	// IncludeGrouping makes Crawl also list the tree's grouping folders
	// (PageNode.IsGrouping), which were not fetched. By default only
	// fetched pages are listed, so len(Pages) matches TotalPages (0.6.0).
	IncludeGrouping bool
}

// StartCrawl starts a site crawl and returns at once (HTTP 202) with its
// crawl ID; follow it with WaitForCrawl, GetCrawlStatus or CrawlEvents. The
// answer has no status: Status and the outcome fields are empty here.
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
		if len(opts.Scope) > 0 {
			body["scope"] = opts.Scope
		}
		if len(opts.Budget) > 0 {
			body["budget"] = opts.Budget
		}
	}
	raw, err := w.t.post(ctx, "webgrph/crawl", body)
	if err != nil {
		return nil, err
	}
	return parseCrawlResult(raw), nil
}

// Crawl starts a site crawl, waits for it to end, and returns every
// discovered page (GET webgrph/crawl/{id}/pages, followed to the end).
//
// The wait also ends, without an error, when the crawl is held: Status is
// "held_quota", Held is true, Success is false, Error is nil, Message is the
// API's text and no pages are loaded. See WaitForCrawl.
func (w *WebgrphNamespace) Crawl(ctx context.Context, targetURL string, opts *CrawlOptions) (*CrawlResult, error) {
	started, err := w.StartCrawl(ctx, targetURL, opts)
	if err != nil {
		return nil, err
	}
	if started.CrawlID == "" {
		return started, nil
	}
	o := CrawlOptions{}
	if opts != nil {
		o = *opts
	}
	result, err := w.waitForCrawl(ctx, started.CrawlID, &PollOptions{Interval: o.PollInterval, Timeout: o.PollTimeout}, o.IncludeGrouping)
	if err != nil {
		return nil, err
	}
	result.URL = targetURL
	return result, nil
}

// WaitForCrawl waits for a crawl that was already started (StartCrawl, or
// an earlier Crawl or WaitForCrawl) and returns its result with every page,
// as Crawl does. poll may be nil (3 s, 60 min); a crawl still running at
// the timeout is an *ApiError with Code "crawl_timeout" and the crawl id in
// JobID. URL is empty: the status route does not send the crawl's URL.
//
// A held crawl is not an error and not a failure. When the organization's
// quota runs out the API pauses the crawl: its status is "held_quota". The
// wait then stops and returns Status "held_quota", Held true, Success
// false, Error nil, Message with the API's text, and no pages. The crawl
// resumes by itself, on the same crawl id, when quota returns (an upgrade,
// a top-up or the next period); call WaitForCrawl with that id again later.
// A crawl held for 30 days ends as "failed" with ErrorCode
// "held_quota_expired".
//
// A crawl that stopped early has IsPartial true and a PartialReason; when
// Resumable is true, ContinueCrawl fetches the rest.
func (w *WebgrphNamespace) WaitForCrawl(ctx context.Context, crawlID string, poll *PollOptions) (*CrawlResult, error) {
	return w.waitForCrawl(ctx, crawlID, poll, false)
}

func (w *WebgrphNamespace) waitForCrawl(ctx context.Context, crawlID string, poll *PollOptions, includeGrouping bool) (*CrawlResult, error) {
	opts := PollOptions{Interval: DefaultCrawlPollInterval, Timeout: DefaultCrawlPollTimeout}
	if poll != nil {
		opts = poll.or(opts)
	}
	deadline := time.Now().Add(opts.Timeout)
	var status map[string]interface{}
	for {
		var err error
		status, err = w.GetCrawlStatus(ctx, crawlID)
		if err != nil {
			return nil, err
		}
		state := strField(status, "status")
		if state == "completed" || state == "failed" || state == "cancelled" || state == crawlHeldQuota {
			break
		}
		if time.Now().After(deadline) {
			return nil, &ApiError{KloakdError: KloakdError{
				Message: fmt.Sprintf("crawl %s still %s after %s", crawlID, state, opts.Timeout), Code: "crawl_timeout",
			}, JobID: crawlID}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(opts.Interval):
		}
	}

	result := crawlResultFromStatus(crawlID, status)
	if !result.Success {
		return result, nil
	}
	cursor := ""
	for {
		params := map[string]string{"limit": "500"}
		if cursor != "" {
			params["cursor"] = cursor
		}
		if includeGrouping {
			params["include_grouping"] = "true"
		}
		page, err := w.t.get(ctx, pathf("webgrph/crawl/%s/pages", crawlID), params)
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

// crawlHeldQuota is the status of a crawl (or of the crawl of a
// crawl-extract job) the API paused because the organization's quota ran
// out (billing/held_work.py). It is a pause, not an end.
const crawlHeldQuota = "held_quota"

// crawlResultFromStatus reads a crawl status body (GET webgrph/crawl/{id}).
// The body has no url.
func crawlResultFromStatus(crawlID string, status map[string]interface{}) *CrawlResult {
	state := strField(status, "status")
	return &CrawlResult{
		Success:         state == "completed",
		CrawlID:         crawlID,
		Status:          state,
		TotalPages:      intField(status, "total_pages"),
		MaxDepthReached: intField(status, "max_depth_reached"),
		ArtifactID:      optStrField(status, "artifact_id"),
		Error:           optStrField(status, "error"),
		Pages:           []PageNode{},
		Held:            state == crawlHeldQuota,
		Message:         optStrField(status, "message"),
		IsPartial:       optBoolField(status, "is_partial"),
		PartialReason:   optStrField(status, "partial_reason"),
		Resumable:       boolField(status, "resumable"),
		PagesDiscovered: optIntField(status, "pages_discovered"),
		PagesFetched:    optIntField(status, "pages_fetched"),
		PagesUnfetched:  optIntField(status, "pages_unfetched"),
		ErrorCode:       optStrField(status, "error_code"),
	}
}

// CrawlContinueResult is returned by Webgrph.ContinueCrawl.
type CrawlContinueResult struct {
	Success bool
	CrawlID string
	// Status is "queued" once the API has accepted the continuation.
	Status string
}

// ContinueCrawl asks the API to fetch the rest of a crawl that stopped at
// its time budget (POST webgrph/crawl/{id}/continue, answered 202). It is
// only for a crawl that has ended and whose result has Resumable true; the
// crawl keeps its id, so wait for it again with WaitForCrawl. For any other
// crawl (not ended, not stopped at its time budget, or already being
// continued) the API answers 409, returned as an *ApiError.
func (w *WebgrphNamespace) ContinueCrawl(ctx context.Context, crawlID string) (*CrawlContinueResult, error) {
	raw, err := w.t.post(ctx, pathf("webgrph/crawl/%s/continue", crawlID), nil)
	if err != nil {
		return nil, err
	}
	return &CrawlContinueResult{
		Success: boolField(raw, "success"),
		CrawlID: strField(raw, "crawl_id"),
		Status:  strField(raw, "status"),
	}, nil
}

// CrawlAll is Crawl, returning only the pages. It returns no pages and no
// error for a crawl that failed, was cancelled or is held for quota
// ("held_quota"); use Crawl or WaitForCrawl to tell these apart (Status,
// Held, Error).
func (w *WebgrphNamespace) CrawlAll(ctx context.Context, targetURL string, opts *CrawlOptions) ([]PageNode, error) {
	result, err := w.Crawl(ctx, targetURL, opts)
	if err != nil {
		return nil, err
	}
	return result.Pages, nil
}

// CrawlStreamOptions configures optional parameters for Webgrph.CrawlStream.
type CrawlStreamOptions struct {
	MaxDepth             int
	MaxPages             int
	IncludeExternalLinks bool
	SessionArtifactID    string
	CaptureMode          string
	Scope                map[string]interface{}
	Budget               map[string]interface{}
}

// CrawlStream starts a crawl and streams its events
// (GET webgrph/crawl/{crawl_id}/events). The channel closes when the crawl
// ends, when it is held for quota (status "held_quota"; see WaitForCrawl)
// or when ctx is cancelled; if reading the stream failed, the last event
// carries Err.
func (w *WebgrphNamespace) CrawlStream(ctx context.Context, targetURL string, opts *CrawlStreamOptions) (<-chan CrawlEvent, error) {
	crawlOpts := &CrawlOptions{}
	if opts != nil {
		crawlOpts.MaxDepth = opts.MaxDepth
		crawlOpts.MaxPages = opts.MaxPages
		crawlOpts.IncludeExternalLinks = opts.IncludeExternalLinks
		crawlOpts.SessionArtifactID = opts.SessionArtifactID
		crawlOpts.CaptureMode = opts.CaptureMode
		crawlOpts.Scope = opts.Scope
		crawlOpts.Budget = opts.Budget
	}
	started, err := w.StartCrawl(ctx, targetURL, crawlOpts)
	if err != nil {
		return nil, err
	}
	return w.CrawlEvents(ctx, started.CrawlID)
}

// GetCrawlStatus gets a crawl's status body as the API sends it: status
// (running, completed, failed, cancelled or held_quota), total_pages,
// message, and the outcome fields CrawlResult names (is_partial,
// partial_reason, resumable, pages_discovered, pages_fetched,
// pages_unfetched, error_code).
func (w *WebgrphNamespace) GetCrawlStatus(ctx context.Context, crawlID string) (map[string]interface{}, error) {
	return w.t.get(ctx, pathf("webgrph/crawl/%s", crawlID), nil)
}

// CrawlEvents streams a crawl's pipeline events (SSE). The channel closes
// when the stream ends or ctx is cancelled; if reading the stream failed,
// the last event carries Err.
func (w *WebgrphNamespace) CrawlEvents(ctx context.Context, crawlID string) (<-chan CrawlEvent, error) {
	raw, err := w.t.getStream(ctx, pathf("webgrph/crawl/%s/events", crawlID), nil)
	if err != nil {
		return nil, err
	}
	return mapStream(ctx, raw, parsePipelineEvent), nil
}

// Bounds of the pages listing's limit (GET webgrph/crawl/{id}/pages).
const (
	CrawlPagesDefaultLimit = 100
	CrawlPagesMaxLimit     = 1000
)

// ErrInvalidPagesLimit is wrapped by the error of a pages listing whose
// limit is outside 1–CrawlPagesMaxLimit; no request is sent (the API would
// answer 422).
var ErrInvalidPagesLimit = errors.New("invalid crawl pages limit")

// CrawlPagesOptions configures Webgrph.ListCrawlPages.
type CrawlPagesOptions struct {
	// Cursor is the previous page's next_cursor ("" for the first page).
	// The API answers 422 for a cursor that is not a non-negative integer.
	Cursor string
	// Limit is the nodes per page, 1 to CrawlPagesMaxLimit; 0 sends none
	// (the API's default, CrawlPagesDefaultLimit).
	Limit int
	// IncludeGrouping also lists the tree's grouping folders
	// (metadata.is_grouping), which were not fetched. By default only
	// fetched pages are listed, each under the page the crawl found it on,
	// and total matches the crawl status's total_pages.
	IncludeGrouping bool
}

// CrawlPagesResult is one page of a crawl's pages listing.
type CrawlPagesResult struct {
	CrawlID string
	Nodes   []PageNode
	// NextCursor is the cursor of the next page; "" on the last.
	NextCursor string
	// Total is the number of nodes the listing has across all pages.
	Total int
	// Raw is the answer as received.
	Raw map[string]interface{}
}

// ListCrawlPages gets one page of a crawl's pages
// (GET webgrph/crawl/{id}/pages) as PageNode values (0.6.0). A Limit
// outside 1–CrawlPagesMaxLimit is refused before any request is sent.
func (w *WebgrphNamespace) ListCrawlPages(ctx context.Context, crawlID string, opts *CrawlPagesOptions) (*CrawlPagesResult, error) {
	var o CrawlPagesOptions
	if opts != nil {
		o = *opts
	}
	raw, err := w.getCrawlPages(ctx, crawlID, o)
	if err != nil {
		return nil, err
	}
	res := &CrawlPagesResult{
		CrawlID:    strField(raw, "crawl_id"),
		Nodes:      []PageNode{},
		NextCursor: strField(raw, "next_cursor"),
		Total:      intField(raw, "total"),
		Raw:        raw,
	}
	for _, n := range sliceMapField(raw, "nodes") {
		res.Nodes = append(res.Nodes, parsePageNode(n))
	}
	return res, nil
}

// GetCrawlPages gets one page of discovered pages (cursor "" for the first;
// limit 0 for the API's default of 100, else 1–1000). Only fetched pages
// are listed; ListCrawlPages has IncludeGrouping and typed nodes.
func (w *WebgrphNamespace) GetCrawlPages(ctx context.Context, crawlID, cursor string, limit int) (map[string]interface{}, error) {
	return w.getCrawlPages(ctx, crawlID, CrawlPagesOptions{Cursor: cursor, Limit: limit})
}

func (w *WebgrphNamespace) getCrawlPages(ctx context.Context, crawlID string, o CrawlPagesOptions) (map[string]interface{}, error) {
	if o.Limit < 0 || o.Limit > CrawlPagesMaxLimit {
		return nil, fmt.Errorf("kloakd: %w: limit must be 1-%d (0 for the default of %d), got %d",
			ErrInvalidPagesLimit, CrawlPagesMaxLimit, CrawlPagesDefaultLimit, o.Limit)
	}
	params := map[string]string{}
	if o.Cursor != "" {
		params["cursor"] = o.Cursor
	}
	if o.Limit > 0 {
		params["limit"] = strconv.Itoa(o.Limit)
	}
	if o.IncludeGrouping {
		params["include_grouping"] = "true"
	}
	return w.t.get(ctx, pathf("webgrph/crawl/%s/pages", crawlID), params)
}

// CancelCrawl cancels a running crawl.
func (w *WebgrphNamespace) CancelCrawl(ctx context.Context, crawlID string) (map[string]interface{}, error) {
	return w.t.post(ctx, pathf("webgrph/crawl/%s/cancel", crawlID), nil)
}

// DetectedAPIs lists the APIs detected on a page.
func (w *WebgrphNamespace) DetectedAPIs(ctx context.Context, pageURL string) (map[string]interface{}, error) {
	return w.t.get(ctx, "webgrph/detected-apis", map[string]string{"page_url": pageURL})
}

// ReaderViewOptions configures optional parameters for Webgrph.ReaderView.
type ReaderViewOptions struct {
	// CrawlID reads the page from that crawl's cache when it has it.
	CrawlID string
	// SessionArtifactID fetches with an authenticated Fetchyr session.
	SessionArtifactID string
}

// ReaderView returns a page's readable content.
func (w *WebgrphNamespace) ReaderView(ctx context.Context, targetURL string, opts *ReaderViewOptions) (map[string]interface{}, error) {
	body := map[string]interface{}{"url": targetURL}
	if opts != nil {
		if opts.CrawlID != "" {
			body["crawl_id"] = opts.CrawlID
		}
		if opts.SessionArtifactID != "" {
			body["session_artifact_id"] = opts.SessionArtifactID
		}
	}
	return w.t.post(ctx, "webgrph/reader-view", body)
}

// ListSessions lists discovery sessions (items under "items"): only the
// active ones when activeOnly is true, all of them when false. (The API's
// default is active only; active_only is always sent.)
func (w *WebgrphNamespace) ListSessions(ctx context.Context, activeOnly bool) (map[string]interface{}, error) {
	return w.t.get(ctx, "webgrph/sessions", map[string]string{"active_only": strconv.FormatBool(activeOnly)})
}

// SaveSession creates or updates a discovery session ("url" is required).
func (w *WebgrphNamespace) SaveSession(ctx context.Context, session map[string]interface{}) (map[string]interface{}, error) {
	return w.t.post(ctx, "webgrph/sessions", session)
}

// GetSession gets a discovery session.
func (w *WebgrphNamespace) GetSession(ctx context.Context, sessionID string) (map[string]interface{}, error) {
	return w.t.get(ctx, pathf("webgrph/sessions/%s", sessionID), nil)
}

// DeleteSession deletes a discovery session.
func (w *WebgrphNamespace) DeleteSession(ctx context.Context, sessionID string) error {
	return w.t.delete(ctx, pathf("webgrph/sessions/%s", sessionID))
}

// EndSession ends a discovery session.
func (w *WebgrphNamespace) EndSession(ctx context.Context, sessionID string) (map[string]interface{}, error) {
	return w.t.post(ctx, pathf("webgrph/sessions/%s/end", sessionID), nil)
}

// UpdateSessionJob links a job to a discovery session.
func (w *WebgrphNamespace) UpdateSessionJob(ctx context.Context, sessionID, jobID string) (map[string]interface{}, error) {
	return w.t.request(ctx, http.MethodPatch, pathf("webgrph/sessions/%s/job", sessionID), nil, map[string]string{"job_id": jobID})
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

// parsePageNode reads a pages-listing node (HierarchyNode: metadata.title,
// metadata.is_grouping, metadata.fetch_status, metadata.fetch_error) or a
// stored page record (flat title, is_grouping, crawl_parent_url).
func parsePageNode(raw map[string]interface{}) PageNode {
	meta := mapField(raw, "metadata")
	title := optStrField(meta, "title")
	if title == nil {
		title = optStrField(raw, "title")
	}
	crawlParent := optStrField(raw, "crawl_parent_url")
	if crawlParent == nil {
		crawlParent = optStrField(meta, "crawl_parent_url")
	}
	return PageNode{
		ID:             strField(raw, "id"),
		URL:            strField(raw, "url"),
		ParentURL:      optStrField(raw, "parent_url"),
		Depth:          intField(raw, "depth"),
		Category:       strField(raw, "category"),
		Title:          title,
		Children:       strSliceField(raw, "children_urls"),
		Metadata:       meta,
		IsGrouping:     boolField(meta, "is_grouping") || boolField(raw, "is_grouping"),
		CrawlParentURL: crawlParent,
		FetchStatus:    optStrField(meta, "fetch_status"),
		FetchError:     optStrField(meta, "fetch_error"),
	}
}
