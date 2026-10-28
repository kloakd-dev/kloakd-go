package kloakd

import (
	"context"
	"net/http"
	"testing"
)

func TestFetchyr_Authenticate(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST fetchyr/authenticate": jsonHandler(200, map[string]interface{}{
			"success": true, "session_id": "s1", "artifact_id": "art-1",
			"target_domain": "example.com", "auth_method": "form",
		}),
	})
	defer server.Close()

	result, err := newTestClient(server).Fetchyr.Authenticate(context.Background(), "my-login", "https://example.com/login")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success || result.SessionID != "s1" || *result.ArtifactID != "art-1" || *result.AuthMethod != "form" {
		t.Errorf("unexpected result: %+v", result)
	}
	body := rs.bodies["POST fetchyr/authenticate"]
	if body["credential_name"] != "my-login" || body["target_url"] != "https://example.com/login" {
		t.Errorf("unexpected body: %v", body)
	}
}

func TestFetchyr_CreateWorkflow(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST fetchyr/workflows": jsonHandler(201, map[string]interface{}{
			"id": "wf-1", "name": "Login", "workflow_type": "form", "steps": []interface{}{},
			"is_active": true, "created_at": "2026-10-27T00:00:00Z",
		}),
	})
	defer server.Close()

	result, err := newTestClient(server).Fetchyr.CreateWorkflow(context.Background(), "Login",
		[]map[string]interface{}{{"action": "click"}}, &CreateWorkflowOptions{WorkflowType: "form"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.WorkflowID != "wf-1" || !result.IsActive {
		t.Errorf("unexpected result: %+v", result)
	}
	if _, ok := rs.bodies["POST fetchyr/workflows"]["url"]; ok {
		t.Error("url is not a field of the workflow request")
	}
}

func TestFetchyr_ExecuteWorkflow_SendsTargetURL(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST fetchyr/workflows/wf-1/execute": jsonHandler(202, map[string]interface{}{
			"execution_id": "ex-1", "workflow_id": "wf-1", "status": "running",
			"started_at": "2026-10-27T00:00:00Z", "result": map[string]interface{}{"k": "v"},
		}),
	})
	defer server.Close()

	result, err := newTestClient(server).Fetchyr.ExecuteWorkflow(context.Background(), "wf-1", "https://example.com", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExecutionID != "ex-1" || result.Result["k"] != "v" {
		t.Errorf("unexpected result: %+v", result)
	}
	if rs.bodies["POST fetchyr/workflows/wf-1/execute"]["target_url"] != "https://example.com" {
		t.Error("target_url not sent")
	}
}

func TestFetchyr_DetectForms(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"POST fetchyr/form/detect": jsonHandler(200, map[string]interface{}{
			"url": "https://example.com", "forms_found": float64(2), "login_form_detected": true,
			"fields": []interface{}{map[string]interface{}{"name": "user"}},
		}),
	})
	defer server.Close()

	result, err := newTestClient(server).Fetchyr.DetectForms(context.Background(), "https://example.com", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.FormsFound != 2 || !result.LoginFormDetected || len(result.Fields) != 1 {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestFetchyr_ResolveMFA(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST fetchyr/mfa-queue/ch-1/resolve": jsonHandler(200, map[string]interface{}{"id": "ch-1", "status": "resolved"}),
	})
	defer server.Close()

	result, err := newTestClient(server).Fetchyr.ResolveMFA(context.Background(), "ch-1", "123456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["status"] != "resolved" || rs.bodies["POST fetchyr/mfa-queue/ch-1/resolve"]["resolution"] != "123456" {
		t.Errorf("unexpected: %v %v", result, rs.bodies)
	}
}

func TestFetchyr_CheckDuplicate(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST fetchyr/deduplication/check": jsonHandler(200, map[string]interface{}{
			"is_duplicate": true, "original_url": "https://example.com/a", "similarity_score": 0.98,
		}),
	})
	defer server.Close()

	result, err := newTestClient(server).Fetchyr.CheckDuplicate(context.Background(), "https://example.com/b",
		&CheckDuplicateOptions{SessionID: "d1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsDuplicate || *result.OriginalURL != "https://example.com/a" || result.SimilarityScore != 0.98 {
		t.Errorf("unexpected result: %+v", result)
	}
	if rs.bodies["POST fetchyr/deduplication/check"]["session_id"] != "d1" {
		t.Error("session_id not sent")
	}
}

