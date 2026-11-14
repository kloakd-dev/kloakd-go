package kloakd

import "context"

// A crawl maps the site's pages and links; crawl-extract also returns the
// items on each page as rows. Before starting either, EstimateCrawl and
// EstimateCrawlExtract say what the job will do: the depth and page cap
// after the plan's limits, how many pages the site seems to have, how long
// the job should take and how much data it uses. An estimate is not
// metered: no charge and no work slot.

// CrawlEstimateOptions configures Webgrph.EstimateCrawl. MaxDepth and
// MaxPages are sent only when set; otherwise the API uses the plan's
// defaults, as StartCrawl does.
type CrawlEstimateOptions struct {
	MaxDepth int
	MaxPages int
}

// CrawlExtractEstimateOptions configures Webgrph.EstimateCrawlExtract.
// Extract is the same spec StartCrawlExtract sends.
type CrawlExtractEstimateOptions struct {
	MaxDepth int
	MaxPages int
	Extract  *CrawlExtractSpec
}

// CrawlEstimatePlanLimits are the plan's limits the request was clamped to.
type CrawlEstimatePlanLimits struct {
	MaxDepth int    `json:"max_depth"`
	MaxPages int    `json:"max_pages"`
	PlanTier string `json:"plan_tier"`
}

// CrawlEstimateSite is what the API's probe of the site found. SitemapPages
// is nil when no sitemap was found; HomepageLinks (same-host links on the
// start page) is nil when the start page was not fetched; ProbeError says
// why the probe failed.
type CrawlEstimateSite struct {
	Reachable     bool    `json:"reachable"`
	SitemapPages  *int    `json:"sitemap_pages"`
	HomepageLinks *int    `json:"homepage_links"`
	ProbeError    *string `json:"probe_error"`
}

// CrawlEstimatePages is the estimated number of pages the job fetches.
// Source is "sitemap", "homepage_links" or "limit" ("limit": only the page
// cap is known, e.g. when the probe failed).
type CrawlEstimatePages struct {
	Low    int    `json:"low"`
	High   int    `json:"high"`
	Source string `json:"source"`
}

// CrawlEstimateDuration is the estimated duration in seconds.
type CrawlEstimateDuration struct {
	Low  float64 `json:"low"`
	High float64 `json:"high"`
}

// CrawlEstimate is the answer of Webgrph.EstimateCrawl and
// Webgrph.EstimateCrawlExtract (POST webgrph/crawl/estimate,
// POST webgrph/crawl-extract/estimate).
type CrawlEstimate struct {
	// Mode is "crawl" or "crawl_extract".
	Mode string `json:"mode"`
	URL  string `json:"url"`
	// MaxDepth and MaxPages are after clamping to the plan; Clamped is
	// true when a requested value was lowered.
	MaxDepth   int                     `json:"max_depth"`
	MaxPages   int                     `json:"max_pages"`
	PlanLimits CrawlEstimatePlanLimits `json:"plan_limits"`
	Clamped    bool                    `json:"clamped"`
	Site       CrawlEstimateSite       `json:"site"`
	EstPages   CrawlEstimatePages      `json:"est_pages"`
	// WillHitPageCap is true when the site seems larger than MaxPages.
	WillHitPageCap bool `json:"will_hit_page_cap"`
	// TimeBudgetS is the time budget in seconds the crawl will use.
	TimeBudgetS  int                   `json:"time_budget_s"`
	EstDurationS CrawlEstimateDuration `json:"est_duration_s"`
	// EstGB is the estimated data use; GBRemaining what the organisation
	// has left (nil when unknown); WillExhaustQuota whether the job would
	// use it up.
	EstGB            float64  `json:"est_gb"`
	GBRemaining      *float64 `json:"gb_remaining"`
	WillExhaustQuota bool     `json:"will_exhaust_quota"`
	// Warning is a plain sentence when there is something to warn about.
	Warning *string `json:"warning"`
	// Explanation says in plain words what the mode does.
	Explanation string `json:"explanation"`
	// Raw is the answer as received.
	Raw map[string]interface{} `json:"-"`
}

// EstimateCrawl estimates a crawl before it starts
// (POST webgrph/crawl/estimate). It is not metered. Start the crawl with
// StartCrawl or Crawl, using the estimate's MaxDepth and MaxPages.
func (w *WebgrphNamespace) EstimateCrawl(ctx context.Context, targetURL string, opts *CrawlEstimateOptions) (*CrawlEstimate, error) {
	body := map[string]interface{}{"url": targetURL}
	if opts != nil {
		setLimits(body, opts.MaxDepth, opts.MaxPages)
	}
	raw, err := w.t.post(ctx, "webgrph/crawl/estimate", body)
	if err != nil {
		return nil, err
	}
	return parseCrawlEstimate(raw), nil
}

