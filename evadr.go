package kloakd

import (
	"context"
	"fmt"
	"strconv"
)

// EvadrNamespace exposes the Anti-Bot Intelligence module.
// Access via client.Evadr.
type EvadrNamespace struct {
	t *transport
}

// FetchOptions configures optional parameters for Evadr.Fetch.
type FetchOptions struct {
	ForceBrowser      bool
	UseProxy          bool
	SessionArtifactID string
}

// Fetch performs an anti-bot fetch of the given URL.
func (e *EvadrNamespace) Fetch(ctx context.Context, targetURL string, opts *FetchOptions) (*FetchResult, error) {
	body := map[string]interface{}{"url": targetURL}
	if opts != nil {
		if opts.ForceBrowser {
			body["force_browser"] = true
		}
		if opts.UseProxy {
			body["use_proxy"] = true
		}
		if opts.SessionArtifactID != "" {
			body["session_artifact_id"] = opts.SessionArtifactID
		}
	}

	raw, err := e.t.post(ctx, "evadr/fetch", body)
	if err != nil {
		return nil, err
	}
	return parseFetchResult(raw), nil
}

// FetchAsyncOptions configures optional parameters for Evadr.FetchAsync.
type FetchAsyncOptions struct {
	ForceBrowser bool
	UseProxy     bool
	WebhookURL   string
}

// FetchAsync enqueues an anti-bot fetch job and returns the job ID.
func (e *EvadrNamespace) FetchAsync(ctx context.Context, targetURL string, opts *FetchAsyncOptions) (string, error) {
	body := map[string]interface{}{"url": targetURL}
	if opts != nil {
		if opts.ForceBrowser {
			body["force_browser"] = true
		}
		if opts.UseProxy {
			body["use_proxy"] = true
		}
		if opts.WebhookURL != "" {
			body["webhook_url"] = opts.WebhookURL
		}
	}

	raw, err := e.t.post(ctx, "evadr/fetch/async", body)
	if err != nil {
		return "", err
	}
	jobID, _ := raw["job_id"].(string)
	return jobID, nil
}

// FetchStreamOptions configures optional parameters for Evadr.FetchStream.
type FetchStreamOptions struct {
	ForceBrowser bool
}

// FetchStream opens a server-sent events stream for an anti-bot fetch operation.
// Returns a read-only channel of FetchEvent values. The channel is closed
// when the stream ends or ctx is cancelled.
func (e *EvadrNamespace) FetchStream(ctx context.Context, targetURL string, opts *FetchStreamOptions) (<-chan FetchEvent, error) {
	body := map[string]interface{}{"url": targetURL}
	if opts != nil && opts.ForceBrowser {
		body["force_browser"] = true
	}

	raw, err := e.t.stream(ctx, "evadr/fetch/stream", body)
	if err != nil {
		return nil, err
	}

	ch := make(chan FetchEvent, 16)
	go func() {
		defer close(ch)
		for payload := range raw {
			ch <- parseFetchEvent(payload)
		}
	}()
	return ch, nil
}

// AnalyzeOptions configures optional parameters for Evadr.Analyze.
type AnalyzeOptions struct {
	StatusCode  int
	Headers     map[string]string
	BodySnippet string
}

// Analyze inspects the anti-bot defenses of the given URL.
func (e *EvadrNamespace) Analyze(ctx context.Context, targetURL string, opts *AnalyzeOptions) (*AnalyzeResult, error) {
	body := map[string]interface{}{"url": targetURL}
	if opts != nil {
		if opts.StatusCode != 0 {
			body["status_code"] = opts.StatusCode
		}
		if len(opts.Headers) > 0 {
			body["headers"] = opts.Headers
		}
		if opts.BodySnippet != "" {
			body["body_snippet"] = opts.BodySnippet
		}
	}

	raw, err := e.t.post(ctx, "evadr/analyze", body)
	if err != nil {
		return nil, err
	}
	return parseAnalyzeResult(raw), nil
}

