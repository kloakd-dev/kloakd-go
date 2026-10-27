package kloakd

import (
	"context"
	"net/http"
	"testing"
)

func TestSkanyr_SitesSyncSearch(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"GET skanyr/sites":       jsonHandler(200, []interface{}{map[string]interface{}{"site_id": "s1"}}),
		"POST skanyr/sync":       jsonHandler(200, map[string]interface{}{"site_id": "s1", "sync_id": "y1", "policy": "changed"}),
		"POST skanyr/search":     jsonHandler(200, map[string]interface{}{"results": []interface{}{}}),
		"PATCH skanyr/sites/s1":  jsonHandler(200, map[string]interface{}{"site_id": "s1", "policy": "full"}),
		"DELETE skanyr/sites/s1": jsonHandler(200, map[string]interface{}{"site_id": "s1", "deleted": true}),
	})
	defer server.Close()
	client := newTestClient(server)
	ctx := context.Background()

	sites, err := client.Skanyr.ListSites(ctx)
	if err != nil || len(sites["items"].([]interface{})) != 1 {
		t.Fatalf("sites: %v %v", sites, err)
	}
	sync, err := client.Skanyr.Sync(ctx, "s1", &SyncOptions{Policy: "changed"})
	if err != nil || sync["sync_id"] != "y1" {
		t.Fatalf("sync: %v %v", sync, err)
	}
	if _, err := client.Skanyr.Search(ctx, "pricing", &SearchOptions{SiteID: "s1", Limit: 5}); err != nil {
		t.Fatalf("search: %v", err)
	}
	if _, err := client.Skanyr.SetSitePolicy(ctx, "s1", "full"); err != nil {
		t.Fatalf("policy: %v", err)
	}
	if err := client.Skanyr.DeleteSite(ctx, "s1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if rs.bodies["POST skanyr/search"]["site_id"] != "s1" || rs.bodies["POST skanyr/sync"]["policy"] != "changed" {
		t.Errorf("unexpected bodies: %v", rs.bodies)
	}
}
