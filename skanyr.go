package kloakd

import "context"

// SkanyrNamespace exposes the Site Knowledge module: sites built from
// crawls, their resources, search, change history, the resource graph and
// technology findings (modules/skanyr/router.py).
//
// Page discovery is Webgrph.Crawl; the session, detected-API and
// reader-view methods are on Webgrph. Access via client.Skanyr.
type SkanyrNamespace struct {
	t *transport
}

// ListPage bounds a Skanyr list (zero values: the API's defaults).
type ListPage struct {
	Limit  int
	Offset int
}

func (p *ListPage) params() map[string]string {
	params := map[string]string{}
	if p != nil {
		setInt(params, "limit", p.Limit)
		setInt(params, "offset", p.Offset)
	}
	return params
}

func setStr(params map[string]string, key, v string) {
	if v != "" {
		params[key] = v
	}
}

// ─── Sites ───────────────────────────────────────────────────────────────────

// ListSites lists the organisation's sites.
func (s *SkanyrNamespace) ListSites(ctx context.Context) ([]map[string]interface{}, error) {
	return s.t.getList(ctx, "skanyr/sites", nil)
}

// CreateSiteFromCrawl builds (or updates) a site from a finished Webgrph crawl.
func (s *SkanyrNamespace) CreateSiteFromCrawl(ctx context.Context, crawlID string) (map[string]interface{}, error) {
	return s.t.post(ctx, pathf("skanyr/sites/from-crawl/%s", crawlID), nil)
}

// GetSite gets a site.
func (s *SkanyrNamespace) GetSite(ctx context.Context, siteID string) (map[string]interface{}, error) {
	return s.t.get(ctx, pathf("skanyr/sites/%s", siteID), nil)
}

// UpdateSite sets a site's default sync policy ("full", "incremental",
// "conditional" or "head_first"), the one setting the API lets you change.
func (s *SkanyrNamespace) UpdateSite(ctx context.Context, siteID, policy string) (map[string]interface{}, error) {
	return s.t.patch(ctx, pathf("skanyr/sites/%s", siteID), map[string]interface{}{"policy": policy})
}

// SetSitePolicy is UpdateSite.
//
// Deprecated: use UpdateSite.
func (s *SkanyrNamespace) SetSitePolicy(ctx context.Context, siteID, policy string) (map[string]interface{}, error) {
	return s.UpdateSite(ctx, siteID, policy)
}

// DeleteSite deletes a site.
func (s *SkanyrNamespace) DeleteSite(ctx context.Context, siteID string) error {
	return s.t.delete(ctx, pathf("skanyr/sites/%s", siteID))
}

// ResourceListOptions filters a resource list.
type ResourceListOptions struct {
	ResourceType string
	Limit        int
	Offset       int
}

// SiteResources lists a site's resources.
func (s *SkanyrNamespace) SiteResources(ctx context.Context, siteID string, opts *ResourceListOptions) ([]map[string]interface{}, error) {
	params := map[string]string{}
	if opts != nil {
		setStr(params, "resource_type", opts.ResourceType)
		setInt(params, "limit", opts.Limit)
		setInt(params, "offset", opts.Offset)
	}
	return s.t.getList(ctx, pathf("skanyr/sites/%s/resources", siteID), params)
}

// SiteAPIs lists the APIs found on a site.
func (s *SkanyrNamespace) SiteAPIs(ctx context.Context, siteID string, page *ListPage) ([]map[string]interface{}, error) {
	return s.t.getList(ctx, pathf("skanyr/sites/%s/apis", siteID), page.params())
}

