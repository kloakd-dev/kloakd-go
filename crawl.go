package kloakd

import (
	"context"
	"errors"
	"fmt"
)

// Crawl is the high-level orchestrator that discovers, fetches, and optionally
// extracts structured data from all pages on a site in a single call.
//
// It internally:
//  1. Discovers pages via a Webgrph crawl (Webgrph.Crawl)
//  2. Fetches each page through the anti-bot engine (Evadr.Fetch)
//  3. Extracts structured data if ExtractSchema or ExtractFields is given (Kolektr)
//
// A page whose fetch or extraction fails is recorded (Success=false, or
// ExtractionError) and the crawl goes on. Errors that would fail every page
// stop it and are returned: billing (PaymentRequiredError,
// QuotaExceededError, BillingUnavailableError), entitlement and
// authentication (NotEntitledError, AuthenticationError), RateLimitError,
// ConcurrencyLimitError and ctx's error. The result's Success and Error are
// the discovery crawl's.
func (c *Client) Crawl(ctx context.Context, targetURL string, opts *CrawlOrchestratorOptions) (*SiteCrawlResult, error) {
	crawlResult, err := c.Webgrph.Crawl(ctx, targetURL, orchestratorCrawlOptions(opts))
	if err != nil {
		return nil, fmt.Errorf("crawl: discovery failed: %w", err)
	}

	result := siteResult(targetURL, crawlResult)
	for _, node := range crawlResult.Pages {
		page, err := c.crawlPage(ctx, node.URL, opts)
		if err != nil {
			return nil, fmt.Errorf("crawl: %s: %w", node.URL, err)
		}
		result.add(page)
	}
	return result, nil
}

// CrawlStream is the streaming version of Crawl. It sends CrawlProgressEvent
// values on the returned channel as pages are fetched; the last is
// crawl_complete, or crawl_error (with Err) when an error stops the crawl
// (see Crawl). The channel closes after it, or when ctx is cancelled.
func (c *Client) CrawlStream(ctx context.Context, targetURL string, opts *CrawlOrchestratorOptions) (<-chan CrawlProgressEvent, error) {
	crawlResult, err := c.Webgrph.Crawl(ctx, targetURL, orchestratorCrawlOptions(opts))
	if err != nil {
		return nil, fmt.Errorf("crawl_stream: discovery failed: %w", err)
	}

	ch := make(chan CrawlProgressEvent, 32)
	go func() {
		defer close(ch)
		send := func(ev CrawlProgressEvent) bool {
			select {
			case ch <- ev:
				return true
			case <-ctx.Done():
				return false
			}
		}

		total := crawlResult.TotalPages
		totalPtr := &total
		if !send(CrawlProgressEvent{
			Type:     "discovery_complete",
			URL:      &targetURL,
			Total:    totalPtr,
			Success:  &crawlResult.Success,
			Error:    crawlResult.Error,
			Metadata: map[string]interface{}{"pages_found": total},
		}) {
			return
		}

		result := siteResult(targetURL, crawlResult)
		for i, node := range crawlResult.Pages {
			pageNum := i + 1
			nodeURL := node.URL
			if !send(CrawlProgressEvent{Type: "page_fetching", URL: &nodeURL, Page: &pageNum, Total: totalPtr}) {
				return
			}
			page, err := c.crawlPage(ctx, nodeURL, opts)
			if err != nil {
				msg := err.Error()
				f := false
				send(CrawlProgressEvent{Type: "crawl_error", URL: &nodeURL, Page: &pageNum, Total: totalPtr,
					Success: &f, Error: &msg, Err: err})
				return
			}
			result.add(page)
			ok := page.Ok()
			ev := CrawlProgressEvent{URL: &nodeURL, Page: &pageNum, Total: totalPtr, Success: &ok}
			if ok {
				ev.Type = "page_fetched"
				ev.Metadata = map[string]interface{}{"tier_used": page.TierUsed}
			} else {
				ev.Type = "page_failed"
				ev.Error = page.Error
			}
			if !send(ev) {
				return
			}
		}

		send(CrawlProgressEvent{
			Type:     "crawl_complete",
			URL:      &targetURL,
			Total:    totalPtr,
			Success:  &result.Success,
			Error:    result.Error,
			Metadata: map[string]interface{}{"result": *result},
		})
	}()
	return ch, nil
}

