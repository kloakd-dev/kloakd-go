package kloakd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestTransport_AuthHeaders(t *testing.T) {
	var capturedReq *http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = r
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := newTestClient(server)
	_, err := client.Evadr.Fetch(context.Background(), "https://example.com", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertHeader(t, capturedReq, "Authorization", "Bearer "+testAPIKey)
	assertHeader(t, capturedReq, "X-Kloakd-Organization", testOrgID)
	assertHeaderContains(t, capturedReq, "X-Kloakd-SDK", "go/")
}

func TestTransport_OrgPrefixInURL(t *testing.T) {
	var capturedURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := newTestClient(server)
	_, _ = client.Evadr.Fetch(context.Background(), "https://example.com", nil)

	expected := "/api/v1/organizations/" + testOrgID + "/evadr/fetch"
	if capturedURL != expected {
		t.Errorf("expected URL path %s, got %s", expected, capturedURL)
	}
}

func TestTransport_NonRetryableError(t *testing.T) {
	server := httptest.NewServer(jsonHandler(401, map[string]interface{}{"detail": "invalid key"}))
	defer server.Close()

	client := newTestClient(server)
	_, err := client.Evadr.Fetch(context.Background(), "https://example.com", nil)
	var authErr *AuthenticationError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected AuthenticationError, got %T: %v", err, err)
	}
}

// retryClient is a client with maxRetries retries and a 1 ms backoff step.
func retryClient(server *httptest.Server, maxRetries int) *Client {
	client, _ := New(Config{APIKey: testAPIKey, OrganizationID: testOrgID, BaseURL: server.URL,
		MaxRetries: maxRetries, HTTPClient: server.Client()})
	client.t.retryBase = time.Millisecond
	return client
}

func TestTransport_RetryExhausted(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		w.Write([]byte(`{"detail":"server error"}`))
	}))
	defer server.Close()

	_, err := retryClient(server, 2).Evadr.GetJob(context.Background(), "job-1")
	var apiErr *ApiError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected ApiError, got %T: %v", err, err)
	}
	if calls != 3 { // 1 initial + 2 retries
		t.Errorf("expected 3 calls (1 + 2 retries), got %d", calls)
	}
}

func TestTransport_5xxIsNotRetriedForPOST(t *testing.T) {
	for _, status := range []int{500, 502, 503, 504} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.WriteHeader(status)
		}))
		_, err := retryClient(server, 3).Evadr.Fetch(context.Background(), "https://example.com", nil)
		server.Close()
		if err == nil || calls != 1 {
			t.Errorf("POST %d: expected one call and an error, got %d calls, %v", status, calls, err)
		}
	}
}

func TestTransport_5xxIsRetriedForDELETE(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(204)
	}))
	defer server.Close()
	if err := retryClient(server, 3).Evadr.DeleteProxy(context.Background(), "p1"); err != nil || calls != 3 {
		t.Fatalf("expected success on the third call, got %d calls, %v", calls, err)
	}
}

func TestTransport_NetworkErrorIsRetriedForGETOnly(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			hj, _ := w.(http.Hijacker)
			conn, _, _ := hj.Hijack()
			conn.Close() // drop the connection: a network error
			return
		}
		jsonHandler(200, map[string]interface{}{"status": "running"})(w, r)
	}))
	defer server.Close()
	client := retryClient(server, 2)
	if _, err := client.Evadr.GetJob(context.Background(), "job-1"); err != nil || atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("GET: expected a retry after the dropped connection, got %d calls, %v", atomic.LoadInt32(&calls), err)
	}
	atomic.StoreInt32(&calls, 0)
	if _, err := client.Evadr.Fetch(context.Background(), "https://example.com", nil); err == nil || atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("POST: a dropped connection must not be retried, got %d calls, %v", atomic.LoadInt32(&calls), err)
	}
}

func TestTransport_RateLimitIsRetriedForPOSTHonouringRetryAfter(t *testing.T) {
	var times []time.Time
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		times = append(times, time.Now())
		if len(times) == 1 {
			w.Header().Set("Retry-After", "1")
			jsonHandler(429, detail("rate_limited", "slow down", map[string]interface{}{"retry_after": float64(1), "reset_at": "2099-01-01T00:00:00Z"}))(w, r)
			return
		}
		jsonHandler(200, fetchResponse)(w, r)
	}))
	defer server.Close()
	result, err := retryClient(server, 3).Evadr.Fetch(context.Background(), "https://example.com", nil)
	if err != nil || !result.Success || len(times) != 2 {
		t.Fatalf("expected success after one retry, got %d calls, %v", len(times), err)
	}
	if gap := times[1].Sub(times[0]); gap < 900*time.Millisecond {
		t.Errorf("Retry-After 1 s not honoured: waited %v", gap)
	}
}