// EstimateCrawlExtract estimates a crawl-extract job before it starts
// (POST webgrph/crawl-extract/estimate). It is not metered; the plan needs
// Kolektr, as for StartCrawlExtract.
func (w *WebgrphNamespace) EstimateCrawlExtract(ctx context.Context, targetURL string, opts *CrawlExtractEstimateOptions) (*CrawlEstimate, error) {
	body := map[string]interface{}{"url": targetURL}
	if opts != nil {
		setLimits(body, opts.MaxDepth, opts.MaxPages)
		if opts.Extract != nil {
			body["extract"] = crawlExtractSpecBody(opts.Extract)
		}
	}
	raw, err := w.t.post(ctx, "webgrph/crawl-extract/estimate", body)
	if err != nil {
		return nil, err
	}
	return parseCrawlEstimate(raw), nil
}

// ContinueOptions raises a crawl's limits when it is continued
// (Webgrph.ContinueCrawl, Webgrph.ContinueCrawlExtract). Zero values are
// not sent. The API clamps them to the plan; a value lower than the
// crawl's own is refused with 422 (Code "continue_cannot_lower").
type ContinueOptions struct {
	MaxPages int
	MaxDepth int
}

// continueBody is the continue request's body: nil (no body, the old
// behaviour) when no option is set.
func continueBody(opts []*ContinueOptions) map[string]interface{} {
	body := map[string]interface{}{}
	for _, o := range opts {
		if o != nil {
			setLimits(body, o.MaxDepth, o.MaxPages)
		}
	}
	if len(body) == 0 {
		return nil
	}
	return body
}

func setLimits(body map[string]interface{}, maxDepth, maxPages int) {
	if maxDepth > 0 {
		body["max_depth"] = maxDepth
	}
	if maxPages > 0 {
		body["max_pages"] = maxPages
	}
}

// CrawlExtractContinueResult is returned by Webgrph.ContinueCrawlExtract.
type CrawlExtractContinueResult struct {
	Success bool
	JobID   string
	CrawlID string
	Status  string
	// Raw is the answer as received.
	Raw map[string]interface{}
}

// ContinueCrawlExtract continues a crawl-extract job's crawl and then
// extracts the new pages (POST webgrph/crawl-extract/{job_id}/continue).
// With no options it sends no body; with options it raises MaxPages and
// MaxDepth. The job keeps its id; wait for it again with
// WaitForCrawlExtract. A job that cannot be continued is an *ApiError
// (409; 422 for a lowered value; 501 if the API does not offer it yet).
func (w *WebgrphNamespace) ContinueCrawlExtract(ctx context.Context, jobID string, opts ...*ContinueOptions) (*CrawlExtractContinueResult, error) {
	raw, err := w.t.post(ctx, pathf("webgrph/crawl-extract/%s/continue", jobID), continueBody(opts))
	if err != nil {
		return nil, err
	}
	return &CrawlExtractContinueResult{
		Success: boolField(raw, "success"),
		JobID:   strField(raw, "job_id"),
		CrawlID: strField(raw, "crawl_id"),
		Status:  strField(raw, "status"),
		Raw:     raw,
	}, nil
}

func parseCrawlEstimate(raw map[string]interface{}) *CrawlEstimate {
	limits := mapField(raw, "plan_limits")
	site := mapField(raw, "site")
	pages := mapField(raw, "est_pages")
	duration := mapField(raw, "est_duration_s")
	return &CrawlEstimate{
		Mode:     strField(raw, "mode"),
		URL:      strField(raw, "url"),
		MaxDepth: intField(raw, "max_depth"),
		MaxPages: intField(raw, "max_pages"),
		PlanLimits: CrawlEstimatePlanLimits{
			MaxDepth: intField(limits, "max_depth"),
			MaxPages: intField(limits, "max_pages"),
			PlanTier: strField(limits, "plan_tier"),
		},
		Clamped: boolField(raw, "clamped"),
		Site: CrawlEstimateSite{
			Reachable:     boolField(site, "reachable"),
			SitemapPages:  optIntField(site, "sitemap_pages"),
			HomepageLinks: optIntField(site, "homepage_links"),
			ProbeError:    optStrField(site, "probe_error"),
		},
		EstPages: CrawlEstimatePages{
			Low:    intField(pages, "low"),
			High:   intField(pages, "high"),
			Source: strField(pages, "source"),
		},
		WillHitPageCap:   boolField(raw, "will_hit_page_cap"),
		TimeBudgetS:      intField(raw, "time_budget_s"),
		EstDurationS:     CrawlEstimateDuration{Low: float64Field(duration, "low"), High: float64Field(duration, "high")},
		EstGB:            float64Field(raw, "est_gb"),
		GBRemaining:      optFloatField(raw, "gb_remaining"),
		WillExhaustQuota: boolField(raw, "will_exhaust_quota"),
		Warning:          optStrField(raw, "warning"),
		Explanation:      strField(raw, "explanation"),
		Raw:              raw,
	}
}
