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

// GetDiscovery polls discovery status by ID.
func (s *SkanyrNamespace) GetDiscovery(ctx context.Context, discoveryID string) (map[string]interface{}, error) {
	return s.t.get(ctx, fmt.Sprintf("skanyr/discover/%s", discoveryID), nil)
}

// GetDiscoveryEvents gets SSE stream events for a discovery run.
func (s *SkanyrNamespace) GetDiscoveryEvents(ctx context.Context, discoveryID string) (map[string]interface{}, error) {
	return s.t.get(ctx, fmt.Sprintf("skanyr/discover/%s/events", discoveryID), nil)
}

// AnalyzeBundle analyses a JS bundle URL for embedded API patterns.
func (s *SkanyrNamespace) AnalyzeBundle(ctx context.Context, targetURL string) (map[string]interface{}, error) {
	return s.t.post(ctx, "skanyr/analyze-bundle", map[string]interface{}{"url": targetURL})
}

// DiscoverPageLive runs all detectors against a single page.
func (s *SkanyrNamespace) DiscoverPageLive(ctx context.Context, targetURL string) (map[string]interface{}, error) {
	return s.t.post(ctx, "skanyr/discover-page/live", map[string]interface{}{"url": targetURL})
}

// DetectedAPIs lists all detected APIs from a prior discovery run.
func (s *SkanyrNamespace) DetectedAPIs(ctx context.Context, pageURL string) (map[string]interface{}, error) {
	return s.t.get(ctx, "skanyr/detected-apis", map[string]string{"page_url": pageURL})
}

// Hierarchy discovers site hierarchy from a URL.
func (s *SkanyrNamespace) Hierarchy(ctx context.Context, targetURL string) (map[string]interface{}, error) {
	return s.t.post(ctx, "skanyr/hierarchy", map[string]interface{}{"url": targetURL})
}

// ExpandNode expands a hierarchy node to discover child pages.
func (s *SkanyrNamespace) ExpandNode(ctx context.Context, nodeID string) (map[string]interface{}, error) {
	return s.t.post(ctx, "skanyr/expand-node", map[string]interface{}{"node_id": nodeID})
}

// ReaderView extracts a clean reader view of a page.
func (s *SkanyrNamespace) ReaderView(ctx context.Context, targetURL string) (map[string]interface{}, error) {
	return s.t.post(ctx, "skanyr/reader-view", map[string]interface{}{"url": targetURL})
}

// Retry retries a discovery run with optional overrides.
func (s *SkanyrNamespace) Retry(ctx context.Context, discoveryID string, overrides map[string]interface{}) (map[string]interface{}, error) {
	body := map[string]interface{}{"discovery_id": discoveryID}
	for k, v := range overrides {
		body[k] = v
	}
	return s.t.post(ctx, "skanyr/retry", body)
}

// Health performs a health check for Skanyr discovery service.
func (s *SkanyrNamespace) Health(ctx context.Context) (map[string]interface{}, error) {
	return s.t.get(ctx, "skanyr/health", nil)
}

// ListSessions lists discovery sessions.
func (s *SkanyrNamespace) ListSessions(ctx context.Context) (map[string]interface{}, error) {
	return s.t.get(ctx, "skanyr/sessions", nil)
}

// SaveSession saves a discovery session.
func (s *SkanyrNamespace) SaveSession(ctx context.Context, config map[string]interface{}) (map[string]interface{}, error) {
	return s.t.post(ctx, "skanyr/sessions", config)
}

// GetSession gets a discovery session by ID.
func (s *SkanyrNamespace) GetSession(ctx context.Context, sessionID string) (map[string]interface{}, error) {
	return s.t.get(ctx, fmt.Sprintf("skanyr/sessions/%s", sessionID), nil)
}

// DeleteSession deletes a discovery session.
func (s *SkanyrNamespace) DeleteSession(ctx context.Context, sessionID string) error {
	return s.t.delete(ctx, fmt.Sprintf("skanyr/sessions/%s", sessionID))
}

// EndSession ends an active discovery session.
func (s *SkanyrNamespace) EndSession(ctx context.Context, sessionID string) (map[string]interface{}, error) {
	return s.t.post(ctx, fmt.Sprintf("skanyr/sessions/%s/end", sessionID), map[string]interface{}{})
}

// UpdateSessionJob updates the job ID associated with a session.
func (s *SkanyrNamespace) UpdateSessionJob(ctx context.Context, sessionID, jobID string) (map[string]interface{}, error) {
	return s.t.patch(ctx, fmt.Sprintf("skanyr/sessions/%s/job", sessionID), map[string]interface{}{"job_id": jobID})
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
