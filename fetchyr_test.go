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