// orchestratorCrawlOptions is the discovery crawl's options. MaxDepth and
// MaxPages are sent only when set (the API applies the tier's defaults).
func orchestratorCrawlOptions(opts *CrawlOrchestratorOptions) *CrawlOptions {
	crawlOpts := &CrawlOptions{}
	if opts != nil {
		crawlOpts.MaxDepth = opts.MaxDepth
		crawlOpts.MaxPages = opts.MaxPages
		crawlOpts.IncludeExternalLinks = opts.IncludeExternalLinks
		crawlOpts.SessionArtifactID = opts.SessionArtifactID
		crawlOpts.Scope = opts.Scope
		crawlOpts.Budget = opts.Budget
		crawlOpts.CaptureMode = opts.CaptureMode
	}
	return crawlOpts
}

func siteResult(targetURL string, crawl *CrawlResult) *SiteCrawlResult {
	return &SiteCrawlResult{
		Success:              crawl.Success,
		URL:                  targetURL,
		TotalPagesDiscovered: crawl.TotalPages,
		Pages:                []CrawlPage{},
		CrawlArtifactID:      crawl.ArtifactID,
		Error:                crawl.Error,
	}
}

func (r *SiteCrawlResult) add(page CrawlPage) {
	r.Pages = append(r.Pages, page)
	if page.Ok() {
		r.PagesFetched++
	} else {
		r.PagesFailed++
	}
}

// crawlPage fetches (and extracts) one page. The returned error is one that
// stops the crawl (stopsCrawl); other failures are recorded on the page.
func (c *Client) crawlPage(ctx context.Context, pageURL string, opts *CrawlOrchestratorOptions) (CrawlPage, error) {
	if err := ctx.Err(); err != nil {
		return CrawlPage{}, err
	}
	fetchOpts := &FetchOptions{}
	if opts != nil {
		fetchOpts.SessionArtifactID = opts.SessionArtifactID
	}
	fetchResult, err := c.Evadr.Fetch(ctx, pageURL, fetchOpts)
	if err != nil {
		if stopsCrawl(ctx, err) {
			return CrawlPage{}, err
		}
		msg := err.Error()
		return CrawlPage{Success: false, URL: pageURL, Error: &msg}, nil
	}

	page := CrawlPage{
		Success:    fetchResult.Success,
		URL:        fetchResult.URL,
		StatusCode: fetchResult.StatusCode,
		TierUsed:   fetchResult.TierUsed,
		HTML:       fetchResult.HTML,
		ArtifactID: fetchResult.ArtifactID,
		Error:      fetchResult.Error,
	}
	if page.URL == "" {
		page.URL = pageURL
	}
	if opts == nil || !fetchResult.Success {
		return page, nil
	}
	spec := buildSchemaSpec(shorthandSchema(opts.ExtractSchema), opts.ExtractFields)
	if spec == nil {
		return page, nil
	}
	extractOpts := &ExtractOptions{SchemaSpec: spec, SessionArtifactID: opts.SessionArtifactID}
	if fetchResult.ArtifactID != nil {
		extractOpts.FetchArtifactID = *fetchResult.ArtifactID
	}
	extraction, err := c.Kolektr.Extract(ctx, pageURL, extractOpts)
	switch {
	case err != nil && stopsCrawl(ctx, err):
		return CrawlPage{}, err
	case err != nil:
		msg := err.Error()
		page.ExtractionError = &msg
	case !extraction.Success && extraction.Error != nil:
		page.ExtractionError = extraction.Error
	case len(extraction.Records) > 0:
		page.StructuredData = extraction.Records[0]
	}
	return page, nil
}

// stopsCrawl reports whether err would fail every remaining page.
func stopsCrawl(ctx context.Context, err error) bool {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var (
		payment     *PaymentRequiredError
		quota       *QuotaExceededError
		billing     *BillingUnavailableError
		entitled    *NotEntitledError
		auth        *AuthenticationError
		rate        *RateLimitError
		concurrency *ConcurrencyLimitError
	)
	return errors.As(err, &payment) || errors.As(err, &quota) || errors.As(err, &billing) ||
		errors.As(err, &entitled) || errors.As(err, &auth) || errors.As(err, &rate) ||
		errors.As(err, &concurrency)
}
