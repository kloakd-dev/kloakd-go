package kloakd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Crawl, extract and download in one job
// (kobe-scraper modules/webgrph/crawl_extract_router.py), and the batch
// job export (kernel/batch_export_router.py).

// Bounds of the crawl-extract results and pages listings' limit.
const (
	CrawlExtractDefaultLimit = 100
	CrawlExtractMaxLimit     = 1000
)

// Defaults for waiting on a crawl-extract job.
const (
	DefaultCrawlExtractPollInterval = 3 * time.Second
	DefaultCrawlExtractPollTimeout  = 60 * time.Minute
)

// ErrInvalidCrawlExtractLimit is wrapped by the error of a results or pages
// listing whose limit is outside 1–CrawlExtractMaxLimit; no request is sent
// (the API would answer 422).
var ErrInvalidCrawlExtractLimit = errors.New("invalid crawl-extract limit")

// ErrInvalidExportFormat is wrapped by the error of an export whose format
// is not "json", "ndjson" or "csv"; no request is sent.
var ErrInvalidExportFormat = errors.New("invalid export format")

// Export formats.
const (
	ExportJSON   = "json"
	ExportNDJSON = "ndjson"
	ExportCSV    = "csv"
)

// CrawlExtractSelectorField is one field of a selector schema.
type CrawlExtractSelectorField struct {
	Name      string
	Selector  string
	Attribute string
	Type      string
}

// CrawlExtractSchema is a selector schema applied to every page: one record
// per ItemSelector match ("" for one record per page), each field read
// inside it. Fields needs at least one entry.
type CrawlExtractSchema struct {
	ItemSelector string
	Fields       []CrawlExtractSelectorField
}

// CrawlExtractSpec says what to extract from each crawled page. All parts
// are optional; with none the API extracts what it finds.
type CrawlExtractSpec struct {
	EntityType string
	Fields     []string
	Schema     *CrawlExtractSchema
}

// CrawlExtractOptions configures Webgrph.StartCrawlExtract and CrawlExtract.
type CrawlExtractOptions struct {
	// MaxDepth (1–100) and MaxPages (1–100000); 0 sends none and the API
	// applies the plan's limits.
	MaxDepth             int
	MaxPages             int
	IncludeExternalLinks bool
	SessionArtifactID    string
	CaptureMode          string
	Scope                map[string]interface{}
	Budget               map[string]interface{}
	Extract              *CrawlExtractSpec
	// PollInterval and PollTimeout govern CrawlExtract's wait (defaults 3 s
	// and 60 min; ctx cancellation also stops it).
	PollInterval time.Duration
	PollTimeout  time.Duration
}

// CrawlExtractLinks are the job's routes, as paths from the API's root.
type CrawlExtractLinks struct {
	Status      string
	Results     string
	Pages       string
	Cancel      string
	Export      map[string]string // by format: json, ndjson, csv
	CrawlEvents string
	Events      string
}

// CrawlExtractJob is the answer to starting a crawl-extract job (202).
type CrawlExtractJob struct {
	Success  bool
	JobID    string
	CrawlID  string
	URL      string
	Status   string
	MaxPages int
	MaxDepth int
	Links    CrawlExtractLinks
	Raw      map[string]interface{}
}

// CrawlExtractProgress are a job's counters.
type CrawlExtractProgress struct {
	PagesCrawled   int
	PagesTotal     int
	PagesExtracted int
	PagesFailed    int
	Items          int
}

// CrawlExtractShape is one set of rows with the same fields.
type CrawlExtractShape struct {
	Shape     string
	Kind      string // "item" or "page"
	Rows      int
	PageTypes []string
	Fields    []string
}

