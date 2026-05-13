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

// StoreCredentials stores login credentials securely.
func (f *FetchyrNamespace) StoreCredentials(ctx context.Context, name string, credentials map[string]interface{}) (map[string]interface{}, error) {
	credentials["name"] = name
	return f.t.post(ctx, "fetchyr/account/credentials", credentials)
}

// ListCredentials lists stored credential names (no secret values).
func (f *FetchyrNamespace) ListCredentials(ctx context.Context) (map[string]interface{}, error) {
	return f.t.get(ctx, "fetchyr/account/credentials", nil)
}

// DeleteCredentials deletes stored credentials by name.
func (f *FetchyrNamespace) DeleteCredentials(ctx context.Context, name string) error {
	return f.t.delete(ctx, fmt.Sprintf("fetchyr/account/credentials/%s", name))
}

// ListSessions lists active browser sessions.
func (f *FetchyrNamespace) ListSessions(ctx context.Context) (map[string]interface{}, error) {
	return f.t.get(ctx, "fetchyr/sessions", nil)
}

// TerminateSession terminates a session and releases Chrome slot.
func (f *FetchyrNamespace) TerminateSession(ctx context.Context, artifactID string) error {
	return f.t.delete(ctx, fmt.Sprintf("fetchyr/sessions/%s", artifactID))
}

// FillForm fills and optionally submits a form on a page.
func (f *FetchyrNamespace) FillForm(ctx context.Context, targetURL string, formData map[string]string, sessionArtifactID string, submit bool) (map[string]interface{}, error) {
	body := map[string]interface{}{"url": targetURL, "form_data": formData, "submit": submit}
	if sessionArtifactID != "" {
		body["session_artifact_id"] = sessionArtifactID
	}
	return f.t.post(ctx, "fetchyr/form/fill", body)
}

// ListMFAChallenges lists pending MFA challenges.
func (f *FetchyrNamespace) ListMFAChallenges(ctx context.Context) (map[string]interface{}, error) {
	return f.t.get(ctx, "fetchyr/mfa-queue", nil)
}

// GetMFAChallenge gets a specific MFA challenge by ID.
func (f *FetchyrNamespace) GetMFAChallenge(ctx context.Context, challengeID string) (map[string]interface{}, error) {
	return f.t.get(ctx, fmt.Sprintf("fetchyr/mfa/challenges/%s", challengeID), nil)
}

// GetMFAStatistics gets MFA statistics for a domain.
func (f *FetchyrNamespace) GetMFAStatistics(ctx context.Context, domain string) (map[string]interface{}, error) {
	return f.t.get(ctx, fmt.Sprintf("fetchyr/mfa/statistics/%s", domain), nil)
}

// ListWorkflows lists all RPA workflow definitions.
func (f *FetchyrNamespace) ListWorkflows(ctx context.Context) (map[string]interface{}, error) {
	return f.t.get(ctx, "fetchyr/workflows", nil)
}

// GetWorkflow gets a workflow definition by ID.
func (f *FetchyrNamespace) GetWorkflow(ctx context.Context, workflowID string) (map[string]interface{}, error) {
	return f.t.get(ctx, fmt.Sprintf("fetchyr/workflows/%s", workflowID), nil)
}

// UpdateWorkflow updates an existing workflow.
func (f *FetchyrNamespace) UpdateWorkflow(ctx context.Context, workflowID string, updates map[string]interface{}) (map[string]interface{}, error) {
	return f.t.patch(ctx, fmt.Sprintf("fetchyr/workflows/%s", workflowID), updates)
}

// DeleteWorkflow deletes a workflow definition.
func (f *FetchyrNamespace) DeleteWorkflow(ctx context.Context, workflowID string) error {
	return f.t.delete(ctx, fmt.Sprintf("fetchyr/workflows/%s", workflowID))
}

// GetWorkflowStatistics gets execution statistics for a workflow.
func (f *FetchyrNamespace) GetWorkflowStatistics(ctx context.Context, workflowID string) (map[string]interface{}, error) {
	return f.t.get(ctx, fmt.Sprintf("fetchyr/workflows/%s/statistics", workflowID), nil)
}

// CreateMultiSiteWorkflow creates a multi-site orchestration workflow.
func (f *FetchyrNamespace) CreateMultiSiteWorkflow(ctx context.Context, sites []map[string]interface{}, name string) (map[string]interface{}, error) {
	body := map[string]interface{}{"sites": sites}
	if name != "" {
		body["name"] = name
	}
	return f.t.post(ctx, "fetchyr/multi-site-workflows", body)
}

// CheckDuplicates deduplicates a list of records, optionally scoped to a domain.
func (f *FetchyrNamespace) CheckDuplicates(ctx context.Context, records []map[string]interface{}, domain string) (*DeduplicationResult, error) {
	body := map[string]interface{}{"records": records}
	if domain != "" {
		body["domain"] = domain
	}

	raw, err := f.t.post(ctx, "fetchyr/deduplication/check", body)
	if err != nil {
		return nil, err
	}
	return parseDeduplicationResult(raw), nil
}

// CreateDedupSession creates a deduplication session.
func (f *FetchyrNamespace) CreateDedupSession(ctx context.Context, config map[string]interface{}) (map[string]interface{}, error) {
	return f.t.post(ctx, "fetchyr/deduplication/sessions", config)
}

// ListDedupSessions lists active deduplication sessions.
func (f *FetchyrNamespace) ListDedupSessions(ctx context.Context) (map[string]interface{}, error) {
	return f.t.get(ctx, "fetchyr/deduplication/sessions/active", nil)
}

// GetDedupSession gets a deduplication session by ID.
func (f *FetchyrNamespace) GetDedupSession(ctx context.Context, sessionID string) (map[string]interface{}, error) {
	return f.t.get(ctx, fmt.Sprintf("fetchyr/deduplication/sessions/%s", sessionID), nil)
}

// GetDedupSessionStatistics gets statistics for a deduplication session.
func (f *FetchyrNamespace) GetDedupSessionStatistics(ctx context.Context, sessionID string) (map[string]interface{}, error) {
	return f.t.get(ctx, fmt.Sprintf("fetchyr/deduplication/sessions/%s/statistics", sessionID), nil)
}

// GetDedupDomainStatistics gets deduplication statistics for a domain.
func (f *FetchyrNamespace) GetDedupDomainStatistics(ctx context.Context, domain string) (map[string]interface{}, error) {
	return f.t.get(ctx, fmt.Sprintf("fetchyr/deduplication/statistics/%s", domain), nil)
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
