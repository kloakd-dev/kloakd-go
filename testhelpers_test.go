package kloakd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"
)

const (
	testAPIKey = "sk-test-fixture-key"
	testOrgID  = "00000000-0000-0000-0000-000000000001"
)

// newTestClient creates a Client pointing at the given httptest.Server.
func newTestClient(server *httptest.Server) *Client {
	client, err := New(Config{
		APIKey:         testAPIKey,
		OrganizationID: testOrgID,
		BaseURL:        server.URL,
		MaxRetries:     0, // no retries in unit tests
		HTTPClient:     server.Client(),
	})
	if err != nil {
		panic(fmt.Sprintf("newTestClient: %v", err))
	}
	return client
}

// jsonHandler returns an httptest handler that responds with the given JSON body.
func jsonHandler(statusCode int, body interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(body)
	}
}

// captureHandler returns a handler that records request body + calls inner.
type captureHandler struct {
	lastBody map[string]interface{}
	lastURL  string
	inner    http.HandlerFunc
}

func (c *captureHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.lastURL = r.URL.String()
	body := map[string]interface{}{}
	_ = json.NewDecoder(r.Body).Decode(&body)
	c.lastBody = body
	c.inner(w, r)
}

// sseHandler returns an httptest handler that streams SSE payloads.
func sseHandler(payloads []map[string]interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		for _, payload := range payloads {
			b, _ := json.Marshal(payload)
			fmt.Fprintf(w, "data: %s\n\n", b)
			if ok {
				flusher.Flush()
			}
		}
	}
}

// sseEventHandler returns an httptest handler that streams SSE with event names.
func sseEventHandler(pairs []struct{ Event string; Data map[string]interface{} }) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		for _, pair := range pairs {
			b, _ := json.Marshal(pair.Data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", pair.Event, b)
			if ok {
				flusher.Flush()
			}
		}
	}
}

// retryHandler returns a handler that responds with errStatus N times, then ok.
func retryHandler(errStatus, successStatus int, failCount int, body interface{}) http.HandlerFunc {
	calls := 0
	return func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls <= failCount {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(errStatus)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"detail": "temporary error"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(successStatus)
		_ = json.NewEncoder(w).Encode(body)
	}
}

// assertHeader fails the test if the request doesn't have the expected header value.
func assertHeader(t interface{ Errorf(string, ...interface{}) }, r *http.Request, key, expected string) {
	if got := r.Header.Get(key); got != expected {
		t.Errorf("header %s: expected %q, got %q", key, expected, got)
	}
}

// assertHeaderContains fails the test if the header doesn't contain the substring.
func assertHeaderContains(t interface{ Errorf(string, ...interface{}) }, r *http.Request, key, substr string) {
	if got := r.Header.Get(key); !strings.Contains(got, substr) {
		t.Errorf("header %s: expected to contain %q, got %q", key, substr, got)
	}
}

// ptr returns a pointer to the given value.
func ptr[T any](v T) *T { return &v }

// drain reads all values from a channel with a timeout.
func drain[T any](ch <-chan T, timeout time.Duration) []T {
	var result []T
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case v, ok := <-ch:
			if !ok {
				return result
			}
			result = append(result, v)
		case <-timer.C:
			return result
		}
	}
}
