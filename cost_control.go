package kloakd

import "context"

// Cost control (COST-CONTROL-CONTRACT.md, 15 Nov 2026).
//
// A page's extras (images, video and audio, fonts, stylesheets and
// tracking) are not loaded unless a job asks for them with
// IncludeResources; the page's document, scripts and data always are.
// MaxDataMB caps the data a crawl or crawl-extract may use: at the limit no
// new page is started, and the job ends partial with PartialReason
// "data_limit". The estimate says how much data a job should use and what
// share of the remaining allowance that is. MapSite lists a site's pages
// by branch before a crawl, so a job can fetch only the branches (or
// pages) it needs.

// Resource categories a job can include (IncludeResources). The page's
// document, scripts and data are always loaded and are not listed here.
const (
	ResourceImages      = "images"
	ResourceMedia       = "media"
	ResourceFonts       = "fonts"
	ResourceStylesheets = "stylesheets"
	ResourceTracking    = "tracking"
)

// PartialReasonDataLimit is the PartialReason of a crawl that stopped at
// its MaxDataMB. The API's message then reads "Stopped at your data limit
// of N MB. Continue with a higher limit to fetch more."; continue it with
// a higher ContinueOptions.MaxDataMB.
const PartialReasonDataLimit = "data_limit"

// costControl holds the cost-control fields of a crawl or crawl-extract
// start (CrawlOptions, CrawlExtractOptions; see their field docs). Zero
// values are not sent, and the API behaves as before: no extras and no
// per-job data limit.
type costControl struct {
	IncludeResources []string
	MaxDataMB        float64
	EstimateID       string
	CostAcknowledged bool
	URLs             []string
	IncludePaths     []string
}

// apply writes the set fields into a start request's body.
func (o costControl) apply(body map[string]interface{}) {
	if len(o.IncludeResources) > 0 {
		body["include_resources"] = o.IncludeResources
	}
	if o.MaxDataMB > 0 {
		body["max_data_mb"] = o.MaxDataMB
	}
	if o.EstimateID != "" {
		body["estimate_id"] = o.EstimateID
	}
	if o.CostAcknowledged {
		body["cost_acknowledged"] = true
	}
	if len(o.URLs) > 0 {
		body["urls"] = o.URLs
	}
	if len(o.IncludePaths) > 0 {
		body["include_paths"] = o.IncludePaths
	}
}

// DataRange is a range of an estimate in MB (or, for ShareOfRemaining, a
// share between 0 and 1). Low is nil when the API sent none (only TotalMB
// has one).
type DataRange struct {
	Low     *float64 `json:"low"`
	Typical float64  `json:"typical"`
	High    float64  `json:"high"`
}

// CrawlEstimateData is the data a job should use. Source says where the
// page weight came from: "site_profile" (measured pages of the host),
// "root_page" (the start page and what it references) or "typical"
// (typical page weights). TotalMBAllResources is the total with every
// extra loaded, and SavingMB what leaving the extras out saves. The
// per-category maps are keyed by document, script, data, images, media,
// fonts, stylesheets and tracking.
type CrawlEstimateData struct {
	Source              string             `json:"source"`
	PerPageMB           DataRange          `json:"per_page_mb"`
	TotalMB             DataRange          `json:"total_mb"`
	TotalMBAllResources DataRange          `json:"total_mb_all_resources"`
	SavingMB            DataRange          `json:"saving_mb"`
	ByCategoryMBPerPage map[string]float64 `json:"by_category_mb_per_page"`
	SavingByCategoryMB  map[string]float64 `json:"saving_by_category_mb"`
}

// MapSiteOptions configures Webgrph.MapSite. MaxURLs 0 sends none (the
// API's default).
type MapSiteOptions struct {
	MaxURLs int
}

