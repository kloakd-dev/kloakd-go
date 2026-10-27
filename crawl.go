package kloakd

import (
	"context"
	"fmt"
)

// Crawl is the high-level orchestrator that discovers, fetches, and optionally
// extracts structured data from all pages on a site in a single call.
//
// It internally:
//  1. Discovers pages via Webgrph BFS crawl
//  2. Fetches each page through the 4-tier anti-bot engine (Evadr)
//  3. Extracts structured data if ExtractSchema is provided (Kolektr)
//
// Per-page failures are caught and marked Success=false — the crawl never
// aborts on a single page error.
func (c *Client) Crawl(ctx context.Context, targetURL string, opts *CrawlOrchestratorOptions) (*SiteCrawlResult, error) {
	crawlOpts := &CrawlOptions{
		MaxDepth: 3,
		MaxPages: 100,
	}
	if opts != nil {
		if opts.MaxDepth > 0 {
			crawlOpts.MaxDepth = opts.MaxDepth
		}
		if opts.MaxPages > 0 {
			crawlOpts.MaxPages = opts.MaxPages
		}
		crawlOpts.IncludeExternalLinks = opts.IncludeExternalLinks
		if opts.SessionArtifactID != "" {
			crawlOpts.SessionArtifactID = opts.SessionArtifactID
		}
	}

	crawlResult, err := c.Webgrph.Crawl(ctx, targetURL, crawlOpts)
	if err != nil {
		return nil, fmt.Errorf("crawl: discovery failed: %w", err)
	}

	var pages []CrawlPage
	fetched := 0
	failed := 0

	for _, node := range crawlResult.Pages {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		fetchOpts := &FetchOptions{}
		if opts != nil && opts.SessionArtifactID != "" {
			fetchOpts.SessionArtifactID = opts.SessionArtifactID
		}

		fetchResult, fetchErr := c.Evadr.Fetch(ctx, node.URL, fetchOpts)
		if fetchErr != nil {
			errStr := fetchErr.Error()
			pages = append(pages, CrawlPage{
				Success: false,
				URL:     node.URL,
				Error:   &errStr,
			})
			failed++
			continue
		}

		var structuredData map[string]interface{}
		if opts != nil && opts.ExtractSchema != nil && fetchResult.Success {
			schema := make(map[string]interface{}, len(opts.ExtractSchema))
			for k, v := range opts.ExtractSchema {
				schema[k] = v
			}
			pageOpts := &ExtractOptions{
				SchemaSpec: schema,
			}
			if fetchResult.ArtifactID != nil {
				pageOpts.FetchArtifactID = *fetchResult.ArtifactID
			}
			extraction, extErr := c.Kolektr.Extract(ctx, node.URL, pageOpts)
			if extErr == nil && len(extraction.Records) > 0 {
				structuredData = extraction.Records[0]
			}
		}

		page := CrawlPage{
			Success:        fetchResult.Success,
			URL:            fetchResult.URL,
			StatusCode:     fetchResult.StatusCode,
			TierUsed:       fetchResult.TierUsed,
			HTML:           fetchResult.HTML,
			StructuredData: structuredData,
			ArtifactID:     fetchResult.ArtifactID,
			Error:          fetchResult.Error,
		}
		pages = append(pages, page)

		if page.Ok() {
			fetched++
		} else {
			failed++
		}
	}

	return &SiteCrawlResult{
		Success:              true,
		URL:                  targetURL,
		TotalPagesDiscovered: crawlResult.TotalPages,
		PagesFetched:         fetched,
		PagesFailed:          failed,
		Pages:                pages,
		CrawlArtifactID:      crawlResult.ArtifactID,
	}, nil
}

