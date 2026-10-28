package kloakd

import (
	"context"
	"net/http"
	"testing"
)

func TestSkanyr_SitesSyncSearch(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"GET skanyr/sites":       jsonHandler(200, []interface{}{map[string]interface{}{"id": "s1"}}),
		"POST skanyr/sync":       jsonHandler(200, map[string]interface{}{"site_id": "s1", "sync_id": "y1", "policy": "incremental"}),
		"POST skanyr/search":     jsonHandler(200, map[string]interface{}{"results": []interface{}{}}),
		"PATCH skanyr/sites/s1":  jsonHandler(200, map[string]interface{}{"id": "s1", "policy": "full"}),
		"DELETE skanyr/sites/s1": jsonHandler(200, map[string]interface{}{"site_id": "s1", "deleted": true}),
	})
	defer server.Close()
	client := newTestClient(server)
	ctx := context.Background()

	sites, err := client.Skanyr.ListSites(ctx)
	if err != nil || len(sites) != 1 || sites[0]["id"] != "s1" {
		t.Fatalf("sites: %v %v", sites, err)
	}
	sync, err := client.Skanyr.Sync(ctx, "s1", &SyncOptions{Policy: "incremental"})
	if err != nil || sync["sync_id"] != "y1" {
		t.Fatalf("sync: %v %v", sync, err)
	}
	if _, err := client.Skanyr.Search(ctx, "pricing", &SearchOptions{SiteID: "s1", Limit: 5, MimeType: "application/pdf"}); err != nil {
		t.Fatalf("search: %v", err)
	}
	if _, err := client.Skanyr.UpdateSite(ctx, "s1", "full"); err != nil {
		t.Fatalf("update: %v", err)
	}
	if rs.bodies["PATCH skanyr/sites/s1"]["policy"] != "full" {
		t.Errorf("UpdateSite must send {policy}, got %v", rs.bodies["PATCH skanyr/sites/s1"])
	}
	if err := client.Skanyr.DeleteSite(ctx, "s1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if rs.bodies["POST skanyr/search"]["site_id"] != "s1" || rs.bodies["POST skanyr/search"]["mime_type"] != "application/pdf" ||
		rs.bodies["POST skanyr/sync"]["policy"] != "incremental" {
		t.Errorf("unexpected bodies: %v", rs.bodies)
	}
}