func TestTransport_QuotaExhaustedIsNotRetried(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Retry-After", "0")
		jsonHandler(429, detail("rate_limited", "Rate limit exceeded. Retry after 0s.", map[string]interface{}{
			"retry_after": float64(0), "reset_at": "", "quota_used": float64(25), "quota_limit": float64(25),
		}))(w, r)
	}))
	defer server.Close()
	_, err := retryClient(server, 3).Evadr.Fetch(context.Background(), "https://example.com", nil)
	var rl *RateLimitError
	if !errors.As(err, &rl) || !rl.QuotaExhausted || rl.QuotaLimit != 25 || calls != 1 {
		t.Fatalf("expected the playground quota after one call, got %d calls, %v", calls, err)
	}
}

func TestTransport_BillingUnavailableIsRetriedForPOST(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			jsonHandler(503, detail("billing_unavailable", "Usage could not be checked", nil))(w, r)
			return
		}
		jsonHandler(200, fetchResponse)(w, r)
	}))
	defer server.Close()
	if _, err := retryClient(server, 3).Evadr.Fetch(context.Background(), "https://example.com", nil); err != nil || calls != 2 {
		t.Fatalf("expected success after one retry, got %d calls, %v", calls, err)
	}
}

func TestRetryAfterHint(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		headers http.Header
		body    map[string]interface{}
		want    time.Duration
		ok      bool
	}{
		{"header", http.Header{"Retry-After": []string{"5"}}, nil, 5 * time.Second, true},
		{"body detail", nil, map[string]interface{}{"detail": map[string]interface{}{"retry_after": float64(10)}}, 10 * time.Second, true},
		{"body top", nil, map[string]interface{}{"retry_after": float64(2)}, 2 * time.Second, true},
		{"reset_at", nil, map[string]interface{}{"detail": map[string]interface{}{"reset_at": "2026-10-28T12:00:30Z"}}, 30 * time.Second, true},
		{"reset_at past", nil, map[string]interface{}{"reset_at": "2026-10-28T11:00:00Z"}, 0, false},
		{"none", http.Header{"Retry-After": []string{"0"}}, map[string]interface{}{}, 0, false},
	}
	for _, c := range cases {
		got, ok := retryAfterHint(c.headers, c.body, now)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: got %v %v, want %v %v", c.name, got, ok, c.want, c.ok)
		}
	}
}

func TestBackoff_ExponentialAndCapped(t *testing.T) {
	tr := &transport{retryBase: time.Second}
	if tr.expBackoff(0) != time.Second || tr.expBackoff(1) != 2*time.Second || tr.expBackoff(3) != 8*time.Second {
		t.Errorf("unexpected backoff: %v %v %v", tr.expBackoff(0), tr.expBackoff(1), tr.expBackoff(3))
	}
	if tr.expBackoff(10) != maxBackoff || tr.expBackoff(100) != maxBackoff {
		t.Errorf("backoff must be capped at %v", maxBackoff)
	}
}

func TestTransport_DefaultTimeoutIsAboveTheSyncWait(t *testing.T) {
	if defaultTimeout != 100*time.Second {
		t.Fatalf("default timeout %v, want 100 s (the API waits 85 s before a 202)", defaultTimeout)
	}
	c, _ := New(Config{APIKey: testAPIKey, OrganizationID: testOrgID})
	if c.t.httpClient.Timeout != 100*time.Second || c.t.streamClient.Timeout != 0 {
		t.Fatalf("request timeout %v, stream timeout %v", c.t.httpClient.Timeout, c.t.streamClient.Timeout)
	}
	custom := &http.Client{Timeout: time.Second}
	c, _ = New(Config{APIKey: testAPIKey, OrganizationID: testOrgID, HTTPClient: custom})
	if c.t.httpClient != custom || c.t.streamClient.Timeout != 0 || custom.Timeout != time.Second {
		t.Fatal("a custom client keeps its timeout for requests; streams use a copy without one")
	}
}

func TestTransport_StreamOutlivesTheRequestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		f := w.(http.Flusher)
		for i := 0; i < 3; i++ {
			fmt.Fprintf(w, "data: {\"event_type\":\"stage_progress\",\"sequence\":%d}\n\n", i)
			f.Flush()
			time.Sleep(150 * time.Millisecond)
		}
	}))
	defer server.Close()
	client, _ := New(Config{APIKey: testAPIKey, OrganizationID: testOrgID, BaseURL: server.URL,
		Timeout: 100 * time.Millisecond, MaxRetries: -1})
	ch, err := client.Webgrph.CrawlEvents(context.Background(), "c1")
	if err != nil {
		t.Fatal(err)
	}
	events := drain(ch, 3*time.Second)
	if len(events) != 3 {
		t.Fatalf("expected 3 events past the 100 ms request timeout, got %+v", events)
	}
	for _, e := range events {
		if e.Err != nil {
			t.Fatalf("unexpected stream error: %v", e.Err)
		}
	}
}

