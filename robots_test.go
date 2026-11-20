package kloakd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

// The 409 answer of a robots.txt stop (core/compliance/robots.py stop_detail).
func robotsStopBody() map[string]interface{} {
	return map[string]interface{}{"detail": map[string]interface{}{
		"error":             "ROBOTS_DISALLOWED",
		"code":              "ROBOTS_DISALLOWED",
		"reason":            "disallowed",
		"host":              "www.reddit.com",
		"url":               "https://www.reddit.com/r/Python/",
		"rule":              "Disallow: /",
		"message":           "www.reddit.com does not allow this in its robots.txt.",
		"acknowledge_field": "robots_acknowledged_hosts",
		"terms_version":     "draft-2026-11-19",
		"redirected_from":   "https://reddit.com/r/Python/",
	}}
}

// The 503 answer when KLOAKD could not ask (robots.py unavailable).
func robotsUnavailableBody(reason string) map[string]interface{} {
	d := map[string]interface{}{
		"error":         "ROBOTS_CHECK_UNAVAILABLE",
		"code":          "ROBOTS_CHECK_UNAVAILABLE",
		"host":          "www.gov.uk",
		"message":       "robots.txt could not be read",
		"retry_after_s": float64(30),
	}
	if reason != "" {
		d["reason"] = reason
	}
	return map[string]interface{}{"detail": d}
}

var ackHosts = []string{"www.reddit.com"}

func wantAck(t *testing.T, rs *routeServer, key string) {
	t.Helper()
	body, ok := rs.bodies[key]
	if !ok {
		t.Fatalf("%s was not called (seen %v)", key, rs.seen)
	}
	got, _ := body["robots_acknowledged_hosts"].([]interface{})
	if !reflect.DeepEqual(got, []interface{}{"www.reddit.com"}) {
		t.Errorf("%s: robots_acknowledged_hosts = %v, want [www.reddit.com]", key, body["robots_acknowledged_hosts"])
	}
}

func wantNoAck(t *testing.T, rs *routeServer, key string) {
	t.Helper()
	if _, ok := rs.bodies[key]["robots_acknowledged_hosts"]; ok {
		t.Errorf("%s: robots_acknowledged_hosts sent without being set", key)
	}
}

