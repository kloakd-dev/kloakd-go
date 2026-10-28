package kloakd

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// KolektrNamespace exposes the Data Extraction module.
// Access via client.Kolektr.
type KolektrNamespace struct {
	t *transport
}

// ExtractOptions configures optional parameters for Kolektr.Extract.
type ExtractOptions struct {
	// SchemaSpec describes the fields to extract (sent as schema_spec):
	// either the API's own spec, {"fields": [{"name", "selector", ...}],
	// "pagination", "max_pages", ...}, or the shorthand
	// {field: "css:selector"} ("css:" is optional), which is converted.
	SchemaSpec map[string]interface{}
	// Fields is the explicit field list; it replaces SchemaSpec's fields.
	Fields            []SchemaField
	Level             string
	FetchArtifactID   string
	SessionArtifactID string
	ApiMapArtifactID  string
	Options           map[string]interface{}
}

// PageOptions is the 0.2 name of ExtractOptions.
//
// Deprecated: use ExtractOptions.
type PageOptions = ExtractOptions

// Extract extracts structured data from a URL with the full Kolektr
// pipeline (POST kolektr/extract). A request the API hands off as a job
// (202, after about 85 s) is followed to its result; see Config.PollInterval.
func (k *KolektrNamespace) Extract(ctx context.Context, targetURL string, opts *ExtractOptions) (*ExtractionResult, error) {
	body := map[string]interface{}{"url": targetURL}
	if opts != nil {
		if spec := buildSchemaSpec(opts.SchemaSpec, opts.Fields); spec != nil {
			body["schema_spec"] = spec
		}
		if opts.Level != "" {
			body["level"] = opts.Level
		}
		if opts.FetchArtifactID != "" {
			body["fetch_artifact_id"] = opts.FetchArtifactID
		}
		if opts.SessionArtifactID != "" {
			body["session_artifact_id"] = opts.SessionArtifactID
		}
		if opts.ApiMapArtifactID != "" {
			body["api_map_artifact_id"] = opts.ApiMapArtifactID
		}
		if len(opts.Options) > 0 {
			body["options"] = opts.Options
		}
	}

	raw, err := k.t.postFollowing(ctx, "kolektr/extract", body, nil)
	if err != nil {
		return nil, err
	}
	return parseExtractionResult(raw), nil
}

// Page is the 0.2 name of Extract.
//
// Deprecated: use Extract.
func (k *KolektrNamespace) Page(ctx context.Context, targetURL string, opts *ExtractOptions) (*ExtractionResult, error) {
	return k.Extract(ctx, targetURL, opts)
}

// ExtractHTMLOptions configures optional parameters for Kolektr.ExtractHTML.
type ExtractHTMLOptions struct {
	// SchemaSpec and Fields are as in ExtractOptions.
	SchemaSpec map[string]interface{}
	Fields     []SchemaField
}

// ExtractHTML extracts structured data from raw HTML without making a network request.
func (k *KolektrNamespace) ExtractHTML(ctx context.Context, html, targetURL string, opts *ExtractHTMLOptions) (*ExtractionResult, error) {
	body := map[string]interface{}{
		"html": html,
		"url":  targetURL,
	}
	if opts != nil {
		if spec := buildSchemaSpec(opts.SchemaSpec, opts.Fields); spec != nil {
			body["schema_spec"] = spec
		}
	}

	raw, err := k.t.post(ctx, "kolektr/extract/html", body)
	if err != nil {
		return nil, err
	}
	return parseExtractionResult(raw), nil
}

// APIDataOptions configures optional parameters for the api-data calls.
type APIDataOptions struct {
	// EndpointHash selects the endpoint by its hash (endpoint_hash).
	EndpointHash string
}

// GetAPIData gets all extracted data for a discovered API endpoint.
// apiEndpoint is the endpoint's URL; it is sent escaped as one path segment.
func (k *KolektrNamespace) GetAPIData(ctx context.Context, apiEndpoint string, opts *APIDataOptions) (map[string]interface{}, error) {
	params := map[string]string{}
	if opts != nil && opts.EndpointHash != "" {
		params["endpoint_hash"] = opts.EndpointHash
	}
	return k.t.get(ctx, pathf("kolektr/api-data/%s", apiEndpoint), params)
}