func TestTransport_StreamReadErrorIsReturned(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Content-Length", "1000") // promise more than is sent
		w.WriteHeader(200)
		_, _ = w.Write([]byte("data: {\"event_type\":\"stage_started\"}\n\n"))
		w.(http.Flusher).Flush()
		hj, _ := w.(http.Hijacker)
		conn, _, _ := hj.Hijack()
		conn.Close()
	}))
	defer server.Close()
	ch, err := newTestClient(server).Webgrph.CrawlEvents(context.Background(), "c1")
	if err != nil {
		t.Fatal(err)
	}
	events := drain(ch, 2*time.Second)
	if len(events) != 2 || events[0].EventType != "stage_started" || events[1].Err == nil {
		t.Fatalf("expected an event then the read error, got %+v", events)
	}
}

func TestTransport_StreamLongLinesAndMultiLineData(t *testing.T) {
	big := strings.Repeat("x", 200*1024) // above bufio.Scanner's 64 KiB default
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fmt.Fprintf(w, ": keepalive\n\nevent: results\ndata: {\"type\":\"results\",\"content\":\"%s\"}\n\n", big)
		fmt.Fprint(w, "event: text\ndata: {\"type\":\"text\",\ndata: \"content\":\"two lines\"}\n\nevent: end\ndata: {}\n\n")
	}))
	defer server.Close()
	ch, err := newTestClient(server).Parlyr.ChatStream(context.Background(), "s", "m")
	if err != nil {
		t.Fatal(err)
	}
	events := drain(ch, 2*time.Second)
	if len(events) != 2 || events[0].Err != nil || events[0].Content == nil || len(*events[0].Content) != len(big) {
		t.Fatalf("unexpected events: %d", len(events))
	}
	if events[1].Content == nil || *events[1].Content != "two lines" {
		t.Fatalf("unexpected event: %+v", events[1])
	}
}

func TestTransport_UserAgentAndVersion(t *testing.T) {
	var got *http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		jsonHandler(200, map[string]interface{}{})(w, r)
	}))
	defer server.Close()
	_, _ = newTestClient(server).Evadr.ListVendors(context.Background())
	assertHeader(t, got, "User-Agent", "kloakd-sdk-go/0.5.0")
	assertHeader(t, got, "X-Kloakd-SDK", "go/0.5.0")
}

func TestTransport_PathSegmentsAreEscaped(t *testing.T) {
	var rawPath, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawPath, path = r.URL.EscapedPath(), r.URL.Path
		jsonHandler(200, map[string]interface{}{})(w, r)
	}))
	defer server.Close()
	client := newTestClient(server)
	prefix := "/api/v1/organizations/" + testOrgID + "/"

	_ = client.Fetchyr.DeleteCredentials(context.Background(), "my login/../x?y")
	if want := prefix + "fetchyr/account/credentials/my%20login%2F..%2Fx%3Fy"; rawPath != want {
		t.Errorf("escaped path %s, want %s", rawPath, want)
	}
	endpoint := "https://api.example.com/v1/items?page=2"
	_, _ = client.Kolektr.GetAPIDataPaginated(context.Background(), endpoint, 0, 10, nil)
	if want := prefix + "kolektr/api-data/" + endpoint + "/paginated"; path != want {
		t.Errorf("decoded path %s, want %s (the API's {api_endpoint:path})", path, want)
	}
	if !strings.Contains(rawPath, "https:%2F%2Fapi.example.com%2Fv1%2Fitems%3Fpage=2/paginated") {
		t.Errorf("the endpoint must be one escaped segment, got %s", rawPath)
	}
}

func TestTransport_QueryParams(t *testing.T) {
	var capturedURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := newTestClient(server)
	_, _ = client.Webgrph.DetectedAPIs(context.Background(), "https://example.com/p")
	if !strings.Contains(capturedURL, "webgrph/detected-apis?page_url=https%3A%2F%2Fexample.com%2Fp") {
		t.Errorf("expected the page_url query, got %s", capturedURL)
	}
}

func TestTransport_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	client := newTestClient(server)
	_, err := client.Evadr.Fetch(ctx, "https://example.com", nil)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}