func TestRobotsAck_SentByEveryBodyRoute(t *testing.T) {
	ok := jsonHandler(200, map[string]interface{}{"success": true, "crawl_id": "c1", "job_id": "j1"})
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST evadr/fetch":                       ok,
		"POST evadr/fetch/async":                 ok,
		"POST evadr/scan":                        ok,
		"POST evadr/profile":                     ok,
		"POST kolektr/extract":                   ok,
		"POST webgrph/crawl":                     jsonHandler(202, map[string]interface{}{"crawl_id": "c1"}),
		"POST webgrph/crawl/estimate":            ok,
		"POST webgrph/crawl-extract/estimate":    ok,
		"POST webgrph/crawl/c1/continue":         ok,
		"POST webgrph/crawl-extract/j1/continue": ok,
		"POST webgrph/map":                       ok,
		"POST webgrph/reader-view":               ok,
		"POST webgrph/crawl-extract":             jsonHandler(202, map[string]interface{}{"job_id": "j1"}),
		"POST skanyr/sync":                       ok,
		"POST scraping/unified":                  ok,
		"POST batch-jobs":                        jsonHandler(201, map[string]interface{}{"id": "b1"}),
	})
	defer server.Close()
	c := newTestClient(server)
	ctx := context.Background()
	u := "https://www.reddit.com/r/Python/"

	calls := []struct {
		key string
		run func() error
	}{
		{"POST evadr/fetch", func() error {
			_, err := c.Evadr.Fetch(ctx, u, &FetchOptions{RobotsAcknowledgedHosts: ackHosts})
			return err
		}},
		{"POST evadr/fetch/async", func() error {
			_, err := c.Evadr.FetchAsync(ctx, u, &FetchAsyncOptions{RobotsAcknowledgedHosts: ackHosts})
			return err
		}},
		{"POST evadr/scan", func() error {
			_, err := c.Evadr.Scan(ctx, u, &ScanOptions{RobotsAcknowledgedHosts: ackHosts})
			return err
		}},
		{"POST evadr/profile", func() error {
			_, err := c.Evadr.Profile(ctx, u, &ProfileOptions{RobotsAcknowledgedHosts: ackHosts})
			return err
		}},
		{"POST kolektr/extract", func() error {
			_, err := c.Kolektr.Extract(ctx, u, &ExtractOptions{RobotsAcknowledgedHosts: ackHosts})
			return err
		}},
		{"POST webgrph/crawl", func() error {
			_, err := c.Webgrph.StartCrawl(ctx, u, &CrawlOptions{RobotsAcknowledgedHosts: ackHosts})
			return err
		}},
		{"POST webgrph/crawl/estimate", func() error {
			_, err := c.Webgrph.EstimateCrawl(ctx, u, &CrawlEstimateOptions{RobotsAcknowledgedHosts: ackHosts})
			return err
		}},
		{"POST webgrph/crawl-extract/estimate", func() error {
			_, err := c.Webgrph.EstimateCrawlExtract(ctx, u, &CrawlExtractEstimateOptions{RobotsAcknowledgedHosts: ackHosts})
			return err
		}},
		{"POST webgrph/crawl/c1/continue", func() error {
			_, err := c.Webgrph.ContinueCrawl(ctx, "c1", &ContinueOptions{RobotsAcknowledgedHosts: ackHosts})
			return err
		}},
		{"POST webgrph/crawl-extract/j1/continue", func() error {
			_, err := c.Webgrph.ContinueCrawlExtract(ctx, "j1", &ContinueOptions{RobotsAcknowledgedHosts: ackHosts})
			return err
		}},
		{"POST webgrph/map", func() error {
			_, err := c.Webgrph.MapSite(ctx, u, &MapSiteOptions{RobotsAcknowledgedHosts: ackHosts})
			return err
		}},
		{"POST webgrph/reader-view", func() error {
			_, err := c.Webgrph.ReaderView(ctx, u, &ReaderViewOptions{RobotsAcknowledgedHosts: ackHosts})
			return err
		}},
		{"POST webgrph/crawl-extract", func() error {
			_, err := c.Webgrph.StartCrawlExtract(ctx, u, &CrawlExtractOptions{RobotsAcknowledgedHosts: ackHosts})
			return err
		}},
		{"POST skanyr/sync", func() error {
			_, err := c.Skanyr.Sync(ctx, "site-1", &SyncOptions{RobotsAcknowledgedHosts: ackHosts})
			return err
		}},
		{"POST scraping/unified", func() error {
			_, err := c.UnifiedScrape(ctx, u, &UnifiedScrapeOptions{RobotsAcknowledgedHosts: ackHosts})
			return err
		}},
		{"POST batch-jobs", func() error {
			_, err := c.CreateBatchJob(ctx, []string{u}, &BatchJobOptions{RobotsAcknowledgedHosts: ackHosts})
			return err
		}},
	}
	for _, call := range calls {
		if err := call.run(); err != nil {
			t.Fatalf("%s: %v", call.key, err)
		}
		wantAck(t, rs, call.key)
	}
}

func TestRobotsAck_NotSentUnlessSet(t *testing.T) {
	ok := jsonHandler(200, map[string]interface{}{"success": true})
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST evadr/fetch":               ok,
		"POST evadr/scan":                ok,
		"POST kolektr/extract":           ok,
		"POST webgrph/crawl/c1/continue": ok,
	})
	defer server.Close()
	c := newTestClient(server)
	ctx := context.Background()
	if _, err := c.Evadr.Fetch(ctx, "https://e.com", &FetchOptions{RobotsAcknowledgedHosts: []string{}}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Evadr.Scan(ctx, "https://e.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Kolektr.Extract(ctx, "https://e.com", nil); err != nil {
		t.Fatal(err)
	}
	wantNoAck(t, rs, "POST evadr/fetch")
	wantNoAck(t, rs, "POST evadr/scan")
	wantNoAck(t, rs, "POST kolektr/extract")

	// Continue sends no body with no options, as before.
	if _, err := c.Webgrph.ContinueCrawl(ctx, "c1", &ContinueOptions{}); err != nil {
		t.Fatal(err)
	}
	if body := rs.bodies["POST webgrph/crawl/c1/continue"]; len(body) != 0 {
		t.Errorf("continue with no option set sent %v", body)
	}
}