// CrawlExtractStatus is a crawl-extract job's state. Status is "queued",
// "crawling", "extracting", "completed", "failed" or "cancelled".
type CrawlExtractStatus struct {
	JobID           string
	CrawlID         string
	URL             string
	Status          string
	CancelRequested bool
	Progress        CrawlExtractProgress
	Error           *string
	ErrorCode       *string
	Request         map[string]interface{}
	CreatedAt       *string
	StartedAt       *string
	CompletedAt     *string
	UpdatedAt       *string
	Links           CrawlExtractLinks
	PagesByMethod   map[string]int
	Shapes          []CrawlExtractShape
	// Crawl is what the job's crawl really did (the status body's "crawl"
	// object); nil when the API sent none. A completed job whose crawl is
	// partial holds the rows of the pages that crawl fetched, not of the
	// whole site.
	Crawl *CrawlExtractCrawl
	// Held is true when the job's crawl is paused because the
	// organization's quota ran out (Crawl.Status "held_quota"). The job
	// itself keeps its status and waits; see WaitForCrawlExtract.
	Held bool
	Raw  map[string]interface{}
}

// CrawlExtractCrawl is the crawl of a crawl-extract job, as the job's
// status reports it. The counts and IsPartial are nil until the crawl has
// reported them.
type CrawlExtractCrawl struct {
	// Status is the crawl's own status (running, completed, failed,
	// cancelled or held_quota), not the job's.
	Status string
	// IsPartial is true when the crawl stopped before asking for every
	// page it found; PartialReason is then time_budget, max_pages or error.
	IsPartial       *bool
	PartialReason   *string
	PagesDiscovered *int
	PagesFetched    *int
	PagesUnfetched  *int
	// Cancelled is true for a cancelled crawl; RootUnfetched when the
	// crawl could not fetch its first page.
	Cancelled     bool
	RootUnfetched bool
	// Error is the crawl's error when it failed; ErrorCode its code.
	Error     *string
	ErrorCode *string
}

// Finished reports whether the job has ended (completed, failed or cancelled).
func (s *CrawlExtractStatus) Finished() bool {
	return s.Status == "completed" || s.Status == "failed" || s.Status == "cancelled"
}

// CrawlExtractRow is one extracted row. RowKind is "item" (one record of a
// page) or "page" (the page's own record).
type CrawlExtractRow struct {
	PageURL   string
	PageType  *string
	RowKind   string
	ItemIndex int
	Data      map[string]interface{}
	Shape     string
}

// CrawlExtractResultsOptions configures Webgrph.ListCrawlExtractResults.
type CrawlExtractResultsOptions struct {
	// Limit is the rows per page, 1 to CrawlExtractMaxLimit; 0 sends none
	// (the API's default, CrawlExtractDefaultLimit).
	Limit int
	// Cursor is the previous page's NextCursor ("" for the first page).
	Cursor string
	// Shape, PageType and Kind ("item" or "page") filter the rows.
	Shape    string
	PageType string
	Kind     string
}

// CrawlExtractResultsPage is one page of a job's rows.
type CrawlExtractResultsPage struct {
	JobID  string
	Status string
	// Total is the number of rows the filters match across all pages.
	Total int
	Count int
	Rows  []CrawlExtractRow
	// NextCursor is the cursor of the next page; "" on the last.
	NextCursor string
	Raw        map[string]interface{}
}

// CrawlExtractPage is one crawled page of a job and what its extraction did.
type CrawlExtractPage struct {
	PageSeq     int
	PageURL     string
	PageType    *string
	Title       *string
	Status      string // "extracted" or "failed"
	Method      *string
	ItemCount   int
	Detail      map[string]interface{}
	ExtractedAt *string
}

// CrawlExtractPagesOptions configures Webgrph.ListCrawlExtractPages.
type CrawlExtractPagesOptions struct {
	// Limit is the pages per call, 1 to CrawlExtractMaxLimit; 0 sends none
	// (the API's default, CrawlExtractDefaultLimit).
	Limit  int
	Offset int
	// Status filters: "extracted" or "failed".
	Status string
}

// CrawlExtractPagesResult is one call's worth of a job's pages.
type CrawlExtractPagesResult struct {
	JobID  string
	Status string
	Count  int
	Offset int
	Pages  []CrawlExtractPage
	Raw    map[string]interface{}
}

