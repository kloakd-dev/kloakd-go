package kloakd

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func crawlRoutes(statuses []string) map[string]http.HandlerFunc {
	calls := 0
	return map[string]http.HandlerFunc{
		"POST webgrph/crawl": jsonHandler(202, map[string]interface{}{
			"success": true, "crawl_id": "c1", "url": "https://example.com", "total_pages": float64(0), "max_depth_reached": float64(0),
		}),
		"GET webgrph/crawl/c1": func(w http.ResponseWriter, r *http.Request) {
			state := statuses[calls]
			if calls < len(statuses)-1 {
				calls++
			}
			jsonHandler(200, map[string]interface{}{
				"crawl_id": "c1", "status": state, "organization_id": testOrgID,
				"total_pages": float64(3), "max_depth_reached": float64(1), "artifact_id": "art-c1",
			})(w, r)
		},
		"GET webgrph/crawl/c1/pages": func(w http.ResponseWriter, r *http.Request) {
			node := func(u string) map[string]interface{} {
				return map[string]interface{}{"id": u, "url": u, "depth": float64(1), "category": "page",
					"children_urls": []interface{}{}, "metadata": map[string]interface{}{"title": "T " + u}}
			}
			if r.URL.Query().Get("cursor") == "" {
				jsonHandler(200, map[string]interface{}{"crawl_id": "c1", "nodes": []interface{}{node("a"), node("b")},
					"next_cursor": "2", "total": float64(3)})(w, r)
				return
			}
			jsonHandler(200, map[string]interface{}{"crawl_id": "c1", "nodes": []interface{}{node("c")}, "total": float64(3)})(w, r)
		},
	}
}

func TestWebgrph_Crawl_WaitsAndCollectsEveryPage(t *testing.T) {
	rs, server := newRouteServer(crawlRoutes([]string{"queued", "crawling", "completed"}))
	defer server.Close()

	result, err := newTestClient(server).Webgrph.Crawl(context.Background(), "https://example.com",
		&CrawlOptions{MaxDepth: 2, MaxPages: 50, PollInterval: time.Millisecond})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success || result.CrawlID != "c1" || len(result.Pages) != 3 || result.Total != 3 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Pages[0].Title == nil || *result.Pages[0].Title != "T a" || *result.ArtifactID != "art-c1" {
		t.Errorf("unexpected page: %+v", result.Pages[0])
	}
	if rs.bodies["POST webgrph/crawl"]["max_pages"] != float64(50) {
		t.Errorf("unexpected body: %v", rs.bodies["POST webgrph/crawl"])
	}
}