func TestRobotsAck_ContinueBodyOnlyTheAcknowledgement(t *testing.T) {
	got := continueBody([]*ContinueOptions{{RobotsAcknowledgedHosts: ackHosts}})
	want := map[string]interface{}{"robots_acknowledged_hosts": ackHosts}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("continueBody = %v, want %v", got, want)
	}
}

func TestRobotsAck_FetchStreamAndCrawlStream(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST evadr/fetch/async":      jsonHandler(202, map[string]interface{}{"job_id": "j1"}),
		"GET evadr/jobs/j1/events":    sseHandler(nil),
		"POST webgrph/crawl":          jsonHandler(202, map[string]interface{}{"crawl_id": "c1"}),
		"GET webgrph/crawl/c1/events": sseHandler(nil),
	})
	defer server.Close()
	c := newTestClient(server)
	ctx := context.Background()
	evs, err := c.Evadr.FetchStream(ctx, "https://e.com", &FetchStreamOptions{RobotsAcknowledgedHosts: ackHosts})
	if err != nil {
		t.Fatal(err)
	}
	for range evs {
	}
	cevs, err := c.Webgrph.CrawlStream(ctx, "https://e.com", &CrawlStreamOptions{RobotsAcknowledgedHosts: ackHosts})
	if err != nil {
		t.Fatal(err)
	}
	for range cevs {
	}
	wantAck(t, rs, "POST evadr/fetch/async")
	wantAck(t, rs, "POST webgrph/crawl")
}

func TestRobotsAck_ChatStream(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST parlyr/chat": sseEventHandler([]struct {
			Event string
			Data  map[string]interface{}
		}{{Event: "end", Data: map[string]interface{}{}}}),
	})
	defer server.Close()
	c := newTestClient(server)
	if _, err := c.Parlyr.Chat(context.Background(), "s1", "fetch reddit", &ChatOptions{RobotsAcknowledgedHosts: ackHosts}); err != nil {
		t.Fatal(err)
	}
	wantAck(t, rs, "POST parlyr/chat")
	if _, err := c.Parlyr.Chat(context.Background(), "s1", "hello"); err != nil {
		t.Fatal(err)
	}
	wantNoAck(t, rs, "POST parlyr/chat")
}