// SyncOptions configures optional parameters for Skanyr.Sync.
type SyncOptions struct {
	URLs []string
	// Policy is "full", "incremental", "conditional" or "head_first"
	// (default: the site's policy, else incremental).
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

// ExportOptions configures Skanyr.ExportSite.
type ExportOptions struct {
	// ResourceIDs selects the resources; nil exports all with content.
	ResourceIDs []string
	// MaxBytes bounds the archive (0: the API's default, 500 MiB).
	MaxBytes int
}

// ExportSite exports a site's captured content and returns the ZIP archive.
func (s *SkanyrNamespace) ExportSite(ctx context.Context, siteID string, opts *ExportOptions) ([]byte, error) {
	body := map[string]interface{}{}
	if opts != nil {
		if opts.ResourceIDs != nil {
			body["resource_ids"] = opts.ResourceIDs
		}
		if opts.MaxBytes > 0 {
			body["max_bytes"] = opts.MaxBytes
		}
	}
	raw, _, err := s.t.postRaw(ctx, pathf("skanyr/sites/%s/export", siteID), body)
	return raw, err
}

// SiteSearchOptions configures Skanyr.SearchSite.
type SiteSearchOptions struct {
	Limit    int
	Offset   int
	MimeType string
}

// SearchSite searches one site's content.
func (s *SkanyrNamespace) SearchSite(ctx context.Context, siteID, query string, opts *SiteSearchOptions) (map[string]interface{}, error) {
	params := map[string]string{"q": query}
	if opts != nil {
		setInt(params, "limit", opts.Limit)
		setInt(params, "offset", opts.Offset)
		setStr(params, "mime_type", opts.MimeType)
	}
	return s.t.get(ctx, pathf("skanyr/sites/%s/search", siteID), params)
}

// FindingsOptions filters Skanyr.GetFindings.
type FindingsOptions struct {
	FindingKind string
	Limit       int
	Offset      int
}

// GetFindings lists a site's findings.
func (s *SkanyrNamespace) GetFindings(ctx context.Context, siteID string, opts *FindingsOptions) ([]map[string]interface{}, error) {
	params := map[string]string{}
	if opts != nil {
		setStr(params, "finding_kind", opts.FindingKind)
		setInt(params, "limit", opts.Limit)
		setInt(params, "offset", opts.Offset)
	}
	return s.t.getList(ctx, pathf("skanyr/sites/%s/findings", siteID), params)
}

// GetTechnologies lists the technologies detected on a site, aggregated.
func (s *SkanyrNamespace) GetTechnologies(ctx context.Context, siteID string) ([]map[string]interface{}, error) {
	return s.t.getList(ctx, pathf("skanyr/sites/%s/technologies", siteID), nil)
}

// TechnologyEvidenceOptions filters Skanyr.GetTechnologyEvidence.
type TechnologyEvidenceOptions struct {
	Detector string
	Limit    int
	Offset   int
}

// GetTechnologyEvidence lists the evidence for one technology (subject,
// e.g. "React") on a site: {total, subject, evidence}.
func (s *SkanyrNamespace) GetTechnologyEvidence(ctx context.Context, siteID, subject string, opts *TechnologyEvidenceOptions) (map[string]interface{}, error) {
	params := map[string]string{"subject": subject}
	if opts != nil {
		setStr(params, "detector", opts.Detector)
		setInt(params, "limit", opts.Limit)
		setInt(params, "offset", opts.Offset)
	}
	return s.t.get(ctx, pathf("skanyr/sites/%s/technology-evidence", siteID), params)
}

// ─── History ─────────────────────────────────────────────────────────────────

// ListSnapshots lists a site's sync snapshots (limit 0: the API's default).
func (s *SkanyrNamespace) ListSnapshots(ctx context.Context, siteID string, limit int) (map[string]interface{}, error) {
	params := map[string]string{}
	setInt(params, "limit", limit)
	return s.t.get(ctx, pathf("skanyr/sites/%s/snapshots", siteID), params)
}

// GetTimeline gets a site's change timeline (limit 0: the API's default).
func (s *SkanyrNamespace) GetTimeline(ctx context.Context, siteID string, limit int) (map[string]interface{}, error) {
	params := map[string]string{}
	setInt(params, "limit", limit)
	return s.t.get(ctx, pathf("skanyr/sites/%s/timeline", siteID), params)
}

// GetTimelineSnapshot gets one snapshot of a site's timeline, with its entries.
func (s *SkanyrNamespace) GetTimelineSnapshot(ctx context.Context, siteID, snapshotID string) (map[string]interface{}, error) {
	return s.t.get(ctx, pathf("skanyr/sites/%s/timeline/%s", siteID, snapshotID), nil)
}

// Diff compares two snapshots of a site ("" for the API's defaults).
func (s *SkanyrNamespace) Diff(ctx context.Context, siteID, snapshotA, snapshotB string) (map[string]interface{}, error) {
	params := map[string]string{}
	setStr(params, "snapshot_a", snapshotA)
	setStr(params, "snapshot_b", snapshotB)
	return s.t.get(ctx, pathf("skanyr/sites/%s/diff", siteID), params)
}

// ─── Graph ───────────────────────────────────────────────────────────────────

// GraphOptions filters Skanyr.GetGraph.
type GraphOptions struct {
	ResourceType string
	MaxDepth     int
	EdgeKind     string
	Limit        int
}

// GetGraph gets a site's resource graph.
func (s *SkanyrNamespace) GetGraph(ctx context.Context, siteID string, opts *GraphOptions) (map[string]interface{}, error) {
	params := map[string]string{}
	if opts != nil {
		setStr(params, "resource_type", opts.ResourceType)
		setInt(params, "max_depth", opts.MaxDepth)
		setStr(params, "edge_kind", opts.EdgeKind)
		setInt(params, "limit", opts.Limit)
	}
	return s.t.get(ctx, pathf("skanyr/sites/%s/graph", siteID), params)
}

// GetGraphStats gets a site's graph statistics.
func (s *SkanyrNamespace) GetGraphStats(ctx context.Context, siteID string) (map[string]interface{}, error) {
	return s.t.get(ctx, pathf("skanyr/sites/%s/graph/stats", siteID), nil)
}

// GetSubgraph gets the graph around one resource, hops away (0: the API's default, 2).
func (s *SkanyrNamespace) GetSubgraph(ctx context.Context, siteID, resourceID string, hops int) (map[string]interface{}, error) {
	params := map[string]string{}
	setInt(params, "hops", hops)
	return s.t.get(ctx, pathf("skanyr/sites/%s/graph/subgraph/%s", siteID, resourceID), params)
}

// ─── Resources, search, extraction ───────────────────────────────────────────

// ResourcesOptions filters Skanyr.ListResources.
type ResourcesOptions struct {
	SiteID       string
	ResourceType string
	Limit        int
	Offset       int
}

// ListResources lists the organisation's resources (of one site when SiteID is set).
func (s *SkanyrNamespace) ListResources(ctx context.Context, opts *ResourcesOptions) ([]map[string]interface{}, error) {
	params := map[string]string{}
	if opts != nil {
		setStr(params, "site_id", opts.SiteID)
		setStr(params, "resource_type", opts.ResourceType)
		setInt(params, "limit", opts.Limit)
		setInt(params, "offset", opts.Offset)
	}
	return s.t.getList(ctx, "skanyr/resources", params)
}

// GetResource gets one resource.
func (s *SkanyrNamespace) GetResource(ctx context.Context, resourceID string) (map[string]interface{}, error) {
	return s.t.get(ctx, pathf("skanyr/resources/%s", resourceID), nil)
}

// SearchOptions configures optional parameters for Skanyr.Search.
type SearchOptions struct {
	SiteID   string
	MimeType string
	Limit    int
	Offset   int
}

// Search searches the organisation's extracted content.
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

// Extract extracts text and structure from stored content (by its content hash).
func (s *SkanyrNamespace) Extract(ctx context.Context, contentHash, mimeType string, metadata map[string]interface{}) (map[string]interface{}, error) {
	body := map[string]interface{}{"content_hash": contentHash, "mime_type": mimeType}
	if len(metadata) > 0 {
		body["metadata"] = metadata
	}
	return s.t.post(ctx, "skanyr/extract", body)
}

// GetExtractionText gets an extraction's plain text (the API answers text/plain).
func (s *SkanyrNamespace) GetExtractionText(ctx context.Context, contentHash string) (string, error) {
	raw, _, err := s.t.getRaw(ctx, pathf("skanyr/extractions/%s/text", contentHash), nil)
	return string(raw), err
}

// GetExtractionStructure gets an extraction's structured content (headings,
// tables, links, metadata).
func (s *SkanyrNamespace) GetExtractionStructure(ctx context.Context, contentHash string) (map[string]interface{}, error) {
	return s.t.get(ctx, pathf("skanyr/extractions/%s/structure", contentHash), nil)
}
