package kloakd

import (
	"context"
	"fmt"
)

// WebgrphNamespace exposes the Site Mapping & Discovery module.
// Access via client.Webgrph.
type WebgrphNamespace struct {
	t *transport
}

// CrawlOptions configures optional parameters for Webgrph.Crawl.
type CrawlOptions struct {
	MaxDepth             int
	MaxPages             int
	IncludeExternalLinks bool
	SessionArtifactID    string
	Limit                int
	Offset               int
}

// Crawl performs a site crawl and returns the hierarchy result.
func (w *WebgrphNamespace) Crawl(ctx context.Context, targetURL string, opts *CrawlOptions) (*CrawlResult, error) {
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
		if opts.Limit > 0 {
			body["limit"] = opts.Limit
		}
		if opts.Offset > 0 {
			body["offset"] = opts.Offset
		}
	}

	raw, err := w.t.post(ctx, "webgrph/crawl", body)
	if err != nil {
		return nil, err
	}
	return parseCrawlResult(raw), nil
}

// CrawlAll auto-paginates until all pages are retrieved.
func (w *WebgrphNamespace) CrawlAll(ctx context.Context, targetURL string, opts *CrawlOptions) ([]PageNode, error) {
	limit := 100
	offset := 0
	var all []PageNode

	for {
		o := &CrawlOptions{}
		if opts != nil {
			*o = *opts
		}
		o.Limit = limit
		o.Offset = offset

		result, err := w.Crawl(ctx, targetURL, o)
		if err != nil {
			return nil, err
		}
		all = append(all, result.Pages...)
		if !result.HasMore {
			break
		}
		offset += len(result.Pages)
	}
	return all, nil
}

// CrawlStreamOptions configures optional parameters for Webgrph.CrawlStream.
type CrawlStreamOptions struct {
	MaxDepth          int
	MaxPages          int
	SessionArtifactID string
}

// CrawlStream opens a server-sent events stream for the crawl operation.
// Returns a read-only channel of CrawlEvent values.
func (w *WebgrphNamespace) CrawlStream(ctx context.Context, targetURL string, opts *CrawlStreamOptions) (<-chan CrawlEvent, error) {
	body := map[string]interface{}{"url": targetURL}
	if opts != nil {
		if opts.MaxDepth > 0 {
			body["max_depth"] = opts.MaxDepth
		}
		if opts.MaxPages > 0 {
			body["max_pages"] = opts.MaxPages
		}
		if opts.SessionArtifactID != "" {
			body["session_artifact_id"] = opts.SessionArtifactID
		}
	}

	raw, err := w.t.stream(ctx, "webgrph/crawl/stream", body)
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

// GetHierarchy retrieves a stored site hierarchy artifact by ID.
func (w *WebgrphNamespace) GetHierarchy(ctx context.Context, artifactID string) (map[string]interface{}, error) {
	return w.t.get(ctx, fmt.Sprintf("webgrph/hierarchy/%s", artifactID), nil)
}

// GetJob retrieves the status of a crawl job by ID.
func (w *WebgrphNamespace) GetJob(ctx context.Context, jobID string) (map[string]interface{}, error) {
	return w.t.get(ctx, fmt.Sprintf("webgrph/jobs/%s", jobID), nil)
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
		HasMore:         boolField(raw, "has_more"),
		Total:           intField(raw, "total"),
		ArtifactID:      optStrField(raw, "artifact_id"),
		Error:           optStrField(raw, "error"),
	}
}

func parsePageNode(raw map[string]interface{}) PageNode {
	return PageNode{
		URL:        strField(raw, "url"),
		Depth:      intField(raw, "depth"),
		Title:      optStrField(raw, "title"),
		StatusCode: optIntField(raw, "status_code"),
		Children:   strSliceField(raw, "children"),
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