// SiteMapBranch is one branch of a site map: a path prefix, how many URLs
// are under it, a few of them, and its sub-branches (up to 3 levels). A
// level is sorted by URLCount and capped at 50 branches, the rest grouped
// as "other".
type SiteMapBranch struct {
	Path       string          `json:"path"`
	URLCount   int             `json:"url_count"`
	SampleURLs []string        `json:"sample_urls"`
	Children   []SiteMapBranch `json:"children"`
}

// SiteMap is the answer of Webgrph.MapSite (POST webgrph/map). Source is
// "sitemap", "links" or "both"; Truncated is true when the site has more
// URLs than MaxURLs; DataUsedMB is the data the map used.
type SiteMap struct {
	URL        string          `json:"url"`
	Source     string          `json:"source"`
	TotalURLs  int             `json:"total_urls"`
	Truncated  bool            `json:"truncated"`
	DataUsedMB float64         `json:"data_used_mb"`
	Branches   []SiteMapBranch `json:"branches"`
	// Raw is the answer as received.
	Raw map[string]interface{} `json:"-"`
}

// MapSite lists a site's pages by branch before a crawl
// (POST webgrph/map): it reads robots.txt, the sitemaps and the start
// page's links, and fetches no other page. It is metered like a crawl and
// needs the same plan. Start a crawl or crawl-extract of the chosen
// branches with IncludePaths, or of chosen pages with
// URLs (CrawlOptions, CrawlExtractOptions).
func (w *WebgrphNamespace) MapSite(ctx context.Context, targetURL string, opts *MapSiteOptions) (*SiteMap, error) {
	body := map[string]interface{}{"url": targetURL}
	if opts != nil && opts.MaxURLs > 0 {
		body["max_urls"] = opts.MaxURLs
	}
	raw, err := w.t.post(ctx, "webgrph/map", body)
	if err != nil {
		return nil, err
	}
	return &SiteMap{
		URL:        strField(raw, "url"),
		Source:     strField(raw, "source"),
		TotalURLs:  intField(raw, "total_urls"),
		Truncated:  boolField(raw, "truncated"),
		DataUsedMB: float64Field(raw, "data_used_mb"),
		Branches:   parseSiteMapBranches(raw, "branches"),
		Raw:        raw,
	}, nil
}

func parseSiteMapBranches(m map[string]interface{}, key string) []SiteMapBranch {
	items := sliceMapField(m, key)
	branches := make([]SiteMapBranch, 0, len(items))
	for _, b := range items {
		branches = append(branches, SiteMapBranch{
			Path:       strField(b, "path"),
			URLCount:   intField(b, "url_count"),
			SampleURLs: strSliceField(b, "sample_urls"),
			Children:   parseSiteMapBranches(b, "children"),
		})
	}
	return branches
}

// Artifact types the artifact routes serve (Artifact.Type). A degraded
// fetch is a page stored from a fetch that did not fully succeed.
const (
	ArtifactFetchedContent         = "FETCHED_CONTENT"
	ArtifactDegradedFetchedContent = "DEGRADED_FETCHED_CONTENT"
)

// Artifact is a stored fetch's metadata (GET artifacts/{artifact_id}).
// Type is ArtifactFetchedContent or ArtifactDegradedFetchedContent.
// FinalURL is the URL after redirects and StatusCode the site's HTTP
// status (nil when not recorded); Verified says the fetch was verified.
// ContentURL is the path of the page's HTML (DownloadArtifact).
type Artifact struct {
	ID          string  `json:"id"`
	Type        string  `json:"type"`
	URL         *string `json:"url"`
	CreatedAt   *string `json:"created_at"`
	SizeBytes   *int    `json:"size_bytes"`
	ContentType *string `json:"content_type"`
	FinalURL    *string `json:"final_url"`
	StatusCode  *int    `json:"status_code"`
	Verified    bool    `json:"verified"`
	ContentURL  string  `json:"content_url"`
	// Raw is the answer as received.
	Raw map[string]interface{} `json:"-"`
}

