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
	// IncludeResources are the extras the job would load (see
	// CrawlOptions.IncludeResources); the estimate's Data counts them.
	IncludeResources []string
	// CaptureMode is the job's capture mode, sent as capture_mode when
	// set: "visual" locks images and stylesheets on
	// (CrawlEstimate.IncludeResourcesLocked).
	CaptureMode string
	// RobotsAcknowledgedHosts acknowledges a robots.txt stop
	// (*RobotsDisallowedError) for these hosts in this request only, sent
	// as robots_acknowledged_hosts when set. Name a host only when you
	// have the right to access its content despite its robots.txt; KLOAKD
	// records the acknowledgement.
	RobotsAcknowledgedHosts []string
}

// CrawlExtractEstimateOptions configures Webgrph.EstimateCrawlExtract.
// Extract is the same spec StartCrawlExtract sends.
type CrawlExtractEstimateOptions struct {
	MaxDepth int
	MaxPages int
	Extract  *CrawlExtractSpec
	// IncludeResources and CaptureMode as in CrawlEstimateOptions.
	IncludeResources []string
	CaptureMode      string
	// RobotsAcknowledgedHosts acknowledges a robots.txt stop
	// (*RobotsDisallowedError) for these hosts in this request only, sent
	// as robots_acknowledged_hosts when set. Name a host only when you
	// have the right to access its content despite its robots.txt; KLOAKD
	// records the acknowledgement.
	RobotsAcknowledgedHosts []string
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

	// Cost control (COST-CONTROL-CONTRACT.md, 15 Nov 2026). These are
	// zero when the API did not send them.
	//
	// IncludeResources is the effective set of extras the estimate
	// counts (none by default; the locked ones included).
	// IncludeResourcesLocked lists the extras the user cannot turn off:
	// CaptureMode "visual" locks images and stylesheets. nil when the API
	// did not send it (an older API). Data is the data the job should use, with what leaving
	// the extras out saves; nil when not sent. EstGB equals
	// Data.TotalMB.Typical / 1000.
	IncludeResources       []string           `json:"include_resources"`
	IncludeResourcesLocked []string           `json:"include_resources_locked"`
	Data                   *CrawlEstimateData `json:"data"`
	// ShareOfRemaining is the share (0–1) of the remaining allowance the
	// job would use; nil when unknown.
	ShareOfRemaining *DataRange `json:"share_of_remaining"`
	// ConsentRequired is true when the job would use a large share of the
	// allowance (or all of it). Start it with EstimateID and
	// CostAcknowledged true once the user has accepted Risks; a dashboard
	// session is refused without them (409
	// *CostAcknowledgementRequiredError), an API key is not.
	ConsentRequired bool     `json:"consent_required"`
	Risks           []string `json:"risks"`
	// SuggestedMaxDataMB is a data limit for the job (Data.TotalMB.High,
	// rounded up); nil when not sent.
	SuggestedMaxDataMB *float64 `json:"suggested_max_data_mb"`
	// EstimateID is an opaque token for this estimate, valid 30 minutes.
	EstimateID string `json:"estimate_id"`
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
		costControl{IncludeResources: opts.IncludeResources}.apply(body)
		if opts.CaptureMode != "" {
			body["capture_mode"] = opts.CaptureMode
		}
		applyRobotsAck(body, opts.RobotsAcknowledgedHosts)
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
		costControl{IncludeResources: opts.IncludeResources}.apply(body)
		if opts.CaptureMode != "" {
			body["capture_mode"] = opts.CaptureMode
		}
		applyRobotsAck(body, opts.RobotsAcknowledgedHosts)
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
//
// MaxDataMB raises the job's data limit in MB (a crawl that stopped with
// PartialReason "data_limit" continues with a higher one).
type ContinueOptions struct {
	MaxPages  int
	MaxDepth  int
	MaxDataMB float64
	// RobotsAcknowledgedHosts acknowledges a robots.txt stop
	// (*RobotsDisallowedError) for these hosts in this request only, sent
	// as robots_acknowledged_hosts when set. Name a host only when you
	// have the right to access its content despite its robots.txt; KLOAKD
	// records the acknowledgement.
	RobotsAcknowledgedHosts []string
}

// continueBody is the continue request's body: nil (no body, the old
// behaviour) when no option is set.
func continueBody(opts []*ContinueOptions) map[string]interface{} {
	body := map[string]interface{}{}
	for _, o := range opts {
		if o != nil {
			setLimits(body, o.MaxDepth, o.MaxPages)
			costControl{MaxDataMB: o.MaxDataMB}.apply(body)
			applyRobotsAck(body, o.RobotsAcknowledgedHosts)
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

// CrawlExtractContinueResult is returned by Webgrph.ContinueCrawlExtract
// (HTTP 202). MaxPages and MaxDepth are the job's limits after the
// continue (limits); TimeBudgetS, ElapsedS and MaxDataMB are nil when the
// API did not send them.
type CrawlExtractContinueResult struct {
	Success     bool
	JobID       string
	CrawlID     string
	Status      string
	MaxPages    int
	MaxDepth    int
	Links       CrawlExtractLinks
	TimeBudgetS *int
	ElapsedS    *float64
	MaxDataMB   *float64
	// Raw is the answer as received.
	Raw map[string]interface{}
}

// ContinueCrawlExtract continues a crawl-extract job's crawl and then
// extracts the new pages (POST webgrph/crawl-extract/{job_id}/continue;
// the API answers 202). With no options it sends no body; with options it
// raises MaxPages and MaxDepth (and MaxDataMB). The job keeps its id;
// wait for it again with WaitForCrawlExtract.
//
// A job that cannot be continued is an *ApiError whose Code is the API's
// detail.error: 409 "continue_needs_more_pages" (it stopped at max_pages:
// give a larger MaxPages), "continue_needs_more_data" (it stopped at its
// data limit: give a larger MaxDataMB), "not_continuable" or
// "already_continuing"; 422 "continue_cannot_lower" for a lowered value.
func (w *WebgrphNamespace) ContinueCrawlExtract(ctx context.Context, jobID string, opts ...*ContinueOptions) (*CrawlExtractContinueResult, error) {
	raw, err := w.t.post(ctx, pathf("webgrph/crawl-extract/%s/continue", jobID), continueBody(opts))
	if err != nil {
		return nil, err
	}
	return &CrawlExtractContinueResult{
		Success:     boolField(raw, "success"),
		JobID:       strField(raw, "job_id"),
		CrawlID:     strField(raw, "crawl_id"),
		Status:      strField(raw, "status"),
		MaxPages:    intField(mapField(raw, "limits"), "max_pages"),
		MaxDepth:    intField(mapField(raw, "limits"), "max_depth"),
		Links:       parseCrawlExtractLinks(mapField(raw, "links")),
		TimeBudgetS: optIntField(raw, "time_budget_s"),
		ElapsedS:    optFloatField(raw, "elapsed_s"),
		MaxDataMB:   optFloatField(raw, "max_data_mb"),
		Raw:         raw,
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
		WillHitPageCap:         boolField(raw, "will_hit_page_cap"),
		TimeBudgetS:            intField(raw, "time_budget_s"),
		EstDurationS:           CrawlEstimateDuration{Low: float64Field(duration, "low"), High: float64Field(duration, "high")},
		EstGB:                  float64Field(raw, "est_gb"),
		GBRemaining:            optFloatField(raw, "gb_remaining"),
		WillExhaustQuota:       boolField(raw, "will_exhaust_quota"),
		Warning:                optStrField(raw, "warning"),
		Explanation:            strField(raw, "explanation"),
		IncludeResources:       optStrSliceField(raw, "include_resources"),
		IncludeResourcesLocked: optStrSliceField(raw, "include_resources_locked"),
		Data:                   parseCrawlEstimateData(raw),
		ShareOfRemaining:       optDataRange(raw, "share_of_remaining"),
		ConsentRequired:        boolField(raw, "consent_required"),
		Risks:                  optStrSliceField(raw, "risks"),
		SuggestedMaxDataMB:     optFloatField(raw, "suggested_max_data_mb"),
		EstimateID:             strField(raw, "estimate_id"),
		Raw:                    raw,
	}
}