// CrawlExtractExportOptions configures the crawl-extract export.
type CrawlExtractExportOptions struct {
	// Format is ExportJSON (the default when ""), ExportNDJSON or ExportCSV.
	Format string
	// Shape, PageType and Kind ("item" or "page") filter the rows.
	Shape    string
	PageType string
	Kind     string
}

// ExportDownload is an open export. Body is the response body as it
// arrives; the caller must Close it.
type ExportDownload struct {
	Body        io.ReadCloser
	ContentType string
	// Filename is the Content-Disposition file name.
	Filename string
	// JobStatus is the crawl-extract job's status when the export started
	// (X-Job-Status): the export of a running or failed job holds the rows
	// written so far. It is set for crawl-extract exports only; it is ""
	// for a batch export, whose route sends no such header.
	JobStatus string
	// Crawl is what the job's crawl did, read from the X-Crawl-* headers
	// of a crawl-extract export. nil when the answer has no X-Crawl-*
	// header, as for a batch export.
	Crawl *ExportCrawl
}

// ExportCrawl is the crawl of an exported crawl-extract job, read from the
// export's X-Crawl-* headers. The API leaves out a header whose value the
// crawl has not reported; that field is then nil. An export of a job whose
// crawl is partial (IsPartial true) holds the rows of the pages the crawl
// fetched, not of the whole site.
type ExportCrawl struct {
	Status          *string // X-Crawl-Status
	IsPartial       *bool   // X-Crawl-Partial
	PartialReason   *string // X-Crawl-Partial-Reason: time_budget, max_pages or error
	PagesDiscovered *int    // X-Crawl-Pages-Discovered
	PagesFetched    *int    // X-Crawl-Pages-Fetched
	PagesUnfetched  *int    // X-Crawl-Pages-Unfetched
	Cancelled       *bool   // X-Crawl-Cancelled
	RootUnfetched   *bool   // X-Crawl-Root-Unfetched: the first page could not be fetched
}

// SavedExport describes an export written to a file. JobStatus and Crawl
// are those of the ExportDownload it was written from.
type SavedExport struct {
	Path        string
	Bytes       int64
	ContentType string
	Filename    string
	JobStatus   string
	Crawl       *ExportCrawl
}

// CrawlExtractCancelResult is the answer to cancelling a job. Status is
// "cancelled" or "cancelling" (the crawl is still stopping).
type CrawlExtractCancelResult struct {
	Success bool
	JobID   string
	CrawlID string
	Status  string
}

// StartCrawlExtract starts a job that crawls a site and extracts every
// page, and returns at once (HTTP 202) with its job ID. The plan needs
// Kolektr; a test key is refused (403).
func (w *WebgrphNamespace) StartCrawlExtract(ctx context.Context, targetURL string, opts *CrawlExtractOptions) (*CrawlExtractJob, error) {
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
		if opts.CaptureMode != "" {
			body["capture_mode"] = opts.CaptureMode
		}
		if len(opts.Scope) > 0 {
			body["scope"] = opts.Scope
		}
		if len(opts.Budget) > 0 {
			body["budget"] = opts.Budget
		}
		if opts.Extract != nil {
			body["extract"] = crawlExtractSpecBody(opts.Extract)
		}
	}
	raw, err := w.t.post(ctx, "webgrph/crawl-extract", body)
	if err != nil {
		return nil, err
	}
	limits := mapField(raw, "limits")
	return &CrawlExtractJob{
		Success:  boolField(raw, "success"),
		JobID:    strField(raw, "job_id"),
		CrawlID:  strField(raw, "crawl_id"),
		URL:      strField(raw, "url"),
		Status:   strField(raw, "status"),
		MaxPages: intField(limits, "max_pages"),
		MaxDepth: intField(limits, "max_depth"),
		Links:    parseCrawlExtractLinks(mapField(raw, "links")),
		Raw:      raw,
	}, nil
}

