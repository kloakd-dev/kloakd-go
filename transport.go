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
	"strings"
	"time"
)

const (
	sdkVersion     = "0.1.0"
	defaultBaseURL = "https://api.kloakd.dev"
	defaultTimeout = 30 * time.Second
	defaultRetries = 3
)

// transport is the shared HTTP transport for all namespace methods.
// It is not part of the public API.
type transport struct {
	apiKey         string
	organizationID string
	baseURL        string
	timeout        time.Duration
	maxRetries     int
	httpClient     *http.Client
}

func newTransport(cfg Config) *transport {
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: cfg.effectiveTimeout()}
	}
	return &transport{
		apiKey:         cfg.APIKey,
		organizationID: cfg.OrganizationID,
		baseURL:        strings.TrimRight(cfg.effectiveBaseURL(), "/"),
		timeout:        cfg.effectiveTimeout(),
		maxRetries:     cfg.effectiveMaxRetries(),
		httpClient:     client,
	}
}

func (t *transport) authHeaders() map[string]string {
	return map[string]string{
		"Authorization":        "Bearer " + t.apiKey,
		"X-Kloakd-Organization": t.organizationID,
		"X-Kloakd-SDK":        "go/" + sdkVersion,
		"Content-Type":        "application/json",
	}
}

func (t *transport) orgPrefix() string {
	return fmt.Sprintf("/api/v1/organizations/%s", t.organizationID)
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
	reqURL := t.buildURL(path, params)

	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("kloakd: marshal request body: %w", err)
		}
	}

	var lastErr error
	for attempt := 0; attempt <= t.maxRetries; attempt++ {
		var reqBody io.Reader
		if bodyBytes != nil {
			reqBody = bytes.NewReader(bodyBytes)
		}

		req, err := http.NewRequestWithContext(ctx, method, reqURL, reqBody)
		if err != nil {
			return nil, fmt.Errorf("kloakd: create request: %w", err)
		}
		for k, v := range t.authHeaders() {
			req.Header.Set(k, v)
		}

		resp, err := t.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("kloakd: execute request: %w", err)
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("kloakd: read response: %w", err)
		}

		var parsed map[string]interface{}
		if len(respBody) > 0 {
			_ = json.Unmarshal(respBody, &parsed)
		}
		if parsed == nil {
			parsed = map[string]interface{}{}
		}

		sdkErr := raiseForStatus(resp.StatusCode, parsed)
		if sdkErr == nil {
			return parsed, nil
		}
		if !isRetryable(resp.StatusCode) {
			return nil, sdkErr
		}

		lastErr = sdkErr
		if attempt < t.maxRetries {
			wait := backoff(attempt, resp.Header.Get("Retry-After"), parsed)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
		}
	}
	return nil, lastErr
}

func (t *transport) get(ctx context.Context, path string, params map[string]string) (map[string]interface{}, error) {
	return t.request(ctx, http.MethodGet, path, nil, params)
}

func (t *transport) post(ctx context.Context, path string, body map[string]interface{}) (map[string]interface{}, error) {
	return t.request(ctx, http.MethodPost, path, body, nil)
}

func (t *transport) delete(ctx context.Context, path string) error {
	_, err := t.request(ctx, http.MethodDelete, path, nil, nil)
	return err
}

// Stream opens a server-sent events stream and returns a channel of parsed data payloads.
// The channel is closed when the stream ends. Any parse error closes the channel.
// Callers must drain or stop reading to allow cleanup.
func (t *transport) stream(
	ctx context.Context,
	path string,
	body map[string]interface{},
) (<-chan map[string]interface{}, error) {
	return t.streamInternal(ctx, path, body, false)
}

// streamWithEvents opens an SSE stream and returns a channel of {event, data} pairs.
func (t *transport) streamWithEvents(
	ctx context.Context,
	path string,
	body map[string]interface{},
) (<-chan ChatEvent, error) {
	reqURL := t.buildURL(path, nil)

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("kloakd: marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("kloakd: create request: %w", err)
	}
	for k, v := range t.authHeaders() {
		req.Header.Set(k, v)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kloakd: execute request: %w", err)
	}

	if err := raiseForStatus(resp.StatusCode, map[string]interface{}{}); err != nil {
		resp.Body.Close()
		return nil, err
	}

	ch := make(chan ChatEvent, 16)
	go func() {
		defer resp.Body.Close()
		defer close(ch)
		scanner := bufio.NewScanner(resp.Body)
		currentEvent := ""
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "event:") {
				currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			} else if strings.HasPrefix(line, "data:") {
				dataStr := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				if dataStr == "" {
					continue
				}
				var data map[string]interface{}
				if err := json.Unmarshal([]byte(dataStr), &data); err != nil {
					continue
				}
				select {
				case ch <- ChatEvent{Event: currentEvent, Data: data}:
				case <-ctx.Done():
					return
				}
				currentEvent = ""
			}
		}
	}()
	return ch, nil
}

func (t *transport) streamInternal(
	ctx context.Context,
	path string,
	body map[string]interface{},
	_ bool,
) (<-chan map[string]interface{}, error) {
	reqURL := t.buildURL(path, nil)

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("kloakd: marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("kloakd: create request: %w", err)
	}
	for k, v := range t.authHeaders() {
		req.Header.Set(k, v)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kloakd: execute request: %w", err)
	}

	if err := raiseForStatus(resp.StatusCode, map[string]interface{}{}); err != nil {
		resp.Body.Close()
		return nil, err
	}

	ch := make(chan map[string]interface{}, 16)
	go func() {
		defer resp.Body.Close()
		defer close(ch)
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			dataStr := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if dataStr == "" {
				continue
			}
			var payload map[string]interface{}
			if err := json.Unmarshal([]byte(dataStr), &payload); err != nil {
				continue
			}
			select {
			case ch <- payload:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

// backoff computes the sleep duration before the next retry attempt.
// retryAfterHdr is the raw Retry-After header value (may be empty).
func backoff(attempt int, retryAfterHdr string, body map[string]interface{}) time.Duration {
	if retryAfterHdr != "" {
		var secs float64
		if _, err := fmt.Sscanf(retryAfterHdr, "%f", &secs); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	if v, ok := body["retry_after"]; ok {
		if secs, ok := v.(float64); ok && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	ms := math.Min(float64(1000)*math.Pow(2, float64(attempt)), 60_000)
	return time.Duration(ms) * time.Millisecond
}
