package kloakd

import (
	"context"
	"fmt"
)

// FetchyrNamespace exposes the RPA & Authentication module.
// Access via client.Fetchyr.
type FetchyrNamespace struct {
	t *transport
}

// LoginOptions configures optional parameters for Fetchyr.Login.
type LoginOptions struct {
	SubmitSelector    string
	SuccessURLContains string
}

// Login performs an automated login and returns an authenticated session artifact.
func (f *FetchyrNamespace) Login(
	ctx context.Context,
	targetURL, usernameSelector, passwordSelector, username, password string,
	opts *LoginOptions,
) (*SessionResult, error) {
	body := map[string]interface{}{
		"url":               targetURL,
		"username_selector": usernameSelector,
		"password_selector": passwordSelector,
		"username":          username,
		"password":          password,
	}
	if opts != nil {
		if opts.SubmitSelector != "" {
			body["submit_selector"] = opts.SubmitSelector
		}
		if opts.SuccessURLContains != "" {
			body["success_url_contains"] = opts.SuccessURLContains
		}
	}

	raw, err := f.t.post(ctx, "fetchyr/login", body)
	if err != nil {
		return nil, err
	}
	return parseSessionResult(raw), nil
}

// FetchyrFetchOptions configures optional parameters for Fetchyr.Fetch.
type FetchyrFetchOptions struct {
	WaitForSelector string
	ExtractHTML     bool
}

// Fetch performs an authenticated fetch within an existing session.
func (f *FetchyrNamespace) Fetch(
	ctx context.Context,
	targetURL, sessionArtifactID string,
	opts *FetchyrFetchOptions,
) (*FetchyrResult, error) {
	body := map[string]interface{}{
		"url":                 targetURL,
		"session_artifact_id": sessionArtifactID,
	}
	if opts != nil {
		if opts.WaitForSelector != "" {
			body["wait_for_selector"] = opts.WaitForSelector
		}
		if opts.ExtractHTML {
			body["extract_html"] = true
		}
	}

	raw, err := f.t.post(ctx, "fetchyr/fetch", body)
	if err != nil {
		return nil, err
	}
	return parseFetchyrResult(raw), nil
}

// CreateWorkflow creates a new RPA workflow.
func (f *FetchyrNamespace) CreateWorkflow(ctx context.Context, name string, steps []map[string]interface{}, targetURL string) (*WorkflowResult, error) {
	body := map[string]interface{}{
		"name":  name,
		"steps": steps,
	}
	if targetURL != "" {
		body["url"] = targetURL
	}

	raw, err := f.t.post(ctx, "fetchyr/workflows", body)
	if err != nil {
		return nil, err
	}
	return parseWorkflowResult(raw), nil
}

// ExecuteWorkflow triggers execution of a workflow by ID.
func (f *FetchyrNamespace) ExecuteWorkflow(ctx context.Context, workflowID string) (*ExecutionResult, error) {
	raw, err := f.t.post(ctx, fmt.Sprintf("fetchyr/workflows/%s/execute", workflowID), map[string]interface{}{})
	if err != nil {
		return nil, err
	}
	return parseExecutionResult(raw), nil
}

// GetExecution retrieves the result of a workflow execution.
func (f *FetchyrNamespace) GetExecution(ctx context.Context, workflowID, executionID string) (*ExecutionResult, error) {
	raw, err := f.t.get(ctx, fmt.Sprintf("fetchyr/workflows/%s/executions/%s", workflowID, executionID), nil)
	if err != nil {
		return nil, err
	}
	return parseExecutionResult(raw), nil
}

// DetectForms detects forms on a page.
func (f *FetchyrNamespace) DetectForms(ctx context.Context, targetURL string, sessionArtifactID string) (*FormDetectionResult, error) {
	body := map[string]interface{}{"url": targetURL}
	if sessionArtifactID != "" {
		body["session_artifact_id"] = sessionArtifactID
	}

	raw, err := f.t.post(ctx, "fetchyr/forms/detect", body)
	if err != nil {
		return nil, err
	}
	return parseFormDetectionResult(raw), nil
}

// DetectMFA detects MFA challenges on a page.
func (f *FetchyrNamespace) DetectMFA(ctx context.Context, targetURL string, sessionArtifactID string) (*MfaDetectionResult, error) {
	body := map[string]interface{}{"url": targetURL}
	if sessionArtifactID != "" {
		body["session_artifact_id"] = sessionArtifactID
	}

	raw, err := f.t.post(ctx, "fetchyr/mfa/detect", body)
	if err != nil {
		return nil, err
	}
	return parseMfaDetectionResult(raw), nil
}