func TestRobotsAck_ExtractAllAPIDataQuery(t *testing.T) {
	var query url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query()
		jsonHandler(200, map[string]interface{}{"job_id": "x"})(w, r)
	}))
	defer server.Close()
	c := newTestClient(server)
	_, err := c.Kolektr.ExtractAllAPIData(context.Background(), "https://api.e.com/v1/items", &ExtractAllAPIDataOptions{
		MaxRecords:              10,
		RobotsAcknowledgedHosts: []string{"api.e.com", "www.e.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := query["robots_acknowledged_hosts"]; !reflect.DeepEqual(got, []string{"api.e.com", "www.e.com"}) {
		t.Errorf("robots_acknowledged_hosts query = %v", got)
	}
	if query.Get("max_records") != "10" {
		t.Errorf("max_records lost: %v", query)
	}
	if _, err := c.Kolektr.ExtractAllAPIData(context.Background(), "https://api.e.com/v1/items", nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := query["robots_acknowledged_hosts"]; ok {
		t.Errorf("robots_acknowledged_hosts sent without being set: %v", query)
	}
}

func TestRobotsAck_OrchestratorPassesHosts(t *testing.T) {
	if got := orchestratorCrawlOptions(&CrawlOrchestratorOptions{RobotsAcknowledgedHosts: ackHosts}).RobotsAcknowledgedHosts; !reflect.DeepEqual(got, ackHosts) {
		t.Errorf("discovery crawl hosts = %v", got)
	}
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST evadr/fetch":     jsonHandler(200, fetchResponse),
		"POST kolektr/extract": jsonHandler(200, map[string]interface{}{"success": true, "records": []interface{}{}}),
	})
	defer server.Close()
	c := newTestClient(server)
	_, err := c.crawlPage(context.Background(), "https://www.reddit.com/r/Python/", &CrawlOrchestratorOptions{
		ExtractSchema: map[string]string{"title": "h1"}, RobotsAcknowledgedHosts: ackHosts,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantAck(t, rs, "POST evadr/fetch")
	wantAck(t, rs, "POST kolektr/extract")
}

func TestRobotsErrors_409Stop(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		jsonHandler(409, robotsStopBody())(w, r)
	}))
	defer server.Close()
	_, err := retryClient(server, 3).Evadr.Fetch(context.Background(), "https://www.reddit.com/r/Python/", nil)
	var stop *RobotsDisallowedError
	if !errors.As(err, &stop) {
		t.Fatalf("want *RobotsDisallowedError, got %T %v", err, err)
	}
	if calls != 1 {
		t.Errorf("a robots.txt stop was retried: %d calls", calls)
	}
	want := RobotsDisallowedError{
		KloakdError:      KloakdError{StatusCode: 409, Code: "ROBOTS_DISALLOWED", Message: "www.reddit.com does not allow this in its robots.txt."},
		Reason:           RobotsReasonDisallowed,
		Host:             "www.reddit.com",
		Rule:             "Disallow: /",
		URL:              "https://www.reddit.com/r/Python/",
		RedirectedFrom:   "https://reddit.com/r/Python/",
		TermsVersion:     "draft-2026-11-19",
		AcknowledgeField: RobotsAcknowledgedHostsField,
	}
	if *stop != want {
		t.Errorf("got %+v\nwant %+v", *stop, want)
	}
	var ra *RobotsCheckUnavailableError
	if errors.As(err, &ra) {
		t.Error("a stop is not a robots check unavailable")
	}
}

func TestRobotsErrors_StopThenAcknowledged(t *testing.T) {
	// The documented flow: catch the stop, ask again naming its host.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap := &captureHandler{inner: func(http.ResponseWriter, *http.Request) {}}
		cap.ServeHTTP(w, r)
		if hosts, _ := cap.lastBody["robots_acknowledged_hosts"].([]interface{}); len(hosts) == 1 && hosts[0] == "www.reddit.com" {
			jsonHandler(200, fetchResponse)(w, r)
			return
		}
		jsonHandler(409, robotsStopBody())(w, r)
	}))
	defer server.Close()
	c := newTestClient(server)
	ctx := context.Background()
	_, err := c.Evadr.Fetch(ctx, "https://www.reddit.com/r/Python/", nil)
	var stop *RobotsDisallowedError
	if !errors.As(err, &stop) {
		t.Fatalf("want a stop, got %v", err)
	}
	res, err := c.Evadr.Fetch(ctx, "https://www.reddit.com/r/Python/", &FetchOptions{RobotsAcknowledgedHosts: []string{stop.Host}})
	if err != nil || !res.Success {
		t.Fatalf("acknowledged fetch: %v %+v", err, res)
	}
}

func TestRobotsErrors_StopWrappedByOrchestrator(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"POST webgrph/crawl": jsonHandler(409, robotsStopBody()),
	})
	defer server.Close()
	_, err := newTestClient(server).Crawl(context.Background(), "https://www.reddit.com/", nil)
	var stop *RobotsDisallowedError
	if !errors.As(err, &stop) || stop.Host != "www.reddit.com" {
		t.Fatalf("want a wrapped *RobotsDisallowedError, got %v", err)
	}
}

