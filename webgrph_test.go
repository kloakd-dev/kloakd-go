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
	if !errorsAs(err, &apiErr) || apiErr.Code != "crawl_timeout" {
		t.Fatalf("expected crawl_timeout, got %v", err)
	}
}

func TestWebgrph_CrawlStream(t *testing.T) {
	routes := crawlRoutes([]string{"completed"})
	routes["GET webgrph/crawl/c1/events"] = sseHandler([]map[string]interface{}{
		{"type": "page_discovered", "url": "https://example.com/a", "depth": float64(1), "metadata": map[string]interface{}{}},
		{"type": "done", "metadata": map[string]interface{}{}},
	})
	_, server := newRouteServer(routes)
	defer server.Close()

	ch, err := newTestClient(server).Webgrph.CrawlStream(context.Background(), "https://example.com", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	events := drain(ch, 2*time.Second)
	if len(events) != 2 || events[0].Type != "page_discovered" {
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
