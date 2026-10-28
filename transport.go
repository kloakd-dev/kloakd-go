package kloakd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	sdkVersion     = "0.5.0"
	userAgent      = "kloakd-sdk-go/" + sdkVersion
	defaultBaseURL = "https://api.kloakd.dev"
	// defaultTimeout is above the API's 85 s synchronous wait
	// (kernel/sync_handoff.py), so a request the API hands off as a job
	// receives its 202 instead of timing out first.
	defaultTimeout = 100 * time.Second
	defaultRetries = 3
	// maxBackoff caps the exponential backoff between retries.
	maxBackoff = 60 * time.Second
	// maxSSEToken is the longest SSE line the stream reader accepts
	// (a Parlyr results event carries every extracted item).
	maxSSEToken = 16 << 20
)

// transport is the shared HTTP transport for all namespace methods.
// It is not part of the public API.
type transport struct {
	apiKey         string
	organizationID string
	baseURL        string
	maxRetries     int
	httpClient     *http.Client
	// streamClient is httpClient without its Timeout: http.Client.Timeout
	// covers reading the whole body, which would cut a long SSE stream.
	// Streams end with ctx.
	streamClient *http.Client
	poll         PollOptions
	// retryBase is the first exponential backoff step (1 s; tests shorten it).
	retryBase time.Duration
}

func newTransport(cfg Config) *transport {
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: cfg.effectiveTimeout()}
	}
	streamClient := *client
	streamClient.Timeout = 0
	return &transport{
		apiKey:         cfg.APIKey,
		organizationID: cfg.OrganizationID,
		baseURL:        strings.TrimRight(cfg.effectiveBaseURL(), "/"),
		maxRetries:     cfg.effectiveMaxRetries(),
		httpClient:     client,
		streamClient:   &streamClient,
		poll:           PollOptions{Interval: cfg.PollInterval, Timeout: cfg.PollTimeout},
		retryBase:      time.Second,
	}
}

func (t *transport) setHeaders(req *http.Request, hasBody bool) {
	req.Header.Set("Authorization", "Bearer "+t.apiKey)
	req.Header.Set("X-Kloakd-Organization", t.organizationID)
	req.Header.Set("X-Kloakd-SDK", "go/"+sdkVersion)
	req.Header.Set("User-Agent", userAgent)
	if hasBody {
		req.Header.Set("Content-Type", "application/json")
	}
}

func (t *transport) orgPrefix() string {
	return "/api/v1/organizations/" + url.PathEscape(t.organizationID)
}

// pathf formats a route path, escaping each argument as one path segment
// (url.PathEscape: "/", "?", "#", "%" and spaces are escaped), so an ID,
// name, domain or URL cannot change the route.
func pathf(format string, args ...string) string {
	escaped := make([]interface{}, len(args))
	for i, a := range args {
		escaped[i] = url.PathEscape(a)
	}
	return fmt.Sprintf(format, escaped...)
}

func (t *transport) buildURL(path string, params map[string]string) string {
	full := t.baseURL + t.orgPrefix() + "/" + strings.TrimLeft(path, "/")
	if len(params) == 0 {
		return full
	}
	q := url.Values{}
	for k, v := range params {
		q.Set(k, v)
	}
	return full + "?" + q.Encode()
}

// request executes an HTTP request with retry/backoff and returns the parsed JSON body.
func (t *transport) request(
	ctx context.Context,
	method, path string,
	body map[string]interface{},
	params map[string]string,
) (map[string]interface{}, error) {
	_, parsed, err := t.do(ctx, method, path, body, params)
	return parsed, err
}

// do is request that also returns the HTTP status (a 202 from a
// synchronous route means the work continues as a job). A top-level JSON
// array is returned as {"items": [...]}.
func (t *transport) do(
	ctx context.Context,
	method, path string,
	body map[string]interface{},
	params map[string]string,
) (int, map[string]interface{}, error) {
	return t.doURL(ctx, method, t.buildURL(path, params), body)
}

// doURL is do for a full URL (a hand-off's poll_url is a path from the
// API's root, not from the organisation prefix).
func (t *transport) doURL(
	ctx context.Context,
	method, reqURL string,
	body map[string]interface{},
) (int, map[string]interface{}, error) {
	status, _, raw, err := t.send(ctx, method, reqURL, body)
	if err != nil {
		return status, nil, err
	}
	return status, decodeBody(raw), nil
}