func TestRobotsErrors_409ConcurrentLimitUnchanged(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"POST evadr/fetch": jsonHandler(409, detail("concurrent_limit", "All slots in use", map[string]interface{}{"concurrent_limit": float64(2)})),
	})
	defer server.Close()
	_, err := newTestClient(server).Evadr.Fetch(context.Background(), "https://e.com", nil)
	var cl *ConcurrencyLimitError
	if !errors.As(err, &cl) || cl.ConcurrentLimit != 2 {
		t.Fatalf("want *ConcurrencyLimitError, got %T %v", err, err)
	}
	var stop *RobotsDisallowedError
	if errors.As(err, &stop) {
		t.Error("concurrent_limit read as a robots stop")
	}
}

func TestRobotsErrors_503Unavailable(t *testing.T) {
	headers := http.Header{"Retry-After": []string{"12"}}
	err := raiseForStatus(503, robotsUnavailableBody(""), headers, nil)
	var ru *RobotsCheckUnavailableError
	if !errors.As(err, &ru) {
		t.Fatalf("want *RobotsCheckUnavailableError, got %T", err)
	}
	if ru.RetryAfter != 12 || ru.Host != "www.gov.uk" || ru.Reason != "" || ru.Code != "ROBOTS_CHECK_UNAVAILABLE" || ru.StatusCode != 503 {
		t.Errorf("got %+v", *ru)
	}
	var er *RobotsExitsRefusedError
	if errors.As(err, &er) {
		t.Error("a capacity failure is not exits_refused")
	}
	// No header: the body's retry_after_s.
	err = raiseForStatus(503, robotsUnavailableBody(""), nil, nil)
	if errors.As(err, &ru); ru.RetryAfter != 30 {
		t.Errorf("RetryAfter = %d, want 30 from retry_after_s", ru.RetryAfter)
	}
}

func TestRobotsErrors_503ExitsRefused(t *testing.T) {
	err := raiseForStatus(503, robotsUnavailableBody("exits_refused"), nil, nil)
	var er *RobotsExitsRefusedError
	if !errors.As(err, &er) {
		t.Fatalf("want *RobotsExitsRefusedError, got %T", err)
	}
	if er.Reason != RobotsReasonExitsRefused || er.Host != "www.gov.uk" || er.StatusCode != 503 {
		t.Errorf("got %+v", *er)
	}
	var ru *RobotsCheckUnavailableError
	if !errors.As(err, &ru) || ru.Host != "www.gov.uk" {
		t.Error("exits_refused should also match *RobotsCheckUnavailableError")
	}
	var stop *RobotsDisallowedError
	if errors.As(err, &stop) {
		t.Error("exits_refused is not a stop")
	}
}

func TestRobotsErrors_503UnavailableRetriedOnPost(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			// A short retry_after_s keeps the test fast.
			body := robotsUnavailableBody("")
			body["detail"].(map[string]interface{})["retry_after_s"] = 0.001
			jsonHandler(503, body)(w, r)
			return
		}
		jsonHandler(200, fetchResponse)(w, r)
	}))
	defer server.Close()
	if _, err := retryClient(server, 3).Evadr.Fetch(context.Background(), "https://e.com", nil); err != nil || calls != 2 {
		t.Fatalf("expected success after one retry, got %d calls, %v", calls, err)
	}
}

func TestRobotsErrors_503ExitsRefusedNotRetried(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		jsonHandler(503, robotsUnavailableBody("exits_refused"))(w, r)
	}))
	defer server.Close()
	_, err := retryClient(server, 3).Evadr.Scan(context.Background(), "https://www.gov.uk")
	var er *RobotsExitsRefusedError
	if !errors.As(err, &er) {
		t.Fatalf("want *RobotsExitsRefusedError, got %v", err)
	}
	if calls != 1 {
		t.Errorf("exits_refused was retried: %d calls", calls)
	}
}

func TestRobotsErrors_Messages(t *testing.T) {
	for _, err := range []error{
		raiseForStatus(409, robotsStopBody(), nil, nil),
		raiseForStatus(503, robotsUnavailableBody(""), nil, nil),
		raiseForStatus(503, robotsUnavailableBody("exits_refused"), nil, nil),
	} {
		if msg := fmt.Sprint(err); msg == "" || !strings.Contains(msg, "www.") {
			t.Errorf("message %q does not name the host", msg)
		}
	}
}