// CrawlStream is the streaming version of Crawl. It sends CrawlProgressEvent
// values on the returned channel as pages are discovered and fetched.
func (c *Client) CrawlStream(ctx context.Context, targetURL string, opts *CrawlOrchestratorOptions) (<-chan CrawlProgressEvent, error) {
	crawlOpts := &CrawlOptions{
		MaxDepth: 3,
		MaxPages: 100,
	}
	if opts != nil {
		if opts.MaxDepth > 0 {
			crawlOpts.MaxDepth = opts.MaxDepth
		}
		if opts.MaxPages > 0 {
			crawlOpts.MaxPages = opts.MaxPages
		}
		crawlOpts.IncludeExternalLinks = opts.IncludeExternalLinks
		if opts.SessionArtifactID != "" {
			crawlOpts.SessionArtifactID = opts.SessionArtifactID
		}
	}

	crawlResult, err := c.Webgrph.Crawl(ctx, targetURL, crawlOpts)
	if err != nil {
		return nil, fmt.Errorf("crawl_stream: discovery failed: %w", err)
	}

	ch := make(chan CrawlProgressEvent, 32)

	go func() {
		defer close(ch)

		total := crawlResult.TotalPages
		totalPtr := &total

		ch <- CrawlProgressEvent{
			Type:  "discovery_complete",
			URL:   &targetURL,
			Total: totalPtr,
			Metadata: map[string]interface{}{
				"pages_found": total,
			},
		}

		fetched := 0
		failed := 0
		var pages []CrawlPage

		for i, node := range crawlResult.Pages {
			select {
			case <-ctx.Done():
				return
			default:
			}

			pageNum := i + 1
			pageNumPtr := &pageNum
			nodeURL := node.URL

			ch <- CrawlProgressEvent{
				Type:  "page_fetching",
				URL:   &nodeURL,
				Page:  pageNumPtr,
				Total: totalPtr,
			}

			fetchOpts := &FetchOptions{}
			if opts != nil && opts.SessionArtifactID != "" {
				fetchOpts.SessionArtifactID = opts.SessionArtifactID
			}

			fetchResult, fetchErr := c.Evadr.Fetch(ctx, node.URL, fetchOpts)
			if fetchErr != nil {
				errStr := fetchErr.Error()
				pages = append(pages, CrawlPage{
					Success: false,
					URL:     node.URL,
					Error:   &errStr,
				})
				failed++
				falseVal := false
				ch <- CrawlProgressEvent{
					Type:    "page_failed",
					URL:     &nodeURL,
					Page:    pageNumPtr,
					Total:   totalPtr,
					Success: &falseVal,
					Error:   &errStr,
				}
				continue
			}

			var structuredData map[string]interface{}
			if opts != nil && opts.ExtractSchema != nil && fetchResult.Success {
				schema := make(map[string]interface{}, len(opts.ExtractSchema))
				for k, v := range opts.ExtractSchema {
					schema[k] = v
				}
				pageOpts := &ExtractOptions{
					SchemaSpec: schema,
				}
				if fetchResult.ArtifactID != nil {
					pageOpts.FetchArtifactID = *fetchResult.ArtifactID
				}
				extraction, extErr := c.Kolektr.Extract(ctx, node.URL, pageOpts)
				if extErr == nil && len(extraction.Records) > 0 {
					structuredData = extraction.Records[0]
				}
			}

			page := CrawlPage{
				Success:        fetchResult.Success,
				URL:            fetchResult.URL,
				StatusCode:     fetchResult.StatusCode,
				TierUsed:       fetchResult.TierUsed,
				HTML:           fetchResult.HTML,
				StructuredData: structuredData,
				ArtifactID:     fetchResult.ArtifactID,
				Error:          fetchResult.Error,
			}
			pages = append(pages, page)

			if page.Ok() {
				fetched++
				trueVal := true
				ch <- CrawlProgressEvent{
					Type:    "page_fetched",
					URL:     &nodeURL,
					Page:    pageNumPtr,
					Total:   totalPtr,
					Success: &trueVal,
					Metadata: map[string]interface{}{
						"tier_used": fetchResult.TierUsed,
					},
				}
			} else {
				failed++
				falseVal := false
				errStr := ""
				if fetchResult.Error != nil {
					errStr = *fetchResult.Error
				}
				ch <- CrawlProgressEvent{
					Type:    "page_failed",
					URL:     &nodeURL,
					Page:    pageNumPtr,
					Total:   totalPtr,
					Success: &falseVal,
					Error:   &errStr,
				}
			}
		}

		trueVal := true
		ch <- CrawlProgressEvent{
			Type:    "crawl_complete",
			URL:     &targetURL,
			Total:   totalPtr,
			Success: &trueVal,
			Metadata: map[string]interface{}{
				"result": SiteCrawlResult{
					Success:              true,
					URL:                  targetURL,
					TotalPagesDiscovered: total,
					PagesFetched:         fetched,
					PagesFailed:          failed,
					Pages:                pages,
					CrawlArtifactID:      crawlResult.ArtifactID,
				},
			},
		}
	}()

	return ch, nil
}