func TestWebgrph_Crawl_Failed(t *testing.T) {
	_, server := newRouteServer(crawlRoutes([]string{"failed"}))
	defer server.Close()

	result, err := newTestClient(server).Webgrph.Crawl(context.Background(), "https://example.com",
		&CrawlOptions{PollInterval: time.Millisecond})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success || result.Status != "failed" || len(result.Pages) != 0 {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestWebgrph_Crawl_TimesOut(t *testing.T) {
	_, server := newRouteServer(crawlRoutes([]string{"crawling"}))
	defer server.Close()

	_, err := newTestClient(server).Webgrph.Crawl(context.Background(), "https://example.com",
		&CrawlOptions{PollInterval: time.Millisecond, PollTimeout: 20 * time.Millisecond})
	var apiErr *ApiError
	if !errorsAs(err, &apiErr) || apiErr.Code != "crawl_timeout" || apiErr.JobID != "c1" {
		t.Fatalf("expected crawl_timeout, got %v", err)
	}
}

func TestWebgrph_CrawlStream(t *testing.T) {
	routes := crawlRoutes([]string{"completed"})
	routes["GET webgrph/crawl/c1/events"] = sseHandler([]map[string]interface{}{
		{"pipeline_id": "c1", "event_type": "stage_progress", "stage_id": "discovery", "status": "running",
			"message": "12 pages", "progress_pct": float64(40), "metadata": map[string]interface{}{"pages": float64(12)}, "sequence": float64(1)},
		{"event_type": "pipeline_complete", "sequence": float64(2), "crawl_id": "c1", "metadata": map[string]interface{}{}},
	})
	_, server := newRouteServer(routes)
	defer server.Close()

	ch, err := newTestClient(server).Webgrph.CrawlStream(context.Background(), "https://example.com", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	events := drain(ch, 2*time.Second)
	if len(events) != 2 || events[0].EventType != "stage_progress" || events[0].StageID != "discovery" ||
		events[0].ProgressPct == nil || *events[0].ProgressPct != 40 || events[0].Sequence != 1 ||
		events[1].EventType != "pipeline_complete" || events[1].Err != nil {
		t.Fatalf("unexpected events: %+v", events)
	}
}

func TestWebgrph_SessionsMovedFromSkanyr(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"GET webgrph/sessions":          jsonHandler(200, []interface{}{map[string]interface{}{"id": "s1"}}),
		"PATCH webgrph/sessions/s1/job": jsonHandler(200, map[string]interface{}{"id": "s1", "last_job_id": "j1"}),
	})
	defer server.Close()
	client := newTestClient(server)

	list, err := client.Webgrph.ListSessions(context.Background(), true)
	if err != nil || len(list["items"].([]interface{})) != 1 {
		t.Fatalf("sessions: %v %v", list, err)
	}
	updated, err := client.Webgrph.UpdateSessionJob(context.Background(), "s1", "j1")
	if err != nil || updated["last_job_id"] != "j1" {
		t.Fatalf("update: %v %v", updated, err)
	}
	if len(rs.seen) != 2 {
		t.Errorf("unexpected calls: %v", rs.seen)
	}
}

func TestWebgrph_ListSessions_AlwaysSendsActiveOnly(t *testing.T) {
	var queries []string
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"GET webgrph/sessions": func(w http.ResponseWriter, r *http.Request) {
			queries = append(queries, r.URL.RawQuery)
			jsonHandler(200, []interface{}{})(w, r)
		},
	})
	defer server.Close()
	client := newTestClient(server)
	_, _ = client.Webgrph.ListSessions(context.Background(), true)
	_, _ = client.Webgrph.ListSessions(context.Background(), false)
	if len(queries) != 2 || queries[0] != "active_only=true" || queries[1] != "active_only=false" {
		t.Fatalf("unexpected queries: %v", queries)
	}
}

func TestWebgrph_CrawlPolicyAndReaderView(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST webgrph/crawl":       jsonHandler(202, map[string]interface{}{"success": true, "crawl_id": "c1"}),
		"POST webgrph/reader-view": jsonHandler(200, map[string]interface{}{"title": "T"}),
	})
	defer server.Close()
	client := newTestClient(server)
	_, err := client.Webgrph.StartCrawl(context.Background(), "https://example.com", &CrawlOptions{
		CaptureMode: "content",
		Scope:       map[string]interface{}{"same_domain": true},
		Budget:      map[string]interface{}{"max_resources": float64(10)},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := rs.bodies["POST webgrph/crawl"]
	if body["capture_mode"] != "content" || body["scope"].(map[string]interface{})["same_domain"] != true ||
		body["budget"].(map[string]interface{})["max_resources"] != float64(10) {
		t.Errorf("unexpected crawl body: %v", body)
	}
	if _, ok := body["max_depth"]; ok {
		t.Error("max_depth must not be sent when unset")
	}
	if _, err := client.Webgrph.ReaderView(context.Background(), "https://example.com/a",
		&ReaderViewOptions{CrawlID: "c1", SessionArtifactID: "sess-1"}); err != nil {
		t.Fatal(err)
	}
	rv := rs.bodies["POST webgrph/reader-view"]
	if rv["crawl_id"] != "c1" || rv["session_artifact_id"] != "sess-1" {
		t.Errorf("unexpected reader-view body: %v", rv)
	}
}
