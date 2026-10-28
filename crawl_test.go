package kloakd

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// orchestratorRoutes is a finished crawl of pages a, b and c, each fetched
// with fetch and extracted with extract.
func orchestratorRoutes(crawlStatus string, fetch, extract http.HandlerFunc) map[string]http.HandlerFunc {
	routes := crawlRoutes([]string{crawlStatus})
	routes["POST evadr/fetch"] = fetch
	if extract != nil {
		routes["POST kolektr/extract"] = extract
	}
	return routes
}

func TestCrawl_CopiesTheCrawlsSuccessAndError(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"POST webgrph/crawl": jsonHandler(202, map[string]interface{}{"success": true, "crawl_id": "c1"}),
		"GET webgrph/crawl/c1": jsonHandler(200, map[string]interface{}{"crawl_id": "c1", "status": "failed",
			"error": "robots.txt disallows the site"}),
	})
	defer server.Close()
	result, err := newTestClient(server).Crawl(context.Background(), "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Success || result.Error == nil || *result.Error != "robots.txt disallows the site" || len(result.Pages) != 0 {
		t.Fatalf("the crawl's own failure must be reported: %+v", result)
	}
}

func TestCrawl_FetchesExtractsAndSendsNoDefaultLimits(t *testing.T) {
	rs, server := newRouteServer(orchestratorRoutes("completed",
		jsonHandler(200, fetchResponse),
		jsonHandler(200, extractionResponse)))
	defer server.Close()
	result, err := newTestClient(server).Crawl(context.Background(), "https://example.com", &CrawlOrchestratorOptions{
		ExtractSchema: map[string]string{"title": "css:h3"},
		CaptureMode:   "content",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success || result.PagesFetched != 3 || result.PagesFailed != 0 || result.Pages[0].StructuredData["title"] != "Book A" {
		t.Fatalf("unexpected result: %+v", result)
	}
	body := rs.bodies["POST webgrph/crawl"]
	if _, ok := body["max_depth"]; ok {
		t.Errorf("max_depth must not be sent unless set: %v", body)
	}
	if _, ok := body["max_pages"]; ok {
		t.Errorf("max_pages must not be sent unless set: %v", body)
	}
	if body["capture_mode"] != "content" {
		t.Errorf("capture_mode not sent: %v", body)
	}
	if f := fieldsOf(t, rs.bodies["POST kolektr/extract"])[0].(map[string]interface{}); f["name"] != "title" || f["selector"] != "h3" {
		t.Errorf("the crawl's extract schema must be converted: %v", f)
	}
}

func TestCrawl_RecordsPageAndExtractionFailures(t *testing.T) {
	calls := 0
	_, server := newRouteServer(orchestratorRoutes("completed",
		func(w http.ResponseWriter, r *http.Request) {
			calls++
			if calls == 2 {
				jsonHandler(502, map[string]interface{}{"detail": "upstream timed out"})(w, r)
				return
			}
			jsonHandler(200, fetchResponse)(w, r)
		},
		jsonHandler(500, map[string]interface{}{"detail": "extractor crashed"})))
	defer server.Close()
	result, err := newTestClient(server).Crawl(context.Background(), "https://example.com", &CrawlOrchestratorOptions{
		ExtractFields: []SchemaField{{Name: "t", Selector: "h1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.PagesFetched != 2 || result.PagesFailed != 1 || result.Pages[1].Error == nil {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Pages[0].ExtractionError == nil {
		t.Errorf("the extraction failure must be recorded: %+v", result.Pages[0])
	}
}

func TestCrawl_StopsOnBillingAndAuthErrors(t *testing.T) {
	for name, handler := range map[string]http.HandlerFunc{
		"payment":  jsonHandler(402, detail("payment_required", "pay", nil)),
		"quota":    jsonHandler(429, detail("gb_exhausted", "used up", nil)),
		"billing":  jsonHandler(503, detail("billing_unavailable", "later", nil)),
		"entitled": jsonHandler(403, detail("not_entitled", "no", map[string]interface{}{"module": "evadr"})),
		"auth":     jsonHandler(401, map[string]interface{}{"detail": "bad key"}),
		"rate":     jsonHandler(429, detail("rate_limited", "slow", map[string]interface{}{"retry_after": float64(1)})),
	} {
		calls := 0
		_, server := newRouteServer(orchestratorRoutes("completed", func(w http.ResponseWriter, r *http.Request) {
			calls++
			handler(w, r)
		}, nil))
		_, err := newTestClient(server).Crawl(context.Background(), "https://example.com", nil)
		server.Close()
		if err == nil || calls != 1 {
			t.Errorf("%s: expected the crawl to stop at the first page, got %d calls, %v", name, calls, err)
		}
	}
}

func TestCrawlStream_EndsWithCrawlErrorOnABillingError(t *testing.T) {
	_, server := newRouteServer(orchestratorRoutes("completed", jsonHandler(402, detail("payment_required", "pay", nil)), nil))
	defer server.Close()
	ch, err := newTestClient(server).CrawlStream(context.Background(), "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	events := drain(ch, 2*time.Second)
	last := events[len(events)-1]
	var pe *PaymentRequiredError
	if last.Type != "crawl_error" || !errorsAs(last.Err, &pe) {
		t.Fatalf("unexpected events: %+v", events)
	}
}

func TestCrawlStream_Completes(t *testing.T) {
	_, server := newRouteServer(orchestratorRoutes("completed", jsonHandler(200, fetchResponse), nil))
	defer server.Close()
	ch, err := newTestClient(server).CrawlStream(context.Background(), "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	events := drain(ch, 2*time.Second)
	types := []string{}
	for _, e := range events {
		types = append(types, e.Type)
	}
	if len(events) != 8 || types[0] != "discovery_complete" || types[7] != "crawl_complete" || !*events[7].Success {
		t.Fatalf("unexpected events: %v", types)
	}
	res := events[7].Metadata["result"].(SiteCrawlResult)
	if !res.Success || res.PagesFetched != 3 {
		t.Fatalf("unexpected result: %+v", res)
	}
}