func crawlExtractSpecBody(spec *CrawlExtractSpec) map[string]interface{} {
	out := map[string]interface{}{}
	if spec.EntityType != "" {
		out["entity_type"] = spec.EntityType
	}
	if len(spec.Fields) > 0 {
		out["fields"] = spec.Fields
	}
	if spec.Schema != nil {
		fields := make([]map[string]interface{}, 0, len(spec.Schema.Fields))
		for _, f := range spec.Schema.Fields {
			field := map[string]interface{}{"name": f.Name}
			if f.Selector != "" {
				field["selector"] = f.Selector
			}
			if f.Attribute != "" {
				field["attribute"] = f.Attribute
			}
			if f.Type != "" {
				field["type"] = f.Type
			}
			fields = append(fields, field)
		}
		schema := map[string]interface{}{"fields": fields}
		if spec.Schema.ItemSelector != "" {
			schema["item_selector"] = spec.Schema.ItemSelector
		}
		out["schema"] = schema
	}
	return out
}

// GetCrawlExtract gets a crawl-extract job's state once. An unknown job, or
// another organisation's, is an *ApiError with StatusCode 404.
func (w *WebgrphNamespace) GetCrawlExtract(ctx context.Context, jobID string) (*CrawlExtractStatus, error) {
	raw, err := w.t.get(ctx, pathf("webgrph/crawl-extract/%s", jobID), nil)
	if err != nil {
		return nil, err
	}
	return parseCrawlExtractStatus(raw), nil
}

// WaitForCrawlExtract polls a job until it ends and returns its last state,
// whatever it is (check Status). poll nil means 3 s and 60 min. A job still
// running at the timeout is an *ApiError with Code "job_timeout" and JobID.
//
// The wait also stops, without an error, when the job's crawl is held: the
// organization's quota ran out and the API paused the crawl (Crawl.Status
// "held_quota"). The returned status then has Held true and the job's own
// Status unchanged (the job is not finished). The crawl resumes by itself
// when quota returns and the job goes on under the same id; call
// WaitForCrawlExtract with that id again later. A crawl held for 30 days
// ends as failed with error code "held_quota_expired".
func (w *WebgrphNamespace) WaitForCrawlExtract(ctx context.Context, jobID string, poll *PollOptions) (*CrawlExtractStatus, error) {
	opts := PollOptions{Interval: DefaultCrawlExtractPollInterval, Timeout: DefaultCrawlExtractPollTimeout}
	if poll != nil {
		opts = poll.or(opts)
	}
	deadline := time.Now().Add(opts.Timeout)
	for {
		status, err := w.GetCrawlExtract(ctx, jobID)
		if err != nil {
			return nil, err
		}
		if status.Finished() || status.Held {
			return status, nil
		}
		if time.Now().After(deadline) {
			return nil, jobError("job_timeout", 0,
				fmt.Sprintf("crawl-extract job %s still %s after %s", jobID, status.Status, opts.Timeout), jobID)
		}
		if err := sleepCtx(ctx, opts.Interval); err != nil {
			return nil, err
		}
	}
}

// CrawlExtract starts a job and waits for it to end. A failed job is an
// *ApiError with Code "job_failed" and JobID; a cancelled one is returned.
// A job whose crawl is held for quota is returned unfinished with Held
// true (see WaitForCrawlExtract); check Held or Finished before reading
// its rows.
func (w *WebgrphNamespace) CrawlExtract(ctx context.Context, targetURL string, opts *CrawlExtractOptions) (*CrawlExtractStatus, error) {
	job, err := w.StartCrawlExtract(ctx, targetURL, opts)
	if err != nil {
		return nil, err
	}
	var poll *PollOptions
	if opts != nil {
		poll = &PollOptions{Interval: opts.PollInterval, Timeout: opts.PollTimeout}
	}
	status, err := w.WaitForCrawlExtract(ctx, job.JobID, poll)
	if err != nil {
		return nil, err
	}
	if status.Status == "failed" {
		msg := fmt.Sprintf("crawl-extract job %s failed", job.JobID)
		if status.Error != nil && *status.Error != "" {
			msg = *status.Error
		}
		return nil, jobError("job_failed", http.StatusOK, msg, job.JobID)
	}
	return status, nil
}