// StoreProxy registers a named proxy in the organization's proxy pool.
func (e *EvadrNamespace) StoreProxy(ctx context.Context, name, proxyURL string) error {
	_, err := e.t.post(ctx, "evadr/proxies", map[string]interface{}{
		"name":      name,
		"proxy_url": proxyURL,
	})
	return err
}

// ─── parsers ─────────────────────────────────────────────────────────────────

func parseFetchResult(raw map[string]interface{}) *FetchResult {
	r := &FetchResult{
		Success:         boolField(raw, "success"),
		URL:             strField(raw, "url"),
		StatusCode:      intField(raw, "status_code"),
		TierUsed:        intField(raw, "tier_used"),
		AntiBotBypassed: boolField(raw, "anti_bot_bypassed"),
		HTML:            optStrField(raw, "html"),
		VendorDetected:  optStrField(raw, "vendor_detected"),
		ArtifactID:      optStrField(raw, "artifact_id"),
		Error:           optStrField(raw, "error"),
	}
	return r
}

func parseFetchEvent(raw map[string]interface{}) FetchEvent {
	ev := FetchEvent{
		Type:     strField(raw, "type"),
		Vendor:   optStrField(raw, "vendor"),
		Metadata: mapField(raw, "metadata"),
	}
	if v, ok := raw["tier"]; ok && v != nil {
		switch n := v.(type) {
		case float64:
			tier := int(n)
			ev.Tier = &tier
		case string:
			if i, err := strconv.Atoi(n); err == nil {
				ev.Tier = &i
			}
		}
	}
	return ev
}

func parseAnalyzeResult(raw map[string]interface{}) *AnalyzeResult {
	actions := []string{}
	if v, ok := raw["recommended_actions"]; ok {
		if arr, ok := v.([]interface{}); ok {
			for _, a := range arr {
				if s, ok := a.(string); ok {
					actions = append(actions, s)
				}
			}
		}
	}
	return &AnalyzeResult{
		Blocked:            boolField(raw, "blocked"),
		Vendor:             optStrField(raw, "vendor"),
		Confidence:         float64Field(raw, "confidence"),
		RecommendedActions: actions,
	}
}

// ─── field helpers (shared across all namespace files via same package) ───────

func strField(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func optStrField(m map[string]interface{}, key string) *string {
	if v, ok := m[key]; ok && v != nil {
		if s, ok := v.(string); ok {
			return &s
		}
	}
	return nil
}

func boolField(m map[string]interface{}, key string) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func intField(m map[string]interface{}, key string) int {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return 0
}

func optIntField(m map[string]interface{}, key string) *int {
	if v, ok := m[key]; ok && v != nil {
		switch n := v.(type) {
		case float64:
			i := int(n)
			return &i
		case int:
			return &n
		}
	}
	return nil
}

func float64Field(m map[string]interface{}, key string) float64 {
	if v, ok := m[key]; ok {
		if f, ok := v.(float64); ok {
			return f
		}
	}
	return 0
}

func mapField(m map[string]interface{}, key string) map[string]interface{} {
	if v, ok := m[key]; ok {
		if mm, ok := v.(map[string]interface{}); ok {
			return mm
		}
	}
	return map[string]interface{}{}
}

func sliceMapField(m map[string]interface{}, key string) []map[string]interface{} {
	if v, ok := m[key]; ok {
		if arr, ok := v.([]interface{}); ok {
			result := make([]map[string]interface{}, 0, len(arr))
			for _, item := range arr {
				if mm, ok := item.(map[string]interface{}); ok {
					result = append(result, mm)
				}
			}
			return result
		}
	}
	return []map[string]interface{}{}
}

func strSliceField(m map[string]interface{}, key string) []string {
	if v, ok := m[key]; ok {
		if arr, ok := v.([]interface{}); ok {
			result := make([]string, 0, len(arr))
			for _, item := range arr {
				if s, ok := item.(string); ok {
					result = append(result, s)
				}
			}
			return result
		}
	}
	return []string{}
}

// unused — suppress compiler warning
var _ = fmt.Sprintf