// GetAPIDataPaginated gets API data with pagination.
func (k *KolektrNamespace) GetAPIDataPaginated(ctx context.Context, apiEndpoint string, offset, limit int, opts *APIDataOptions) (map[string]interface{}, error) {
	params := map[string]string{
		"offset": strconv.Itoa(offset),
		"limit":  strconv.Itoa(limit),
	}
	if opts != nil && opts.EndpointHash != "" {
		params["endpoint_hash"] = opts.EndpointHash
	}
	return k.t.get(ctx, pathf("kolektr/api-data/%s/paginated", apiEndpoint), params)
}

// ExtractAllAPIDataOptions configures optional parameters for ExtractAllAPIData.
type ExtractAllAPIDataOptions struct {
	EndpointHash string
	// MaxRecords bounds the records extracted; 0 means no bound.
	MaxRecords int
}

// ExtractAllAPIData starts a job extracting all data from a discovered API endpoint.
func (k *KolektrNamespace) ExtractAllAPIData(ctx context.Context, apiEndpoint string, opts *ExtractAllAPIDataOptions) (map[string]interface{}, error) {
	params := map[string]string{}
	if opts != nil {
		if opts.EndpointHash != "" {
			params["endpoint_hash"] = opts.EndpointHash
		}
		if opts.MaxRecords > 0 {
			params["max_records"] = strconv.Itoa(opts.MaxRecords)
		}
	}
	_, raw, err := k.t.do(ctx, http.MethodPost, pathf("kolektr/api-data/%s/extract-all", apiEndpoint), map[string]interface{}{}, params)
	return raw, err
}

// ListContentOptions configures Kolektr.ListContent (zero values: the API's defaults).
type ListContentOptions struct {
	Page     int
	PageSize int
	Search   string
}

// ListContent lists content items.
func (k *KolektrNamespace) ListContent(ctx context.Context, opts *ListContentOptions) (map[string]interface{}, error) {
	params := map[string]string{}
	if opts != nil {
		setInt(params, "page", opts.Page)
		setInt(params, "page_size", opts.PageSize)
		if opts.Search != "" {
			params["search"] = opts.Search
		}
	}
	return k.t.get(ctx, "kolektr/content", params)
}

// GetContent gets a content item by ID.
func (k *KolektrNamespace) GetContent(ctx context.Context, itemID string) (map[string]interface{}, error) {
	return k.t.get(ctx, pathf("kolektr/content/%s", itemID), nil)
}

// DeleteContent deletes a content item.
func (k *KolektrNamespace) DeleteContent(ctx context.Context, itemID string) error {
	return k.t.delete(ctx, pathf("kolektr/content/%s", itemID))
}

// ListJobsOptions configures Kolektr.ListJobs (zero values: the API's defaults).
type ListJobsOptions struct {
	Page         int
	Limit        int
	StatusFilter string
}

// ListJobs lists extraction jobs.
func (k *KolektrNamespace) ListJobs(ctx context.Context, opts *ListJobsOptions) (map[string]interface{}, error) {
	params := map[string]string{}
	if opts != nil {
		setInt(params, "page", opts.Page)
		setInt(params, "limit", opts.Limit)
		if opts.StatusFilter != "" {
			params["status_filter"] = opts.StatusFilter
		}
	}
	return k.t.get(ctx, "kolektr/jobs", params)
}

// GetJob gets an extraction job by ID.
func (k *KolektrNamespace) GetJob(ctx context.Context, jobID string) (map[string]interface{}, error) {
	return k.t.get(ctx, pathf("kolektr/jobs/%s", jobID), nil)
}

// GetJobStatus gets extraction job status.
func (k *KolektrNamespace) GetJobStatus(ctx context.Context, jobID string) (map[string]interface{}, error) {
	return k.t.get(ctx, pathf("kolektr/extraction-jobs/%s/status", jobID), nil)
}

// ListScrapersOptions configures Kolektr.ListScrapers (zero values: the API's defaults).
type ListScrapersOptions struct {
	Page     int
	PageSize int
	// IsTemplate filters on templates (true) or non-templates (false); nil for all.
	IsTemplate *bool
}

// ListScrapers lists scraper configurations.
func (k *KolektrNamespace) ListScrapers(ctx context.Context, opts *ListScrapersOptions) (map[string]interface{}, error) {
	params := map[string]string{}
	if opts != nil {
		setInt(params, "page", opts.Page)
		setInt(params, "page_size", opts.PageSize)
		if opts.IsTemplate != nil {
			params["is_template"] = strconv.FormatBool(*opts.IsTemplate)
		}
	}
	return k.t.get(ctx, "kolektr/scrapers", params)
}