func checkCrawlExtractLimit(limit int) error {
	if limit < 0 || limit > CrawlExtractMaxLimit {
		return fmt.Errorf("kloakd: %w: limit must be 1-%d (0 for the default of %d), got %d",
			ErrInvalidCrawlExtractLimit, CrawlExtractMaxLimit, CrawlExtractDefaultLimit, limit)
	}
	return nil
}

func setParam(params map[string]string, key, value string) {
	if value != "" {
		params[key] = value
	}
}

// ListCrawlExtractResults gets one page of a job's rows. A running job
// answers with the rows written so far.
func (w *WebgrphNamespace) ListCrawlExtractResults(ctx context.Context, jobID string, opts *CrawlExtractResultsOptions) (*CrawlExtractResultsPage, error) {
	var o CrawlExtractResultsOptions
	if opts != nil {
		o = *opts
	}
	if err := checkCrawlExtractLimit(o.Limit); err != nil {
		return nil, err
	}
	params := map[string]string{}
	if o.Limit > 0 {
		params["limit"] = strconv.Itoa(o.Limit)
	}
	setParam(params, "cursor", o.Cursor)
	setParam(params, "shape", o.Shape)
	setParam(params, "page_type", o.PageType)
	setParam(params, "kind", o.Kind)
	raw, err := w.t.get(ctx, pathf("webgrph/crawl-extract/%s/results", jobID), params)
	if err != nil {
		return nil, err
	}
	page := &CrawlExtractResultsPage{
		JobID:      strField(raw, "job_id"),
		Status:     strField(raw, "status"),
		Total:      intField(raw, "total"),
		Count:      intField(raw, "count"),
		Rows:       []CrawlExtractRow{},
		NextCursor: strField(raw, "next_cursor"),
		Raw:        raw,
	}
	for _, r := range sliceMapField(raw, "rows") {
		page.Rows = append(page.Rows, CrawlExtractRow{
			PageURL:   strField(r, "page_url"),
			PageType:  optStrField(r, "page_type"),
			RowKind:   strField(r, "row_kind"),
			ItemIndex: intField(r, "item_index"),
			Data:      mapField(r, "data"),
			Shape:     strField(r, "shape"),
		})
	}
	return page, nil
}

// AllCrawlExtractResults follows NextCursor from opts.Cursor to the last
// page and returns every row. For a large job prefer the export.
func (w *WebgrphNamespace) AllCrawlExtractResults(ctx context.Context, jobID string, opts *CrawlExtractResultsOptions) ([]CrawlExtractRow, error) {
	var o CrawlExtractResultsOptions
	if opts != nil {
		o = *opts
	}
	rows := []CrawlExtractRow{}
	for {
		page, err := w.ListCrawlExtractResults(ctx, jobID, &o)
		if err != nil {
			return nil, err
		}
		rows = append(rows, page.Rows...)
		if page.NextCursor == "" {
			return rows, nil
		}
		o.Cursor = page.NextCursor
	}
}

