package kloakd

import (
	"context"
	"net/http/httptest"
	"testing"
)

var sessionResponse = map[string]interface{}{
	"success":        true,
	"session_id":     "sess-001",
	"url":            "https://example.com/dashboard",
	"artifact_id":    "art-sess-001",
	"screenshot_url": "https://cdn.kloakd.dev/screenshot.png",
	"error":          nil,
}

func TestFetchyr_Login(t *testing.T) {
	server := httptest.NewServer(jsonHandler(200, sessionResponse))
	defer server.Close()

	client := newTestClient(server)
	result, err := client.Fetchyr.Login(
		context.Background(),
		"https://example.com/login",
		"#username", "#password",
		"user@example.com", "secret",
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected Success=true")
	}
	if result.SessionID != "sess-001" {
		t.Errorf("expected sess-001, got %s", result.SessionID)
	}
	if result.ArtifactID == nil || *result.ArtifactID != "art-sess-001" {
		t.Errorf("unexpected ArtifactID: %v", result.ArtifactID)
	}
}

func TestFetchyr_Login_WithOptions(t *testing.T) {
	cap := &captureHandler{inner: jsonHandler(200, sessionResponse)}
	server := httptest.NewServer(cap)
	defer server.Close()

	client := newTestClient(server)
	_, err := client.Fetchyr.Login(
		context.Background(),
		"https://example.com/login",
		"#username", "#password",
		"user@example.com", "secret",
		&LoginOptions{SubmitSelector: "#submit", SuccessURLContains: "/dashboard"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cap.lastBody["submit_selector"] != "#submit" {
		t.Errorf("unexpected submit_selector: %v", cap.lastBody["submit_selector"])
	}
}

func TestFetchyr_Fetch(t *testing.T) {
	server := httptest.NewServer(jsonHandler(200, map[string]interface{}{
		"success":             true,
		"url":                 "https://example.com/protected",
		"status_code":         float64(200),
		"html":                "<html>Protected page</html>",
		"artifact_id":         "art-fetchyr-001",
		"session_artifact_id": "art-sess-001",
	}))
	defer server.Close()

	client := newTestClient(server)
	result, err := client.Fetchyr.Fetch(context.Background(), "https://example.com/protected", "art-sess-001", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.StatusCode != 200 {
		t.Errorf("expected 200, got %d", result.StatusCode)
	}
	if result.SessionArtifactID != "art-sess-001" {
		t.Errorf("unexpected session_artifact_id: %s", result.SessionArtifactID)
	}
}

func TestFetchyr_CreateWorkflow(t *testing.T) {
	server := httptest.NewServer(jsonHandler(200, map[string]interface{}{
		"workflow_id": "wf-001",
		"name":        "login_flow",
		"steps":       []interface{}{map[string]interface{}{"action": "click", "selector": "#btn"}},
		"url":         "https://example.com",
		"created_at":  "2026-04-09T00:00:00Z",
	}))
	defer server.Close()

	client := newTestClient(server)
	result, err := client.Fetchyr.CreateWorkflow(
		context.Background(),
		"login_flow",
		[]map[string]interface{}{{"action": "click", "selector": "#btn"}},
		"https://example.com",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.WorkflowID != "wf-001" {
		t.Errorf("expected wf-001, got %s", result.WorkflowID)
	}
}

func TestFetchyr_ExecuteWorkflow(t *testing.T) {
	server := httptest.NewServer(jsonHandler(200, map[string]interface{}{
		"execution_id": "exec-001",
		"workflow_id":  "wf-001",
		"status":       "running",
		"records":      []interface{}{},
	}))
	defer server.Close()

	client := newTestClient(server)
	result, err := client.Fetchyr.ExecuteWorkflow(context.Background(), "wf-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "running" {
		t.Errorf("expected running, got %s", result.Status)
	}
}

func TestFetchyr_GetExecution(t *testing.T) {
	server := httptest.NewServer(jsonHandler(200, map[string]interface{}{
		"execution_id": "exec-001",
		"workflow_id":  "wf-001",
		"status":       "completed",
		"records":      []interface{}{map[string]interface{}{"data": "value"}},
	}))
	defer server.Close()

	client := newTestClient(server)
	result, err := client.Fetchyr.GetExecution(context.Background(), "wf-001", "exec-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "completed" {
		t.Errorf("expected completed, got %s", result.Status)
	}
}

func TestFetchyr_DetectForms(t *testing.T) {
	server := httptest.NewServer(jsonHandler(200, map[string]interface{}{
		"forms": []interface{}{
			map[string]interface{}{"selector": "form#login", "confidence": float64(0.99)},
		},
		"total_forms": float64(1),
	}))
	defer server.Close()

	client := newTestClient(server)
	result, err := client.Fetchyr.DetectForms(context.Background(), "https://example.com/login", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalForms != 1 {
		t.Errorf("expected 1 form, got %d", result.TotalForms)
	}
}

func TestFetchyr_DetectMFA(t *testing.T) {
	server := httptest.NewServer(jsonHandler(200, map[string]interface{}{
		"mfa_detected": true,
		"challenge_id": "chall-001",
		"mfa_type":     "totp",
	}))
	defer server.Close()

	client := newTestClient(server)
	result, err := client.Fetchyr.DetectMFA(context.Background(), "https://example.com/mfa", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.MfaDetected {
		t.Error("expected MfaDetected=true")
	}
	if result.MfaType == nil || *result.MfaType != "totp" {
		t.Errorf("unexpected MfaType: %v", result.MfaType)
	}
}

func TestFetchyr_SubmitMFA(t *testing.T) {
	server := httptest.NewServer(jsonHandler(200, map[string]interface{}{
		"success":             true,
		"session_artifact_id": "art-sess-002",
	}))
	defer server.Close()

	client := newTestClient(server)
	result, err := client.Fetchyr.SubmitMFA(context.Background(), "chall-001", "123456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected Success=true")
	}
	if result.SessionArtifactID == nil || *result.SessionArtifactID != "art-sess-002" {
		t.Errorf("unexpected SessionArtifactID: %v", result.SessionArtifactID)
	}
}

func TestFetchyr_CheckDuplicates(t *testing.T) {
	server := httptest.NewServer(jsonHandler(200, map[string]interface{}{
		"unique_records":  []interface{}{map[string]interface{}{"id": "1"}},
		"duplicate_count": float64(2),
		"total_input":     float64(3),
	}))
	defer server.Close()

	client := newTestClient(server)
	records := []map[string]interface{}{
		{"id": "1"}, {"id": "2"}, {"id": "2"},
	}
	result, err := client.Fetchyr.CheckDuplicates(context.Background(), records, "example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.DuplicateCount != 2 {
		t.Errorf("expected 2 duplicates, got %d", result.DuplicateCount)
	}
	if result.TotalInput != 3 {
		t.Errorf("expected 3 total, got %d", result.TotalInput)
	}
}