// GetArtifact gets a fetched page's artifact metadata.
//
// Only the organisation's fetched-content artifacts are served. An unknown
// id, another organisation's, a session or cookie artifact, or one with no
// HTML is a 404 *ApiError ("Artifact not found"); the wrong organisation
// is a 403 *ForbiddenError; an id that is not a UUID is a 422 *ApiError.
func (c *Client) GetArtifact(ctx context.Context, artifactID string) (*Artifact, error) {
	raw, err := c.t.get(ctx, pathf("artifacts/%s", artifactID), nil)
	if err != nil {
		return nil, err
	}
	return &Artifact{
		ID:          strField(raw, "id"),
		Type:        strField(raw, "type"),
		URL:         optStrField(raw, "url"),
		CreatedAt:   optStrField(raw, "created_at"),
		SizeBytes:   optIntField(raw, "size_bytes"),
		ContentType: optStrField(raw, "content_type"),
		FinalURL:    optStrField(raw, "final_url"),
		StatusCode:  optIntField(raw, "status_code"),
		Verified:    boolField(raw, "verified"),
		ContentURL:  strField(raw, "content_url"),
		Raw:         raw,
	}, nil
}

// DownloadArtifact opens a fetched page's stored HTML
// (GET artifacts/{artifact_id}/content) as a stream of the raw bytes; the
// caller must Close Body. The API sends it as an attachment: ContentType
// "text/html; charset=utf-8" and Filename "<host>-<first 8 of the id>.html"
// from Content-Disposition. JobStatus is "" and Crawl nil. The errors are
// those of GetArtifact (404, 403, 422). Do not render the page with
// scripts enabled: it is the site's HTML.
func (c *Client) DownloadArtifact(ctx context.Context, artifactID string) (*ExportDownload, error) {
	return c.t.download(ctx, pathf("artifacts/%s/content", artifactID), nil)
}

// SaveArtifact writes a fetched page's stored HTML to filePath
// (DownloadArtifact, then copied to the file as it arrives).
func (c *Client) SaveArtifact(ctx context.Context, artifactID, filePath string) (*SavedExport, error) {
	dl, err := c.DownloadArtifact(ctx, artifactID)
	if err != nil {
		return nil, err
	}
	return saveExport(dl, filePath)
}

func parseDataRange(m map[string]interface{}) DataRange {
	return DataRange{
		Low:     optFloatField(m, "low"),
		Typical: float64Field(m, "typical"),
		High:    float64Field(m, "high"),
	}
}

func optDataRange(m map[string]interface{}, key string) *DataRange {
	r := optMapField(m, key)
	if r == nil {
		return nil
	}
	d := parseDataRange(r)
	return &d
}

func floatMapField(m map[string]interface{}, key string) map[string]float64 {
	src := mapField(m, key)
	out := make(map[string]float64, len(src))
	for k, v := range src {
		if f, ok := v.(float64); ok {
			out[k] = f
		}
	}
	return out
}

func parseCrawlEstimateData(raw map[string]interface{}) *CrawlEstimateData {
	d := optMapField(raw, "data")
	if d == nil {
		return nil
	}
	return &CrawlEstimateData{
		Source:              strField(d, "source"),
		PerPageMB:           parseDataRange(mapField(d, "per_page_mb")),
		TotalMB:             parseDataRange(mapField(d, "total_mb")),
		TotalMBAllResources: parseDataRange(mapField(d, "total_mb_all_resources")),
		SavingMB:            parseDataRange(mapField(d, "saving_mb")),
		ByCategoryMBPerPage: floatMapField(d, "by_category_mb_per_page"),
		SavingByCategoryMB:  floatMapField(d, "saving_by_category_mb"),
	}
}

// optStrSliceField is strSliceField, but nil when the key is absent or null.
func optStrSliceField(m map[string]interface{}, key string) []string {
	if _, ok := m[key].([]interface{}); !ok {
		return nil
	}
	return strSliceField(m, key)
}