// ListCrawlExtractPages lists a job's pages in crawl order, with each
// page's status, extraction method and item count.
func (w *WebgrphNamespace) ListCrawlExtractPages(ctx context.Context, jobID string, opts *CrawlExtractPagesOptions) (*CrawlExtractPagesResult, error) {
	var o CrawlExtractPagesOptions
	if opts != nil {
		o = *opts
	}
	if err := checkCrawlExtractLimit(o.Limit); err != nil {
		return nil, err
	}
	params := map[string]string{}
	if o.Limit > 0 {
		params["limit"] = strconv.Itoa(o.Limit)
	}
	if o.Offset > 0 {
		params["offset"] = strconv.Itoa(o.Offset)
	}
	setParam(params, "status", o.Status)
	raw, err := w.t.get(ctx, pathf("webgrph/crawl-extract/%s/pages", jobID), params)
	if err != nil {
		return nil, err
	}
	res := &CrawlExtractPagesResult{
		JobID:  strField(raw, "job_id"),
		Status: strField(raw, "status"),
		Count:  intField(raw, "count"),
		Offset: intField(raw, "offset"),
		Pages:  []CrawlExtractPage{},
		Raw:    raw,
	}
	for _, p := range sliceMapField(raw, "pages") {
		res.Pages = append(res.Pages, CrawlExtractPage{
			PageSeq:     intField(p, "page_seq"),
			PageURL:     strField(p, "page_url"),
			PageType:    optStrField(p, "page_type"),
			Title:       optStrField(p, "title"),
			Status:      strField(p, "status"),
			Method:      optStrField(p, "method"),
			ItemCount:   intField(p, "item_count"),
			Detail:      mapField(p, "detail"),
			ExtractedAt: optStrField(p, "extracted_at"),
		})
	}
	return res, nil
}

func exportFormat(format string) (string, error) {
	switch format {
	case "":
		return ExportJSON, nil
	case ExportJSON, ExportNDJSON, ExportCSV:
		return format, nil
	}
	return "", fmt.Errorf("kloakd: %w: %q (json, ndjson or csv)", ErrInvalidExportFormat, format)
}

func crawlExtractExportParams(opts *CrawlExtractExportOptions) (map[string]string, error) {
	var o CrawlExtractExportOptions
	if opts != nil {
		o = *opts
	}
	format, err := exportFormat(o.Format)
	if err != nil {
		return nil, err
	}
	params := map[string]string{"format": format}
	setParam(params, "shape", o.Shape)
	setParam(params, "page_type", o.PageType)
	setParam(params, "kind", o.Kind)
	return params, nil
}

// ExportCrawlExtract opens a job's rows as one download (json, ndjson or
// csv). The body is streamed, not held in memory, and is not bound by
// Config.Timeout (ctx ends it); the caller must Close it. JobStatus is the
// job's status when the export started and Crawl what its crawl did
// (X-Job-Status and the X-Crawl-* headers).
func (w *WebgrphNamespace) ExportCrawlExtract(ctx context.Context, jobID string, opts *CrawlExtractExportOptions) (*ExportDownload, error) {
	params, err := crawlExtractExportParams(opts)
	if err != nil {
		return nil, err
	}
	return w.t.download(ctx, pathf("webgrph/crawl-extract/%s/export", jobID), params)
}

// DownloadCrawlExtract writes a job's export to filePath as it arrives. The
// file is created only once the API has answered 200; if the download then
// breaks, the partial file is removed and the error returned.
func (w *WebgrphNamespace) DownloadCrawlExtract(ctx context.Context, jobID, filePath string, opts *CrawlExtractExportOptions) (*SavedExport, error) {
	dl, err := w.ExportCrawlExtract(ctx, jobID, opts)
	if err != nil {
		return nil, err
	}
	return saveExport(dl, filePath)
}

// CancelCrawlExtract stops a job. A job that has already ended is an
// *ApiError with StatusCode 409.
func (w *WebgrphNamespace) CancelCrawlExtract(ctx context.Context, jobID string) (*CrawlExtractCancelResult, error) {
	raw, err := w.t.post(ctx, pathf("webgrph/crawl-extract/%s/cancel", jobID), nil)
	if err != nil {
		return nil, err
	}
	return &CrawlExtractCancelResult{
		Success: boolField(raw, "success"),
		JobID:   strField(raw, "job_id"),
		CrawlID: strField(raw, "crawl_id"),
		Status:  strField(raw, "status"),
	}, nil
}

