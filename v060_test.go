package kloakd

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"
)

// ─── Kolektr item_selector (GAP-API-11) ──────────────────────────────────────

var itemRecordsResponse = map[string]interface{}{
	"success": true, "url": "https://books.toscrape.com", "method": "l1_css",
	"records": []interface{}{
		map[string]interface{}{"title": "A Light in the Attic", "price": "£51.77"},
		map[string]interface{}{"title": "Tipping the Velvet", "price": nil},
	},
	"total_records": float64(2), "pages_scraped": float64(1),
}

func schemaSpecOf(t *testing.T, body map[string]interface{}) map[string]interface{} {
	t.Helper()
	spec, ok := body["schema_spec"].(map[string]interface{})
	if !ok {
		t.Fatalf("no schema_spec in %v", body)
	}
	return spec
}

func TestKolektr_Extract_ItemSelectorWithTypedFields(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST kolektr/extract": jsonHandler(200, itemRecordsResponse),
	})
	defer server.Close()

	result, err := newTestClient(server).Kolektr.Extract(context.Background(), "https://books.toscrape.com", &ExtractOptions{
		ItemSelector: "article.product_pod",
		Fields: []SchemaField{
			{Name: "title", Selector: "h3 a", Attribute: "title"},
			{Name: "price", Selector: "css:.price_color"},
			{Name: "tags", Selector: ".tag", Type: "list"},
			{Name: "id", Attribute: "data-id"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalRecords != 2 || len(result.Records) != 2 || result.Records[1]["price"] != nil {
		t.Fatalf("unexpected result: %+v", result)
	}
	spec := schemaSpecOf(t, rs.bodies["POST kolektr/extract"])
	if spec["item_selector"] != "article.product_pod" {
		t.Errorf("item_selector not sent: %v", spec)
	}
	fields := spec["fields"].([]interface{})
	if len(fields) != 4 {
		t.Fatalf("fields: %v", fields)
	}
	title := fields[0].(map[string]interface{})
	if title["selector"] != "h3 a" || title["attribute"] != "title" {
		t.Errorf("title field: %v", title)
	}
	if fields[1].(map[string]interface{})["selector"] != ".price_color" {
		t.Errorf("css: prefix must be stripped: %v", fields[1])
	}
	if fields[2].(map[string]interface{})["type"] != "list" {
		t.Errorf("list type not sent: %v", fields[2])
	}
	id := fields[3].(map[string]interface{})
	if _, ok := id["selector"]; ok || id["attribute"] != "data-id" {
		t.Errorf("a field without selector reads the item element: %v", id)
	}
}

func TestKolektr_Extract_ShorthandCarriesItemSelector(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST kolektr/extract": jsonHandler(200, itemRecordsResponse),
	})
	defer server.Close()

	_, err := newTestClient(server).Kolektr.Extract(context.Background(), "https://books.toscrape.com", &ExtractOptions{
		SchemaSpec: map[string]interface{}{"item_selector": "article.product_pod", "title": "css:h3 a", "price": ".price_color"},
	})
	if err != nil {
		t.Fatal(err)
	}
	spec := schemaSpecOf(t, rs.bodies["POST kolektr/extract"])
	fields := spec["fields"].([]interface{})
	if spec["item_selector"] != "article.product_pod" || len(fields) != 2 {
		t.Fatalf("item_selector must not become a field: %v", spec)
	}
	if f := fields[1].(map[string]interface{}); f["name"] != "title" || f["selector"] != "h3 a" {
		t.Errorf("unexpected field: %v", f)
	}
}

func TestKolektr_Extract_FullSpecItemSelectorAndOverride(t *testing.T) {
	spec := buildItemSchemaSpec(map[string]interface{}{
		"fields":        []interface{}{map[string]interface{}{"name": "t", "selector": "h3"}},
		"item_selector": "li",
	}, nil, "")
	if spec["item_selector"] != "li" {
		t.Errorf("a full spec's item_selector is sent as given: %v", spec)
	}
	spec = buildItemSchemaSpec(map[string]interface{}{"item_selector": "li", "t": "h3"}, nil, "article")
	if spec["item_selector"] != "article" {
		t.Errorf("ItemSelector overrides the spec's: %v", spec)
	}
}

func TestKolektr_Extract_WithoutItemSelectorUnchanged(t *testing.T) {
	spec := buildItemSchemaSpec(map[string]interface{}{"title": "css:h3"}, nil, "")
	if _, ok := spec["item_selector"]; ok {
		t.Errorf("no item_selector unless given: %v", spec)
	}
	if buildItemSchemaSpec(nil, nil, "") != nil {
		t.Error("nothing to send is nil")
	}
}

func TestKolektr_ExtractHTML_ItemSelector(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST kolektr/extract/html": jsonHandler(200, itemRecordsResponse),
	})
	defer server.Close()

	_, err := newTestClient(server).Kolektr.ExtractHTML(context.Background(), "<ul><li>a</li></ul>", "https://example.com",
		&ExtractHTMLOptions{ItemSelector: "li", Fields: []SchemaField{{Name: "text"}}})
	if err != nil {
		t.Fatal(err)
	}
	spec := schemaSpecOf(t, rs.bodies["POST kolektr/extract/html"])
	if spec["item_selector"] != "li" {
		t.Errorf("item_selector not sent: %v", spec)
	}
}