// CreateScraper creates a scraper configuration.
func (k *KolektrNamespace) CreateScraper(ctx context.Context, config map[string]interface{}) (map[string]interface{}, error) {
	return k.t.post(ctx, "kolektr/scrapers", config)
}

// GetScraper gets a scraper configuration by ID.
func (k *KolektrNamespace) GetScraper(ctx context.Context, scraperID string) (map[string]interface{}, error) {
	return k.t.get(ctx, pathf("kolektr/scrapers/%s", scraperID), nil)
}

// UpdateScraper updates a scraper configuration.
func (k *KolektrNamespace) UpdateScraper(ctx context.Context, scraperID string, updates map[string]interface{}) (map[string]interface{}, error) {
	return k.t.patch(ctx, pathf("kolektr/scrapers/%s", scraperID), updates)
}

// DeleteScraper deletes a scraper configuration.
func (k *KolektrNamespace) DeleteScraper(ctx context.Context, scraperID string) error {
	return k.t.delete(ctx, pathf("kolektr/scrapers/%s", scraperID))
}

// ─── parsers ─────────────────────────────────────────────────────────────────

func parseExtractionResult(raw map[string]interface{}) *ExtractionResult {
	return &ExtractionResult{
		Success:      boolField(raw, "success"),
		URL:          strField(raw, "url"),
		Method:       strField(raw, "method"),
		Records:      sliceMapField(raw, "records"),
		TotalRecords: intField(raw, "total_records"),
		PagesScraped: intField(raw, "pages_scraped"),
		ArtifactID:   nestedArtifactID(raw),
		JobID:        optStrField(raw, "job_id"),
		Error:        optStrField(raw, "error"),
		AuthRequired: boolField(raw, "auth_required"),
		DiscoveryID:  optStrField(raw, "discovery_id"),
	}
}

// buildSchemaSpec is the API's schema_spec (modules/kolektr/router.py
// SchemaSpec): a spec with a "fields" list is sent as given, a shorthand
// {field: "css:selector"} is converted to {"fields": [{name, selector}]}
// (field names sorted), and fields, when given, replace the spec's fields.
// It returns nil when there is nothing to send.
func buildSchemaSpec(schema map[string]interface{}, fields []SchemaField) map[string]interface{} {
	var spec map[string]interface{}
	switch {
	case len(schema) == 0:
	case schema["fields"] != nil:
		spec = make(map[string]interface{}, len(schema))
		for k, v := range schema {
			spec[k] = v
		}
		if typed, ok := schema["fields"].([]SchemaField); ok {
			spec["fields"] = schemaFieldMaps(typed)
		}
	default:
		names := make([]string, 0, len(schema))
		for name := range schema {
			names = append(names, name)
		}
		sort.Strings(names)
		list := make([]interface{}, 0, len(names))
		for _, name := range names {
			switch v := schema[name].(type) {
			case string:
				list = append(list, map[string]interface{}{"name": name, "selector": strings.TrimPrefix(v, "css:")})
			case map[string]interface{}:
				f := map[string]interface{}{"name": name}
				for k, fv := range v {
					f[k] = fv
				}
				list = append(list, f)
			default:
				list = append(list, map[string]interface{}{"name": name, "selector": fmt.Sprintf("%v", v)})
			}
		}
		spec = map[string]interface{}{"fields": list}
	}
	if len(fields) > 0 {
		if spec == nil {
			spec = map[string]interface{}{}
		}
		spec["fields"] = schemaFieldMaps(fields)
	}
	return spec
}

// shorthandSchema converts the orchestrator's {field: "css:selector"}.
func shorthandSchema(schema map[string]string) map[string]interface{} {
	if len(schema) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(schema))
	for k, v := range schema {
		out[k] = v
	}
	return out
}

func schemaFieldMaps(fields []SchemaField) []interface{} {
	list := make([]interface{}, 0, len(fields))
	for _, f := range fields {
		m := map[string]interface{}{"name": f.Name}
		if f.Selector != "" {
			m["selector"] = strings.TrimPrefix(f.Selector, "css:")
		}
		if f.Attribute != "" {
			m["attribute"] = f.Attribute
		}
		if f.Type != "" {
			m["type"] = f.Type
		}
		list = append(list, m)
	}
	return list
}

// setInt sets params[key] when n is positive.
func setInt(params map[string]string, key string, n int) {
	if n > 0 {
		params[key] = strconv.Itoa(n)
	}
}
