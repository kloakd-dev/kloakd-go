package kloakd

import (
	"context"
	"fmt"
)

// SkanyrNamespace exposes the Site Knowledge module: synced sites, their
// resources, extraction and search.
//
// In 0.3.0 the discovery methods of 0.2 (Discover, DiscoverStream,
// AnalyzeBundle, Hierarchy, ExpandNode, Retry, Health, GetApiMap, GetJob,
// GetDiscovery*) are removed: the API no longer serves those routes. The
// session, detected-API and reader-view methods moved to Webgrph.
// Access via client.Skanyr.
type SkanyrNamespace struct {
	t *transport
}

// ListSites lists the organisation's synced sites (under "items").
func (s *SkanyrNamespace) ListSites(ctx context.Context) (map[string]interface{}, error) {
	return s.t.get(ctx, "skanyr/sites", nil)
}

// CreateSiteFromCrawl registers a site from a finished Webgrph crawl.
func (s *SkanyrNamespace) CreateSiteFromCrawl(ctx context.Context, crawlID string) (map[string]interface{}, error) {
	return s.t.post(ctx, fmt.Sprintf("skanyr/sites/from-crawl/%s", crawlID), nil)
}

// GetSite gets a synced site.
func (s *SkanyrNamespace) GetSite(ctx context.Context, siteID string) (map[string]interface{}, error) {
	return s.t.get(ctx, fmt.Sprintf("skanyr/sites/%s", siteID), nil)
}

// SetSitePolicy sets a site's sync policy.
func (s *SkanyrNamespace) SetSitePolicy(ctx context.Context, siteID, policy string) (map[string]interface{}, error) {
	return s.t.patch(ctx, fmt.Sprintf("skanyr/sites/%s", siteID), map[string]interface{}{"policy": policy})
}

// DeleteSite deletes a synced site.
func (s *SkanyrNamespace) DeleteSite(ctx context.Context, siteID string) error {
	return s.t.delete(ctx, fmt.Sprintf("skanyr/sites/%s", siteID))
}

// SiteResources lists a site's resources.
func (s *SkanyrNamespace) SiteResources(ctx context.Context, siteID string) (map[string]interface{}, error) {
	return s.t.get(ctx, fmt.Sprintf("skanyr/sites/%s/resources", siteID), nil)
}

// SiteAPIs lists the APIs found on a site.
func (s *SkanyrNamespace) SiteAPIs(ctx context.Context, siteID string) (map[string]interface{}, error) {
	return s.t.get(ctx, fmt.Sprintf("skanyr/sites/%s/apis", siteID), nil)
}

// SyncOptions configures optional parameters for Skanyr.Sync.
type SyncOptions struct {
	URLs   []string
	Policy string
}

// Sync re-fetches a site's changed resources.
func (s *SkanyrNamespace) Sync(ctx context.Context, siteID string, opts *SyncOptions) (map[string]interface{}, error) {
	body := map[string]interface{}{"site_id": siteID}
	if opts != nil {
		if len(opts.URLs) > 0 {
			body["urls"] = opts.URLs
		}
		if opts.Policy != "" {
			body["policy"] = opts.Policy
		}
	}
	return s.t.post(ctx, "skanyr/sync", body)
}

// SearchOptions configures optional parameters for Skanyr.Search.
type SearchOptions struct {
	SiteID   string
	MimeType string
	Limit    int
	Offset   int
}

// Search searches extracted content.
func (s *SkanyrNamespace) Search(ctx context.Context, query string, opts *SearchOptions) (map[string]interface{}, error) {
	body := map[string]interface{}{"query": query}
	if opts != nil {
		if opts.SiteID != "" {
			body["site_id"] = opts.SiteID
		}
		if opts.MimeType != "" {
			body["mime_type"] = opts.MimeType
		}
		if opts.Limit > 0 {
			body["limit"] = opts.Limit
		}
		if opts.Offset > 0 {
			body["offset"] = opts.Offset
		}
	}
	return s.t.post(ctx, "skanyr/search", body)
}

// Extract extracts text and structure from stored content.
func (s *SkanyrNamespace) Extract(ctx context.Context, contentHash, mimeType string, metadata map[string]interface{}) (map[string]interface{}, error) {
	body := map[string]interface{}{"content_hash": contentHash, "mime_type": mimeType}
	if len(metadata) > 0 {
		body["metadata"] = metadata
	}
	return s.t.post(ctx, "skanyr/extract", body)
}
