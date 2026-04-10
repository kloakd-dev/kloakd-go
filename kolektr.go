package kloakd

import "context"

// KolektrNamespace exposes the Data Extraction module.
// Access via client.Kolektr.
type KolektrNamespace struct {
	t *transport
}

// PageOptions configures optional parameters for Kolektr.Page.
type PageOptions struct {
	Schema                  map[string]interface{}
	FetchArtifactID         string
	SessionArtifactID       string
	ApiMapArtifactID        string
	Limit                   int
	Offset                  int
}

// Page extracts structured data from a URL using the full Kolektr pipeline.
func (k *KolektrNamespace) Page(ctx context.Context, targetURL string, opts *PageOptions) (*ExtractionResult, error) {
	body := map[string]interface{}{"url": targetURL}
	if opts != nil {
		if len(opts.Schema) > 0 {
			body["schema"] = opts.Schema
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
		if opts.Limit > 0 {
			body["limit"] = opts.Limit
		}
		if opts.Offset > 0 {
			body["offset"] = opts.Offset
		}
	}

	raw, err := k.t.post(ctx, "kolektr/page", body)
	if err != nil {
		return nil, err
	}
	return parseExtractionResult(raw), nil
}

// PageAll auto-paginates until all records are retrieved.
func (k *KolektrNamespace) PageAll(ctx context.Context, targetURL string, opts *PageOptions) ([]map[string]interface{}, error) {
	limit := 100
	offset := 0
	var all []map[string]interface{}

	for {
		o := &PageOptions{}
		if opts != nil {
			*o = *opts
		}
		o.Limit = limit
		o.Offset = offset

		result, err := k.Page(ctx, targetURL, o)
		if err != nil {
			return nil, err
		}
		all = append(all, result.Records...)
		if !result.HasMore {
			break
		}
		offset += len(result.Records)
	}
	return all, nil
}

// ExtractHTMLOptions configures optional parameters for Kolektr.ExtractHTML.
type ExtractHTMLOptions struct {
	Schema map[string]interface{}
}

// ExtractHTML extracts structured data from raw HTML without making a network request.
func (k *KolektrNamespace) ExtractHTML(ctx context.Context, html, targetURL string, opts *ExtractHTMLOptions) (*ExtractionResult, error) {
	body := map[string]interface{}{
		"html": html,
		"url":  targetURL,
	}
	if opts != nil && len(opts.Schema) > 0 {
		body["schema"] = opts.Schema
	}

	raw, err := k.t.post(ctx, "kolektr/extract-html", body)
	if err != nil {
		return nil, err
	}
	return parseExtractionResult(raw), nil
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
		HasMore:      boolField(raw, "has_more"),
		Total:        intField(raw, "total"),
		ArtifactID:   optStrField(raw, "artifact_id"),
		JobID:        optStrField(raw, "job_id"),
		Error:        optStrField(raw, "error"),
	}
}