func TestKolektr_Extract_InvalidItemSelectorIs422ApiError(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"POST kolektr/extract/html": jsonHandler(422, map[string]interface{}{"detail": []interface{}{
			map[string]interface{}{"loc": []interface{}{"body", "schema_spec", "item_selector"},
				"msg": "Value error, invalid CSS selector 'a[': Malformed attribute selector"},
		}}),
	})
	defer server.Close()

	_, err := newTestClient(server).Kolektr.ExtractHTML(context.Background(), "<p/>", "https://example.com",
		&ExtractHTMLOptions{ItemSelector: "a[", Fields: []SchemaField{{Name: "x", Selector: "p"}}})
	var ae *ApiError
	if !errors.As(err, &ae) || ae.StatusCode != 422 {
		t.Fatalf("expected a 422 ApiError, got %T %v", err, err)
	}
}

func TestCrawl_ExtractItemSelectorKeepsEveryRecord(t *testing.T) {
	rs, server := newRouteServer(orchestratorRoutes("completed",
		jsonHandler(200, fetchResponse),
		jsonHandler(200, itemRecordsResponse)))
	defer server.Close()
	result, err := newTestClient(server).Crawl(context.Background(), "https://example.com", &CrawlOrchestratorOptions{
		ExtractSchema:       map[string]string{"title": "css:h3 a"},
		ExtractItemSelector: "article.product_pod",
	})
	if err != nil {
		t.Fatal(err)
	}
	page := result.Pages[0]
	if len(page.Records) != 2 || page.StructuredData["title"] != "A Light in the Attic" {
		t.Fatalf("unexpected page: %+v", page)
	}
	if spec := schemaSpecOf(t, rs.bodies["POST kolektr/extract"]); spec["item_selector"] != "article.product_pod" {
		t.Errorf("item_selector not sent: %v", spec)
	}
}

// ─── Webgrph pages (GAP-API-9/10) ────────────────────────────────────────────

