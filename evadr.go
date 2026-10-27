package kloakd

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// Defaults for following a fetch that the API hands off as a job.
const (
	DefaultFetchPollInterval = 3 * time.Second
	DefaultFetchPollTimeout  = 15 * time.Minute
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
	// PollInterval and PollTimeout govern following a fetch the API hands
	// off as a job (HTTP 202, after about 85 s). Defaults 3 s and 15 min;
	// ctx cancellation also stops the wait.
	PollInterval time.Duration
	PollTimeout  time.Duration
}

// HandedOff describes a 202 from the synchronous fetch: still running,
// continuing as the job JobID.
type HandedOff struct {
	JobID         string
	Reason        string
	WaitedSeconds float64
	PollURL       string
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

	status, raw, err := e.t.do(ctx, http.MethodPost, "evadr/fetch", body, nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusAccepted {
		if jobID, _ := raw["job_id"].(string); jobID != "" {
			return e.followJob(ctx, jobID, opts)
		}
	}
	return parseFetchResult(raw), nil
}

// followJob polls a handed-off fetch until it completes or fails.
func (e *EvadrNamespace) followJob(ctx context.Context, jobID string, opts *FetchOptions) (*FetchResult, error) {
	interval, limit := DefaultFetchPollInterval, DefaultFetchPollTimeout
	if opts != nil {
		if opts.PollInterval > 0 {
			interval = opts.PollInterval
		}
		if opts.PollTimeout > 0 {
			limit = opts.PollTimeout
		}
	}
	deadline := time.Now().Add(limit)
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
		job, err := e.t.get(ctx, fmt.Sprintf("evadr/jobs/%s", jobID), nil)
		if err != nil {
			return nil, err
		}
		switch strField(job, "status") {
		case "completed":
			return parseFetchResult(mapField(job, "result")), nil
		case "failed":
			msg := strField(job, "error")
			if msg == "" {
				msg = "fetch job failed"
			}
			return nil, &ApiError{KloakdError: KloakdError{StatusCode: 200, Message: msg, Code: "job_failed"}}
		}
		if time.Now().After(deadline) {
			return nil, &ApiError{KloakdError: KloakdError{
				Message: fmt.Sprintf("fetch job %s still running after %s", jobID, limit), Code: "job_timeout",
			}}
		}
	}
}

// FetchAsyncOptions configures optional parameters for Evadr.FetchAsync.
type FetchAsyncOptions struct {
	ForceBrowser      bool
	UseProxy          bool
	SessionArtifactID string
	WebhookURL        string
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
		if opts.SessionArtifactID != "" {
			body["session_artifact_id"] = opts.SessionArtifactID
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
	ForceBrowser      bool
	UseProxy          bool
	SessionArtifactID string
}

// FetchStream starts an asynchronous fetch and streams its progress events
// (GET evadr/jobs/{job_id}/events). The channel closes when the job ends or
// ctx is cancelled. There is no POST stream route; this is the live route.
func (e *EvadrNamespace) FetchStream(ctx context.Context, targetURL string, opts *FetchStreamOptions) (<-chan FetchEvent, error) {
	asyncOpts := &FetchAsyncOptions{}
	if opts != nil {
		asyncOpts.ForceBrowser = opts.ForceBrowser
		asyncOpts.UseProxy = opts.UseProxy
		asyncOpts.SessionArtifactID = opts.SessionArtifactID
	}
	jobID, err := e.FetchAsync(ctx, targetURL, asyncOpts)
	if err != nil {
		return nil, err
	}
	raw, err := e.JobEvents(ctx, jobID)
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

// Scan fetches a URL once (plain HTTP) and reports its anti-bot
// infrastructure, vendors and challenges.
func (e *EvadrNamespace) Scan(ctx context.Context, targetURL string) (map[string]interface{}, error) {
	return e.t.post(ctx, "evadr/scan", map[string]interface{}{"url": targetURL})
}

// Profile analyses a URL and returns its canonical site profile.
func (e *EvadrNamespace) Profile(ctx context.Context, targetURL string) (map[string]interface{}, error) {
	return e.t.post(ctx, "evadr/profile", map[string]interface{}{"url": targetURL})
}

// GetJob polls an async fetch job by ID.
func (e *EvadrNamespace) GetJob(ctx context.Context, jobID string) (map[string]interface{}, error) {
	return e.t.get(ctx, fmt.Sprintf("evadr/jobs/%s", jobID), nil)
}

// JobEvents streams an async fetch job's progress events (SSE). The
// channel closes when the job ends or ctx is cancelled.
func (e *EvadrNamespace) JobEvents(ctx context.Context, jobID string) (<-chan map[string]interface{}, error) {
	return e.t.getStream(ctx, fmt.Sprintf("evadr/jobs/%s/events", jobID), nil)
}

// ListVendors lists all known anti-bot detection vendors.
func (e *EvadrNamespace) ListVendors(ctx context.Context) (map[string]interface{}, error) {
	return e.t.get(ctx, "evadr/vendors", nil)
}

// ListProfiles lists available evasion profiles.
func (e *EvadrNamespace) ListProfiles(ctx context.Context) (map[string]interface{}, error) {
	return e.t.get(ctx, "evadr/profiles", nil)
}

// ListProxies lists stored proxy configurations (names only, no secrets).
func (e *EvadrNamespace) ListProxies(ctx context.Context) (map[string]interface{}, error) {
	return e.t.get(ctx, "evadr/proxies", nil)
}

// StoreProxy registers a named proxy in the organization's proxy pool.
func (e *EvadrNamespace) StoreProxy(ctx context.Context, name, proxyURL string) error {
	_, err := e.t.post(ctx, "evadr/proxies", map[string]interface{}{
		"name":      name,
		"proxy_url": proxyURL,
	})
	return err
}

// DeleteProxy deletes a stored proxy configuration by name.
func (e *EvadrNamespace) DeleteProxy(ctx context.Context, name string) error {
	return e.t.delete(ctx, fmt.Sprintf("evadr/proxies/%s", name))
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
		ArtifactID:      nestedArtifactID(raw),
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

// nestedArtifactID extracts artifact_id from the nested "artifact" object
// returned by the API, falling back to a top-level "artifact_id" field.
func nestedArtifactID(raw map[string]interface{}) *string {
	if artifact, ok := raw["artifact"]; ok {
		if m, ok := artifact.(map[string]interface{}); ok {
			if id, ok := m["artifact_id"]; ok && id != nil {
				if s, ok := id.(string); ok {
					return &s
				}
			}
		}
	}
	return optStrField(raw, "artifact_id")
}

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