// send executes a request with the retry policy and returns the raw body of
// a 2xx answer:
//   - 5xx answers and network errors are retried only for GET, HEAD and
//     DELETE: a POST, PUT or PATCH may already have done its work;
//   - a 429 rate limit is retried on any method (the gateway refused it
//     before any work), honouring Retry-After / retry_after / reset_at,
//     except the playground quota (retry_after 0, no reset), which does not
//     clear by waiting;
//   - a 503 billing_unavailable is retried on any method (no work was done).
func (t *transport) send(
	ctx context.Context,
	method, reqURL string,
	body map[string]interface{},
) (int, http.Header, []byte, error) {
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return 0, nil, nil, fmt.Errorf("kloakd: marshal request body: %w", err)
		}
	}

	for attempt := 0; ; attempt++ {
		var reqBody io.Reader
		if bodyBytes != nil {
			reqBody = bytes.NewReader(bodyBytes)
		}
		req, err := http.NewRequestWithContext(ctx, method, reqURL, reqBody)
		if err != nil {
			return 0, nil, nil, fmt.Errorf("kloakd: create request: %w", err)
		}
		t.setHeaders(req, bodyBytes != nil)

		resp, err := t.httpClient.Do(req)
		var respBody []byte
		if err == nil {
			respBody, err = io.ReadAll(resp.Body)
			resp.Body.Close()
		}
		if err != nil {
			if ctx.Err() != nil {
				return 0, nil, nil, ctx.Err()
			}
			netErr := fmt.Errorf("kloakd: %s %s: %w", method, reqURL, err)
			if attempt >= t.maxRetries || !idempotent(method) {
				return 0, nil, nil, netErr
			}
			if werr := sleepCtx(ctx, t.expBackoff(attempt)); werr != nil {
				return 0, nil, nil, werr
			}
			continue
		}

		sdkErr := raiseForStatus(resp.StatusCode, decodeBody(respBody), resp.Header, respBody)
		if sdkErr == nil {
			return resp.StatusCode, resp.Header, respBody, nil
		}
		if attempt >= t.maxRetries || !shouldRetry(method, sdkErr) {
			return resp.StatusCode, resp.Header, nil, sdkErr
		}
		if werr := sleepCtx(ctx, t.retryWait(attempt, resp.Header, decodeBody(respBody))); werr != nil {
			return 0, nil, nil, werr
		}
	}
}

// idempotent reports whether a failed request may be sent again.
func idempotent(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodDelete:
		return true
	}
	return false
}

// shouldRetry is the retry policy for an error answer (see send).
func shouldRetry(method string, err error) bool {
	switch e := err.(type) {
	case *RateLimitError:
		return !e.QuotaExhausted
	case *BillingUnavailableError:
		return true
	case *UpstreamError:
		return idempotent(method)
	case *ApiError:
		switch e.StatusCode {
		case 500, 502, 503, 504:
			return idempotent(method)
		}
	}
	return false
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// expBackoff is retryBase × 2^attempt, at most maxBackoff.
func (t *transport) expBackoff(attempt int) time.Duration {
	d := time.Duration(float64(t.retryBase) * math.Pow(2, float64(attempt)))
	if d > maxBackoff || d <= 0 {
		return maxBackoff
	}
	return d
}

// retryWait is the wait before the next attempt: the Retry-After header,
// else the body's retry_after, else the time until its reset_at, else the
// exponential backoff.
func (t *transport) retryWait(attempt int, headers http.Header, body map[string]interface{}) time.Duration {
	if d, ok := retryAfterHint(headers, body, time.Now()); ok {
		return d
	}
	return t.expBackoff(attempt)
}

// retryAfterHint reads the API's retry hint: the Retry-After header
// (seconds), retry_after (in detail or at the top level) or reset_at (an
// ISO 8601 time).
func retryAfterHint(headers http.Header, body map[string]interface{}, now time.Time) (time.Duration, bool) {
	if headers != nil {
		if secs, err := strconv.ParseFloat(strings.TrimSpace(headers.Get("Retry-After")), 64); err == nil && secs > 0 {
			return time.Duration(secs * float64(time.Second)), true
		}
	}
	for _, m := range []map[string]interface{}{detailMap(body), body} {
		if secs := numField(m, "retry_after"); secs > 0 {
			return time.Duration(secs * float64(time.Second)), true
		}
	}
	for _, m := range []map[string]interface{}{detailMap(body), body} {
		if s, ok := m["reset_at"].(string); ok && s != "" {
			if at, err := time.Parse(time.RFC3339, s); err == nil {
				if d := at.Sub(now); d > 0 {
					return d, true
				}
			}
		}
	}
	return 0, false
}

// decodeBody parses a JSON body; an array becomes {"items": [...]}.
func decodeBody(respBody []byte) map[string]interface{} {
	parsed := map[string]interface{}{}
	if len(respBody) == 0 {
		return parsed
	}
	var anyBody interface{}
	if err := json.Unmarshal(respBody, &anyBody); err != nil {
		return parsed
	}
	switch v := anyBody.(type) {
	case map[string]interface{}:
		return v
	case []interface{}:
		parsed["items"] = v
	}
	return parsed
}

func (t *transport) get(ctx context.Context, path string, params map[string]string) (map[string]interface{}, error) {
	return t.request(ctx, http.MethodGet, path, nil, params)
}

// getList is get for a route that answers a JSON array.
func (t *transport) getList(ctx context.Context, path string, params map[string]string) ([]map[string]interface{}, error) {
	raw, err := t.request(ctx, http.MethodGet, path, nil, params)
	if err != nil {
		return nil, err
	}
	return sliceMapField(raw, "items"), nil
}

// getRaw is get for a route that answers something other than JSON (text,
// a ZIP archive); the body is returned as is.
func (t *transport) getRaw(ctx context.Context, path string, params map[string]string) ([]byte, http.Header, error) {
	_, headers, raw, err := t.send(ctx, http.MethodGet, t.buildURL(path, params), nil)
	return raw, headers, err
}

// postRaw is post for a route that answers something other than JSON.
func (t *transport) postRaw(ctx context.Context, path string, body map[string]interface{}) ([]byte, http.Header, error) {
	if body == nil {
		body = map[string]interface{}{}
	}
	_, headers, raw, err := t.send(ctx, http.MethodPost, t.buildURL(path, nil), body)
	return raw, headers, err
}

func (t *transport) post(ctx context.Context, path string, body map[string]interface{}) (map[string]interface{}, error) {
	return t.request(ctx, http.MethodPost, path, body, nil)
}

func (t *transport) patch(ctx context.Context, path string, body map[string]interface{}) (map[string]interface{}, error) {
	return t.request(ctx, http.MethodPatch, path, body, nil)
}

func (t *transport) delete(ctx context.Context, path string) error {
	_, err := t.request(ctx, http.MethodDelete, path, nil, nil)
	return err
}

// ─── server-sent events ──────────────────────────────────────────────────────

// sseMessage is one server-sent event: its event name and JSON data, or the
// error that ended the stream.
type sseMessage struct {
	event string
	data  map[string]interface{}
	err   error
}

// postStream opens a server-sent events stream with POST (Parlyr chat).
func (t *transport) postStream(ctx context.Context, path string, body map[string]interface{}) (<-chan sseMessage, error) {
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("kloakd: marshal request body: %w", err)
	}
	return t.openStream(ctx, http.MethodPost, t.buildURL(path, nil), bodyBytes)
}