func TestWebgrph_ListCrawlPages_IncludeGroupingAndTypedNodes(t *testing.T) {
	var query url.Values
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"GET webgrph/crawl/c1/pages": func(w http.ResponseWriter, r *http.Request) {
			query = r.URL.Query()
			jsonHandler(200, map[string]interface{}{"crawl_id": "c1", "next_cursor": "50", "total": float64(120),
				"nodes": []interface{}{
					map[string]interface{}{"id": "g", "url": "https://e.com#path-catalogue", "depth": float64(1), "category": "page",
						"metadata": map[string]interface{}{"title": "catalogue", "is_grouping": true}},
					map[string]interface{}{"id": "p", "url": "https://e.com/catalogue/a", "parent_url": "https://e.com#path-catalogue",
						"depth": float64(2), "category": "page", "metadata": map[string]interface{}{"title": "A"}},
				}})(w, r)
		},
	})
	defer server.Close()

	res, err := newTestClient(server).Webgrph.ListCrawlPages(context.Background(), "c1",
		&CrawlPagesOptions{Cursor: "0", Limit: 50, IncludeGrouping: true})
	if err != nil {
		t.Fatal(err)
	}
	if query.Get("include_grouping") != "true" || query.Get("limit") != "50" || query.Get("cursor") != "0" {
		t.Errorf("unexpected query: %v", query)
	}
	if res.CrawlID != "c1" || res.NextCursor != "50" || res.Total != 120 || len(res.Nodes) != 2 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if !res.Nodes[0].IsGrouping || res.Nodes[1].IsGrouping || *res.Nodes[1].Title != "A" {
		t.Errorf("unexpected nodes: %+v", res.Nodes)
	}
}

func TestWebgrph_ListCrawlPages_DefaultSendsNoGroupingOrLimit(t *testing.T) {
	var query url.Values
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"GET webgrph/crawl/c1/pages": func(w http.ResponseWriter, r *http.Request) {
			query = r.URL.Query()
			jsonHandler(200, map[string]interface{}{"crawl_id": "c1", "nodes": []interface{}{}, "total": float64(0)})(w, r)
		},
	})
	defer server.Close()
	res, err := newTestClient(server).Webgrph.ListCrawlPages(context.Background(), "c1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(query) != 0 || res.NextCursor != "" {
		t.Errorf("defaults are the API's: %v %+v", query, res)
	}
}

func TestWebgrph_CrawlPagesLimitValidated(t *testing.T) {
	var calls int32
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"GET webgrph/crawl/c1/pages": func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&calls, 1)
			jsonHandler(200, map[string]interface{}{"crawl_id": "c1", "nodes": []interface{}{}, "total": float64(0)})(w, r)
		},
	})
	defer server.Close()
	c := newTestClient(server)
	for _, limit := range []int{-1, 1001} {
		if _, err := c.Webgrph.GetCrawlPages(context.Background(), "c1", "", limit); !errors.Is(err, ErrInvalidPagesLimit) {
			t.Errorf("limit %d: expected ErrInvalidPagesLimit, got %v", limit, err)
		}
		if _, err := c.Webgrph.ListCrawlPages(context.Background(), "c1", &CrawlPagesOptions{Limit: limit}); !errors.Is(err, ErrInvalidPagesLimit) {
			t.Errorf("limit %d: expected ErrInvalidPagesLimit, got %v", limit, err)
		}
	}
	if calls != 0 {
		t.Errorf("no request is sent for a bad limit, got %d", calls)
	}
	for _, limit := range []int{0, 1, 1000} {
		if _, err := c.Webgrph.GetCrawlPages(context.Background(), "c1", "", limit); err != nil {
			t.Errorf("limit %d: %v", limit, err)
		}
	}
}

func TestWebgrph_CrawlPagesBadCursorIs422(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"GET webgrph/crawl/c1/pages": jsonHandler(422, map[string]interface{}{
			"detail": "cursor must be a non-negative integer returned as next_cursor"}),
	})
	defer server.Close()
	_, err := newTestClient(server).Webgrph.ListCrawlPages(context.Background(), "c1", &CrawlPagesOptions{Cursor: "abc"})
	var ae *ApiError
	if !errors.As(err, &ae) || ae.StatusCode != 422 || ae.Message == "" {
		t.Fatalf("expected a 422 ApiError, got %T %v", err, err)
	}
}