// SubmitMFA submits an MFA code for the given challenge ID.
func (f *FetchyrNamespace) SubmitMFA(ctx context.Context, challengeID, code string) (*MfaResult, error) {
	raw, err := f.t.post(ctx, "fetchyr/mfa/submit", map[string]interface{}{
		"challenge_id": challengeID,
		"code":         code,
	})
	if err != nil {
		return nil, err
	}
	return parseMfaResult(raw), nil
}

// CheckDuplicates deduplicates a list of records, optionally scoped to a domain.
func (f *FetchyrNamespace) CheckDuplicates(ctx context.Context, records []map[string]interface{}, domain string) (*DeduplicationResult, error) {
	body := map[string]interface{}{"records": records}
	if domain != "" {
		body["domain"] = domain
	}

	raw, err := f.t.post(ctx, "fetchyr/duplicates/check", body)
	if err != nil {
		return nil, err
	}
	return parseDeduplicationResult(raw), nil
}

// ─── parsers ─────────────────────────────────────────────────────────────────

func parseSessionResult(raw map[string]interface{}) *SessionResult {
	return &SessionResult{
		Success:       boolField(raw, "success"),
		SessionID:     strField(raw, "session_id"),
		URL:           strField(raw, "url"),
		ArtifactID:    optStrField(raw, "artifact_id"),
		ScreenshotURL: optStrField(raw, "screenshot_url"),
		Error:         optStrField(raw, "error"),
	}
}

func parseFetchyrResult(raw map[string]interface{}) *FetchyrResult {
	return &FetchyrResult{
		Success:           boolField(raw, "success"),
		URL:               strField(raw, "url"),
		StatusCode:        intField(raw, "status_code"),
		HTML:              optStrField(raw, "html"),
		ArtifactID:        optStrField(raw, "artifact_id"),
		SessionArtifactID: strField(raw, "session_artifact_id"),
		Error:             optStrField(raw, "error"),
	}
}

func parseWorkflowResult(raw map[string]interface{}) *WorkflowResult {
	return &WorkflowResult{
		WorkflowID: strField(raw, "workflow_id"),
		Name:       strField(raw, "name"),
		Steps:      sliceMapField(raw, "steps"),
		URL:        optStrField(raw, "url"),
		CreatedAt:  strField(raw, "created_at"),
		Error:      optStrField(raw, "error"),
	}
}

func parseExecutionResult(raw map[string]interface{}) *ExecutionResult {
	return &ExecutionResult{
		ExecutionID: strField(raw, "execution_id"),
		WorkflowID:  strField(raw, "workflow_id"),
		Status:      strField(raw, "status"),
		StartedAt:   optStrField(raw, "started_at"),
		CompletedAt: optStrField(raw, "completed_at"),
		Records:     sliceMapField(raw, "records"),
		Error:       optStrField(raw, "error"),
	}
}

func parseFormDetectionResult(raw map[string]interface{}) *FormDetectionResult {
	return &FormDetectionResult{
		Forms:      sliceMapField(raw, "forms"),
		TotalForms: intField(raw, "total_forms"),
		Error:      optStrField(raw, "error"),
	}
}

func parseMfaDetectionResult(raw map[string]interface{}) *MfaDetectionResult {
	return &MfaDetectionResult{
		MfaDetected: boolField(raw, "mfa_detected"),
		ChallengeID: optStrField(raw, "challenge_id"),
		MfaType:     optStrField(raw, "mfa_type"),
		Error:       optStrField(raw, "error"),
	}
}

func parseMfaResult(raw map[string]interface{}) *MfaResult {
	return &MfaResult{
		Success:           boolField(raw, "success"),
		SessionArtifactID: optStrField(raw, "session_artifact_id"),
		Error:             optStrField(raw, "error"),
	}
}

func parseDeduplicationResult(raw map[string]interface{}) *DeduplicationResult {
	return &DeduplicationResult{
		UniqueRecords:  sliceMapField(raw, "unique_records"),
		DuplicateCount: intField(raw, "duplicate_count"),
		TotalInput:     intField(raw, "total_input"),
		Error:          optStrField(raw, "error"),
	}
}
