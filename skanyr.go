package kloakd

import (
	"context"
	"fmt"
)

// SkanyrNamespace exposes the API Discovery module.
// Access via client.Skanyr.
type SkanyrNamespace struct {
	t *transport
}

// DiscoverOptions configures optional parameters for Skanyr.Discover.
type DiscoverOptions struct {
	SiteHierarchyArtifactID string
	MaxRequests             int
	SessionArtifactID       string
	Limit                   int
	Offset                  int
}

// Discover discovers API endpoints on the given URL.
func (s *SkanyrNamespace) Discover(ctx context.Context, targetURL string, opts *DiscoverOptions) (*DiscoverResult, error) {
	body := map[string]interface{}{"url": targetURL}
	if opts != nil {
		if opts.SiteHierarchyArtifactID != "" {
			body["site_hierarchy_artifact_id"] = opts.SiteHierarchyArtifactID
		}
		if opts.MaxRequests > 0 {
			body["max_requests"] = opts.MaxRequests
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

	raw, err := s.t.post(ctx, "skanyr/discover", body)
	if err != nil {
		return nil, err
	}
	return parseDiscoverResult(raw), nil
}

// DiscoverAll auto-paginates until all endpoints are retrieved.
func (s *SkanyrNamespace) DiscoverAll(ctx context.Context, targetURL string, opts *DiscoverOptions) ([]ApiEndpoint, error) {
	limit := 100
	offset := 0
	var all []ApiEndpoint

	for {
		o := &DiscoverOptions{}
		if opts != nil {
			*o = *opts
		}
		o.Limit = limit
		o.Offset = offset

		result, err := s.Discover(ctx, targetURL, o)
		if err != nil {
			return nil, err
		}
		all = append(all, result.Endpoints...)
		if !result.HasMore {
			break
		}
		offset += len(result.Endpoints)
	}
	return all, nil
}

// DiscoverStreamOptions configures optional parameters for Skanyr.DiscoverStream.
type DiscoverStreamOptions struct {
	SiteHierarchyArtifactID string
	MaxRequests             int
}

// DiscoverStream opens a server-sent events stream for the API discovery operation.
// Returns a read-only channel of DiscoverEvent values.
func (s *SkanyrNamespace) DiscoverStream(ctx context.Context, targetURL string, opts *DiscoverStreamOptions) (<-chan DiscoverEvent, error) {
	body := map[string]interface{}{"url": targetURL}
	if opts != nil {
		if opts.SiteHierarchyArtifactID != "" {
			body["site_hierarchy_artifact_id"] = opts.SiteHierarchyArtifactID
		}
		if opts.MaxRequests > 0 {
			body["max_requests"] = opts.MaxRequests
		}
	}

	raw, err := s.t.stream(ctx, "skanyr/discover/stream", body)
	if err != nil {
		return nil, err
	}

	ch := make(chan DiscoverEvent, 16)
	go func() {
		defer close(ch)
		for payload := range raw {
			ch <- parseDiscoverEvent(payload)
		}
	}()
	return ch, nil
}

// GetApiMap retrieves a stored API map artifact by ID.
func (s *SkanyrNamespace) GetApiMap(ctx context.Context, artifactID string) (map[string]interface{}, error) {
	return s.t.get(ctx, fmt.Sprintf("skanyr/api-map/%s", artifactID), nil)
}

// GetJob retrieves the status of a discovery job by ID.
func (s *SkanyrNamespace) GetJob(ctx context.Context, jobID string) (map[string]interface{}, error) {
	return s.t.get(ctx, fmt.Sprintf("skanyr/jobs/%s", jobID), nil)
}

// ─── parsers ─────────────────────────────────────────────────────────────────

func parseDiscoverResult(raw map[string]interface{}) *DiscoverResult {
	endpoints := []ApiEndpoint{}
	if v, ok := raw["endpoints"]; ok {
		if arr, ok := v.([]interface{}); ok {
			for _, item := range arr {
				if m, ok := item.(map[string]interface{}); ok {
					endpoints = append(endpoints, parseApiEndpoint(m))
				}
			}
		}
	}
	return &DiscoverResult{
		Success:        boolField(raw, "success"),
		DiscoveryID:    strField(raw, "discovery_id"),
		URL:            strField(raw, "url"),
		TotalEndpoints: intField(raw, "total_endpoints"),
		Endpoints:      endpoints,
		HasMore:        boolField(raw, "has_more"),
		Total:          intField(raw, "total"),
		ArtifactID:     optStrField(raw, "artifact_id"),
		Error:          optStrField(raw, "error"),
	}
}

func parseApiEndpoint(raw map[string]interface{}) ApiEndpoint {
	return ApiEndpoint{
		URL:        strField(raw, "url"),
		Method:     strField(raw, "method"),
		ApiType:    strField(raw, "api_type"),
		Confidence: float64Field(raw, "confidence"),
		Parameters: mapField(raw, "parameters"),
	}
}

func parseDiscoverEvent(raw map[string]interface{}) DiscoverEvent {
	return DiscoverEvent{
		Type:        strField(raw, "type"),
		EndpointURL: optStrField(raw, "endpoint_url"),
		ApiType:     optStrField(raw, "api_type"),
		Metadata:    mapField(raw, "metadata"),
	}
}