// Every Skanyr read the 0.4.0 JS and Python SDKs have, against its route,
// query and answer shape (modules/skanyr/router.py).
func TestSkanyr_ParityRoutes(t *testing.T) {
	var queries = map[string]string{}
	list := func(w http.ResponseWriter, r *http.Request) {
		queries[r.URL.Path] = r.URL.RawQuery
		jsonHandler(200, []interface{}{map[string]interface{}{"id": "x"}})(w, r)
	}
	obj := func(w http.ResponseWriter, r *http.Request) {
		queries[r.URL.Path] = r.URL.RawQuery
		jsonHandler(200, map[string]interface{}{"ok": true})(w, r)
	}
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"GET skanyr/resources":                    list,
		"GET skanyr/resources/r1":                 obj,
		"GET skanyr/sites/s1/resources":           list,
		"GET skanyr/sites/s1/apis":                list,
		"GET skanyr/sites/s1/findings":            list,
		"GET skanyr/sites/s1/technologies":        list,
		"GET skanyr/sites/s1/technology-evidence": obj,
		"GET skanyr/sites/s1/search":              obj,
		"GET skanyr/sites/s1/snapshots":           obj,
		"GET skanyr/sites/s1/timeline":            obj,
		"GET skanyr/sites/s1/timeline/snap1":      obj,
		"GET skanyr/sites/s1/diff":                obj,
		"GET skanyr/sites/s1/graph":               obj,
		"GET skanyr/sites/s1/graph/stats":         obj,
		"GET skanyr/sites/s1/graph/subgraph/r1":   obj,
		"GET skanyr/extractions/h1/structure":     obj,
		"GET skanyr/extractions/h1/text": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("plain text"))
		},
		"POST skanyr/sites/s1/export": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write([]byte("PK\x03\x04zip"))
		},
	})
	defer server.Close()
	sk := newTestClient(server).Skanyr
	ctx := context.Background()
	prefix := "/api/v1/organizations/" + testOrgID + "/"
	q := func(path string) string { return queries[prefix+path] }

	lists := []func() ([]map[string]interface{}, error){
		func() ([]map[string]interface{}, error) {
			return sk.ListResources(ctx, &ResourcesOptions{SiteID: "s1", ResourceType: "page", Limit: 10, Offset: 20})
		},
		func() ([]map[string]interface{}, error) {
			return sk.SiteResources(ctx, "s1", &ResourceListOptions{ResourceType: "pdf", Limit: 5})
		},
		func() ([]map[string]interface{}, error) { return sk.SiteAPIs(ctx, "s1", &ListPage{Limit: 7}) },
		func() ([]map[string]interface{}, error) {
			return sk.GetFindings(ctx, "s1", &FindingsOptions{FindingKind: "technology", Offset: 3})
		},
		func() ([]map[string]interface{}, error) { return sk.GetTechnologies(ctx, "s1") },
	}
	for i, call := range lists {
		items, err := call()
		if err != nil || len(items) != 1 || items[0]["id"] != "x" {
			t.Errorf("list %d: %v %v", i, items, err)
		}
	}
	objs := []func() (map[string]interface{}, error){
		func() (map[string]interface{}, error) { return sk.GetResource(ctx, "r1") },
		func() (map[string]interface{}, error) {
			return sk.GetTechnologyEvidence(ctx, "s1", "React", &TechnologyEvidenceOptions{Detector: "wappalyzer"})
		},
		func() (map[string]interface{}, error) {
			return sk.SearchSite(ctx, "s1", "price", &SiteSearchOptions{MimeType: "text/html", Limit: 2})
		},
		func() (map[string]interface{}, error) { return sk.ListSnapshots(ctx, "s1", 4) },
		func() (map[string]interface{}, error) { return sk.GetTimeline(ctx, "s1", 0) },
		func() (map[string]interface{}, error) { return sk.GetTimelineSnapshot(ctx, "s1", "snap1") },
		func() (map[string]interface{}, error) { return sk.Diff(ctx, "s1", "a", "b") },
		func() (map[string]interface{}, error) {
			return sk.GetGraph(ctx, "s1", &GraphOptions{ResourceType: "page", MaxDepth: 2, EdgeKind: "link", Limit: 9})
		},
		func() (map[string]interface{}, error) { return sk.GetGraphStats(ctx, "s1") },
		func() (map[string]interface{}, error) { return sk.GetSubgraph(ctx, "s1", "r1", 3) },
		func() (map[string]interface{}, error) { return sk.GetExtractionStructure(ctx, "h1") },
	}
	for i, call := range objs {
		if m, err := call(); err != nil || m["ok"] != true {
			t.Errorf("call %d: %v %v", i, m, err)
		}
	}
	text, err := sk.GetExtractionText(ctx, "h1")
	if err != nil || text != "plain text" {
		t.Errorf("text: %q %v", text, err)
	}
	zip, err := sk.ExportSite(ctx, "s1", &ExportOptions{ResourceIDs: []string{"r1"}, MaxBytes: 1024})
	if err != nil || string(zip) != "PK\x03\x04zip" {
		t.Errorf("export: %q %v", zip, err)
	}

	for path, want := range map[string]string{
		"skanyr/resources":                    "limit=10&offset=20&resource_type=page&site_id=s1",
		"skanyr/sites/s1/resources":           "limit=5&resource_type=pdf",
		"skanyr/sites/s1/apis":                "limit=7",
		"skanyr/sites/s1/findings":            "finding_kind=technology&offset=3",
		"skanyr/sites/s1/technology-evidence": "detector=wappalyzer&subject=React",
		"skanyr/sites/s1/search":              "limit=2&mime_type=text%2Fhtml&q=price",
		"skanyr/sites/s1/snapshots":           "limit=4",
		"skanyr/sites/s1/timeline":            "",
		"skanyr/sites/s1/diff":                "snapshot_a=a&snapshot_b=b",
		"skanyr/sites/s1/graph":               "edge_kind=link&limit=9&max_depth=2&resource_type=page",
		"skanyr/sites/s1/graph/subgraph/r1":   "hops=3",
	} {
		if got := q(path); got != want {
			t.Errorf("%s: query %q, want %q", path, got, want)
		}
	}
}