// ExportBatchJob opens a batch job's results as one download; format is
// ExportJSON (also when ""), ExportNDJSON or ExportCSV. The body is
// streamed and not bound by Config.Timeout; the caller must Close it. The
// batch export sends neither X-Job-Status nor X-Crawl-* headers: JobStatus
// is "" and Crawl is nil.
func (c *Client) ExportBatchJob(ctx context.Context, batchJobID, format string) (*ExportDownload, error) {
	format, err := exportFormat(format)
	if err != nil {
		return nil, err
	}
	return c.t.download(ctx, pathf("batch-jobs/%s/export", batchJobID), map[string]string{"format": format})
}

// DownloadBatchJob writes a batch job's export to filePath as it arrives
// (see Webgrph.DownloadCrawlExtract).
func (c *Client) DownloadBatchJob(ctx context.Context, batchJobID, filePath, format string) (*SavedExport, error) {
	dl, err := c.ExportBatchJob(ctx, batchJobID, format)
	if err != nil {
		return nil, err
	}
	return saveExport(dl, filePath)
}

func saveExport(dl *ExportDownload, filePath string) (*SavedExport, error) {
	defer dl.Body.Close()
	f, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("kloakd: create %s: %w", filePath, err)
	}
	n, err := io.Copy(f, dl.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(filePath)
		return nil, fmt.Errorf("kloakd: write export to %s: %w", filePath, err)
	}
	return &SavedExport{Path: filePath, Bytes: n, ContentType: dl.ContentType, Filename: dl.Filename, JobStatus: dl.JobStatus, Crawl: dl.Crawl}, nil
}

// download opens a streamed GET on the client without a Timeout, with the
// retry policy of send applied until an answer's headers arrive.
func (t *transport) download(ctx context.Context, path string, params map[string]string) (*ExportDownload, error) {
	reqURL := t.buildURL(path, params)
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return nil, fmt.Errorf("kloakd: create request: %w", err)
		}
		t.setHeaders(req, false)
		resp, err := t.streamClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if attempt >= t.maxRetries {
				return nil, fmt.Errorf("kloakd: GET %s: %w", reqURL, err)
			}
			if werr := sleepCtx(ctx, t.expBackoff(attempt)); werr != nil {
				return nil, werr
			}
			continue
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			filename := ""
			if _, cd, perr := mime.ParseMediaType(resp.Header.Get("Content-Disposition")); perr == nil {
				filename = cd["filename"]
			}
			return &ExportDownload{
				Body:        resp.Body,
				ContentType: resp.Header.Get("Content-Type"),
				Filename:    filename,
				JobStatus:   resp.Header.Get("X-Job-Status"),
				Crawl:       parseExportCrawl(resp.Header),
			}, nil
		}
		errBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		sdkErr := raiseForStatus(resp.StatusCode, decodeBody(errBody), resp.Header, errBody)
		if attempt >= t.maxRetries || !shouldRetry(http.MethodGet, sdkErr) {
			return nil, sdkErr
		}
		if werr := sleepCtx(ctx, t.retryWait(attempt, resp.Header, decodeBody(errBody))); werr != nil {
			return nil, werr
		}
	}
}

// ─── parsers ─────────────────────────────────────────────────────────────────

func parseCrawlExtractLinks(raw map[string]interface{}) CrawlExtractLinks {
	export := map[string]string{}
	for k, v := range mapField(raw, "export") {
		if s, ok := v.(string); ok {
			export[k] = s
		}
	}
	return CrawlExtractLinks{
		Status:      strField(raw, "status"),
		Results:     strField(raw, "results"),
		Pages:       strField(raw, "pages"),
		Cancel:      strField(raw, "cancel"),
		Export:      export,
		CrawlEvents: strField(raw, "crawl_events"),
		Events:      strField(raw, "events"),
	}
}