func TestFetchyr_ListMFAChallenges_ArrayBody(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"GET fetchyr/mfa-queue": jsonHandler(200, []interface{}{map[string]interface{}{"id": "ch-1"}}),
	})
	defer server.Close()

	result, err := newTestClient(server).Fetchyr.ListMFAChallenges(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	items, _ := result["items"].([]interface{})
	if len(items) != 1 {
		t.Errorf("expected the array under items, got %v", result)
	}
}

func TestFetchyr_MultiSiteWorkflowDependencies(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST fetchyr/multi-site-workflows": jsonHandler(201, map[string]interface{}{"id": "w1"}),
	})
	defer server.Close()
	sites := []map[string]interface{}{
		{"site_id": "login", "name": "Login", "url": "https://a.example.com"},
		{"site_id": "data", "name": "Data", "url": "https://b.example.com"},
	}
	_, err := newTestClient(server).Fetchyr.CreateMultiSiteWorkflow(context.Background(), "two sites", sites, &MultiSiteWorkflowOptions{
		Description:  "login then data",
		Dependencies: []SiteDependency{{DependentSite: "data", DependencySite: "login", DependencyType: "data", DataRequirements: []string{"token"}, TimeoutSecs: 60}},
		GlobalConfig: map[string]interface{}{"parallel": false},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := rs.bodies["POST fetchyr/multi-site-workflows"]
	dep := body["dependencies"].([]interface{})[0].(map[string]interface{})
	if dep["dependent_site"] != "data" || dep["dependency_site"] != "login" || dep["dependency_type"] != "data" ||
		dep["timeout"] != float64(60) || dep["data_requirements"].([]interface{})[0] != "token" {
		t.Errorf("unexpected dependency: %v", dep)
	}
	if body["description"] != "login then data" || body["global_config"].(map[string]interface{})["parallel"] != false {
		t.Errorf("unexpected body: %v", body)
	}
}

func TestFetchyr_AuthChainResume(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST fetchyr/auth-chain/d1/resume-credentials": jsonHandler(200, map[string]interface{}{"status": "resumed"}),
		"POST fetchyr/auth-chain/d1/resume-mfa":         jsonHandler(200, map[string]interface{}{"status": "resumed", "discovery_id": "d1"}),
	})
	defer server.Close()
	f := newTestClient(server).Fetchyr
	if _, err := f.ResumeAuthChainCredentials(context.Background(), "d1", "user", "test-password", true); err != nil {
		t.Fatal(err)
	}
	if r, err := f.ResumeAuthChainMFA(context.Background(), "d1", "123456"); err != nil || r["status"] != "resumed" {
		t.Fatal(r, err)
	}
	c := rs.bodies["POST fetchyr/auth-chain/d1/resume-credentials"]
	if c["username"] != "user" || c["password"] != "test-password" || c["remember"] != true {
		t.Errorf("unexpected credentials body: %v", c)
	}
	if rs.bodies["POST fetchyr/auth-chain/d1/resume-mfa"]["mfa_code"] != "123456" {
		t.Errorf("unexpected mfa body: %v", rs.bodies["POST fetchyr/auth-chain/d1/resume-mfa"])
	}
}

func TestFetchyr_ListWorkflowsPagingAndStoreCredentialsCopies(t *testing.T) {
	var query string
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"GET fetchyr/workflows": func(w http.ResponseWriter, r *http.Request) {
			query = r.URL.RawQuery
			jsonHandler(200, map[string]interface{}{"workflows": []interface{}{}})(w, r)
		},
		"POST fetchyr/account/credentials": jsonHandler(201, map[string]interface{}{"name": "n"}),
	})
	defer server.Close()
	f := newTestClient(server).Fetchyr
	if _, err := f.ListWorkflows(context.Background(), &ListWorkflowsOptions{Page: 2, PageSize: 25}); err != nil || query != "page=2&page_size=25" {
		t.Fatalf("unexpected query %q (%v)", query, err)
	}
	creds := map[string]interface{}{"username": "u"}
	if _, err := f.StoreCredentials(context.Background(), "n", creds); err != nil {
		t.Fatal(err)
	}
	if _, ok := creds["name"]; ok || rs.bodies["POST fetchyr/account/credentials"]["name"] != "n" {
		t.Error("StoreCredentials must send name without changing the caller's map")
	}
}