func TestWebgrph_Crawl_IncludeGroupingIsSentOnEveryPagesCall(t *testing.T) {
	routes := crawlRoutes([]string{"completed"})
	inner := routes["GET webgrph/crawl/c1/pages"]
	var grouped []string
	routes["GET webgrph/crawl/c1/pages"] = func(w http.ResponseWriter, r *http.Request) {
		grouped = append(grouped, r.URL.Query().Get("include_grouping"))
		inner(w, r)
	}
	rs, server := newRouteServer(routes)
	defer server.Close()
	if _, err := newTestClient(server).Webgrph.Crawl(context.Background(), "https://example.com",
		&CrawlOptions{IncludeGrouping: true}); err != nil {
		t.Fatal(err)
	}
	if len(grouped) != 2 || grouped[0] != "true" || grouped[1] != "true" {
		t.Errorf("include_grouping on every page: %v", grouped)
	}
	if _, ok := rs.bodies["POST webgrph/crawl"]["include_grouping"]; ok {
		t.Error("include_grouping is a pages parameter, not a crawl body field")
	}
}

func TestParsePageNode_StoredRecordGroupingAndCrawlParent(t *testing.T) {
	n := parsePageNode(map[string]interface{}{
		"url": "https://e.com/catalogue/a", "parent_url": "https://e.com#path-catalogue",
		"title": "A", "is_grouping": false, "crawl_parent_url": "https://e.com/",
	})
	if n.CrawlParentURL == nil || *n.CrawlParentURL != "https://e.com/" || n.IsGrouping || n.Title == nil || *n.Title != "A" {
		t.Errorf("unexpected node: %+v", n)
	}
	g := parsePageNode(map[string]interface{}{"url": "https://e.com#path-catalogue", "is_grouping": true})
	if !g.IsGrouping || g.CrawlParentURL != nil {
		t.Errorf("unexpected grouping node: %+v", g)
	}
}

// ─── Work slots (GAP-BILL-13) ────────────────────────────────────────────────

func TestRaiseForStatus_409WorkSlotsCarryActiveRequests(t *testing.T) {
	err := raiseForStatus(409, detail("concurrent_limit",
		"Your free plan runs 2 jobs at once and all 2 are in use (1 crawl(s) or batch(es), 1 other request(s)). Try again when one finishes.",
		map[string]interface{}{"tier": "free", "concurrent_limit": float64(2), "active_crawls": float64(1), "active_requests": float64(1)},
	), nil, nil)
	var ce *ConcurrencyLimitError
	if !errors.As(err, &ce) || ce.Tier != "free" || ce.ConcurrentLimit != 2 || ce.ActiveCrawls != 1 || ce.ActiveRequests != 1 || ce.Code != "concurrent_limit" {
		t.Fatalf("unexpected error: %T %+v", err, err)
	}
}

func TestTransport_409ConcurrentLimitIsNotRetried(t *testing.T) {
	var calls int32
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"POST kolektr/extract": func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&calls, 1)
			jsonHandler(409, map[string]interface{}{"detail": map[string]interface{}{
				"error": "concurrent_limit", "message": "all in use", "tier": "pro",
				"concurrent_limit": float64(10), "active_crawls": float64(4), "active_requests": float64(6)}})(w, r)
		},
		"GET webgrph/crawl/c1": func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&calls, 1)
			jsonHandler(409, map[string]interface{}{"detail": map[string]interface{}{
				"error": "concurrent_limit", "message": "all in use", "concurrent_limit": float64(10)}})(w, r)
		},
	})
	defer server.Close()
	c, err := New(Config{APIKey: testAPIKey, OrganizationID: testOrgID, BaseURL: server.URL, MaxRetries: 3, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	c.Kolektr.t.retryBase = 0
	_, err = c.Kolektr.Extract(context.Background(), "https://example.com", nil)
	var ce *ConcurrencyLimitError
	if !errors.As(err, &ce) || ce.ActiveRequests != 6 || ce.ActiveCrawls != 4 {
		t.Fatalf("unexpected error: %T %v", err, err)
	}
	if _, err := c.Webgrph.GetCrawlStatus(context.Background(), "c1"); !errors.As(err, &ce) {
		t.Fatalf("unexpected error: %T %v", err, err)
	}
	if calls != 2 {
		t.Errorf("a 409 must not be retried (POST or GET): %d calls", calls)
	}
}