// getStream opens a server-sent events stream with GET (job and crawl
// event routes).
func (t *transport) getStream(ctx context.Context, path string, params map[string]string) (<-chan sseMessage, error) {
	return t.openStream(ctx, http.MethodGet, t.buildURL(path, params), nil)
}

// openStream opens an SSE stream on the client without a Timeout (ctx ends
// it) and returns its events. The channel closes when the stream ends; if
// reading it failed, the last message carries the error.
func (t *transport) openStream(ctx context.Context, method, reqURL string, bodyBytes []byte) (<-chan sseMessage, error) {
	var reqBody io.Reader
	if bodyBytes != nil {
		reqBody = bytes.NewReader(bodyBytes)
	}
	req, err := http.NewRequestWithContext(ctx, method, reqURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("kloakd: create request: %w", err)
	}
	t.setHeaders(req, bodyBytes != nil)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := t.streamClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("kloakd: %s %s: %w", method, reqURL, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, raiseForStatus(resp.StatusCode, decodeBody(errBody), resp.Header, errBody)
	}

	ch := make(chan sseMessage, 16)
	go func() {
		defer resp.Body.Close()
		defer close(ch)
		send := func(m sseMessage) bool {
			select {
			case ch <- m:
				return true
			case <-ctx.Done():
				return false
			}
		}
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 64*1024), maxSSEToken)
		event := ""
		var data []string
		dispatch := func() bool {
			defer func() { event, data = "", nil }()
			if len(data) == 0 {
				return true
			}
			var payload map[string]interface{}
			if err := json.Unmarshal([]byte(strings.Join(data, "\n")), &payload); err != nil {
				return true // not a JSON object: skipped
			}
			return send(sseMessage{event: event, data: payload})
		}
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case line == "":
				if !dispatch() {
					return
				}
			case strings.HasPrefix(line, ":"):
				// comment (keep-alive)
			case strings.HasPrefix(line, "event:"):
				event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			case strings.HasPrefix(line, "data:"):
				data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			}
		}
		if err := scanner.Err(); err != nil {
			if ctx.Err() == nil {
				send(sseMessage{err: fmt.Errorf("kloakd: read event stream: %w", err)})
			}
			return
		}
		dispatch()
	}()
	return ch, nil
}

// mapStream converts a stream's messages with f, closing the result when
// the input closes or ctx ends.
func mapStream[T any](ctx context.Context, in <-chan sseMessage, f func(sseMessage) T) <-chan T {
	out := make(chan T, 16)
	go func() {
		defer close(out)
		for m := range in {
			select {
			case out <- f(m):
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}