// parseExportCrawl reads an export's X-Crawl-* headers. nil when the
// answer has none of them.
func parseExportCrawl(h http.Header) *ExportCrawl {
	str := func(name string) *string {
		if vs := h.Values(name); len(vs) > 0 {
			return &vs[0]
		}
		return nil
	}
	num := func(name string) *int {
		if s := str(name); s != nil {
			if n, err := strconv.Atoi(strings.TrimSpace(*s)); err == nil {
				return &n
			}
		}
		return nil
	}
	flag := func(name string) *bool {
		if s := str(name); s != nil {
			if b, err := strconv.ParseBool(strings.TrimSpace(*s)); err == nil {
				return &b
			}
		}
		return nil
	}
	crawl := ExportCrawl{
		Status:          str("X-Crawl-Status"),
		IsPartial:       flag("X-Crawl-Partial"),
		PartialReason:   str("X-Crawl-Partial-Reason"),
		PagesDiscovered: num("X-Crawl-Pages-Discovered"),
		PagesFetched:    num("X-Crawl-Pages-Fetched"),
		PagesUnfetched:  num("X-Crawl-Pages-Unfetched"),
		Cancelled:       flag("X-Crawl-Cancelled"),
		RootUnfetched:   flag("X-Crawl-Root-Unfetched"),
	}
	if crawl == (ExportCrawl{}) {
		return nil
	}
	return &crawl
}

// parseCrawlExtractCrawl reads a status body's "crawl" object; nil when it
// is absent or null.
func parseCrawlExtractCrawl(raw map[string]interface{}) *CrawlExtractCrawl {
	c, ok := raw["crawl"].(map[string]interface{})
	if !ok {
		return nil
	}
	return &CrawlExtractCrawl{
		Status:          strField(c, "status"),
		IsPartial:       optBoolField(c, "is_partial"),
		PartialReason:   optStrField(c, "partial_reason"),
		PagesDiscovered: optIntField(c, "pages_discovered"),
		PagesFetched:    optIntField(c, "pages_fetched"),
		PagesUnfetched:  optIntField(c, "pages_unfetched"),
		Cancelled:       boolField(c, "cancelled"),
		RootUnfetched:   boolField(c, "root_unfetched"),
		Error:           optStrField(c, "error"),
		ErrorCode:       optStrField(c, "error_code"),
	}
}

func parseCrawlExtractStatus(raw map[string]interface{}) *CrawlExtractStatus {
	crawl := parseCrawlExtractCrawl(raw)
	progress := mapField(raw, "progress")
	methods := map[string]int{}
	byMethod := mapField(raw, "pages_by_method")
	for k := range byMethod {
		methods[k] = intField(byMethod, k)
	}
	shapes := []CrawlExtractShape{}
	for _, s := range sliceMapField(raw, "shapes") {
		shapes = append(shapes, CrawlExtractShape{
			Shape:     strField(s, "shape"),
			Kind:      strField(s, "kind"),
			Rows:      intField(s, "rows"),
			PageTypes: strSliceField(s, "page_types"),
			Fields:    strSliceField(s, "fields"),
		})
	}
	return &CrawlExtractStatus{
		JobID:           strField(raw, "job_id"),
		CrawlID:         strField(raw, "crawl_id"),
		URL:             strField(raw, "url"),
		Status:          strField(raw, "status"),
		CancelRequested: boolField(raw, "cancel_requested"),
		Progress: CrawlExtractProgress{
			PagesCrawled:   intField(progress, "pages_crawled"),
			PagesTotal:     intField(progress, "pages_total"),
			PagesExtracted: intField(progress, "pages_extracted"),
			PagesFailed:    intField(progress, "pages_failed"),
			Items:          intField(progress, "items"),
		},
		Error:         optStrField(raw, "error"),
		ErrorCode:     optStrField(raw, "error_code"),
		Request:       mapField(raw, "request"),
		CreatedAt:     optStrField(raw, "created_at"),
		StartedAt:     optStrField(raw, "started_at"),
		CompletedAt:   optStrField(raw, "completed_at"),
		UpdatedAt:     optStrField(raw, "updated_at"),
		Links:         parseCrawlExtractLinks(mapField(raw, "links")),
		PagesByMethod: methods,
		Shapes:        shapes,
		Crawl:         crawl,
		Held:          crawl != nil && crawl.Status == crawlHeldQuota,
		Raw:           raw,
	}
}
