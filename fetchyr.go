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

// Authenticate logs in to a site with credentials stored under
// credentialName (StoreCredentials) and returns the authenticated session.
// It replaces Login, whose route was removed: the API no longer takes
// selectors or a password per call.
func (f *FetchyrNamespace) Authenticate(ctx context.Context, credentialName, targetURL string) (*SessionResult, error) {
	raw, err := f.t.post(ctx, "fetchyr/authenticate", map[string]interface{}{
		"credential_name": credentialName,
		"target_url":      targetURL,
	})
	if err != nil {
		return nil, err
	}
	return parseSessionResult(raw), nil
}

// CreateWorkflowOptions configures optional parameters for CreateWorkflow.
type CreateWorkflowOptions struct {
	Description  string
	WorkflowType string
}

// CreateWorkflow creates a new RPA workflow.
func (f *FetchyrNamespace) CreateWorkflow(ctx context.Context, name string, steps []map[string]interface{}, opts *CreateWorkflowOptions) (*WorkflowResult, error) {
	body := map[string]interface{}{
		"name":  name,
		"steps": steps,
	}
	if opts != nil {
		if opts.Description != "" {
			body["description"] = opts.Description
		}
		if opts.WorkflowType != "" {
			body["workflow_type"] = opts.WorkflowType
		}
	}

	raw, err := f.t.post(ctx, "fetchyr/workflows", body)
	if err != nil {
		return nil, err
	}
	return parseWorkflowResult(raw), nil
}

// ExecuteWorkflowOptions configures optional parameters for ExecuteWorkflow.
type ExecuteWorkflowOptions struct {
	SessionArtifactID string
	InputData         map[string]interface{}
}

// ExecuteWorkflow runs a workflow against targetURL.
func (f *FetchyrNamespace) ExecuteWorkflow(ctx context.Context, workflowID, targetURL string, opts *ExecuteWorkflowOptions) (*ExecutionResult, error) {
	body := map[string]interface{}{"target_url": targetURL}
	if opts != nil {
		if opts.SessionArtifactID != "" {
			body["session_artifact_id"] = opts.SessionArtifactID
		}
		if len(opts.InputData) > 0 {
			body["input_data"] = opts.InputData
		}
	}
	raw, err := f.t.post(ctx, fmt.Sprintf("fetchyr/workflows/%s/execute", workflowID), body)
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

// ResumeExecution resumes a paused execution (after MFA).
func (f *FetchyrNamespace) ResumeExecution(ctx context.Context, workflowID, executionID string, resume map[string]interface{}) (*ExecutionResult, error) {
	if resume == nil {
		resume = map[string]interface{}{}
	}
	raw, err := f.t.post(ctx, fmt.Sprintf("fetchyr/workflows/%s/executions/%s/resume", workflowID, executionID), resume)
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

	raw, err := f.t.post(ctx, "fetchyr/form/detect", body)
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

// ResolveMFA resolves a queued MFA challenge (the code, or "approved").
// It replaces SubmitMFA, whose route was removed.
func (f *FetchyrNamespace) ResolveMFA(ctx context.Context, challengeID, resolution string) (map[string]interface{}, error) {
	return f.t.post(ctx, fmt.Sprintf("fetchyr/mfa-queue/%s/resolve", challengeID), map[string]interface{}{
		"resolution": resolution,
	})
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

// FillForm fills and submits a form on a page.
func (f *FetchyrNamespace) FillForm(ctx context.Context, targetURL string, formData map[string]string, sessionArtifactID string) (map[string]interface{}, error) {
	body := map[string]interface{}{"url": targetURL, "form_data": formData}
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
func (f *FetchyrNamespace) CreateMultiSiteWorkflow(ctx context.Context, name string, sites []map[string]interface{}) (map[string]interface{}, error) {
	return f.t.post(ctx, "fetchyr/multi-site-workflows", map[string]interface{}{"name": name, "sites": sites})
}

// CheckDuplicateOptions configures optional parameters for CheckDuplicate.
type CheckDuplicateOptions struct {
	ContentHash string
	SessionID   string
}

// CheckDuplicate reports whether a URL (or its content hash) was already
// seen, within a deduplication session if given.
func (f *FetchyrNamespace) CheckDuplicate(ctx context.Context, targetURL string, opts *CheckDuplicateOptions) (*DeduplicationResult, error) {
	body := map[string]interface{}{"url": targetURL}
	if opts != nil {
		if opts.ContentHash != "" {
			body["content_hash"] = opts.ContentHash
		}
		if opts.SessionID != "" {
			body["session_id"] = opts.SessionID
		}
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

// DiscoverAuthChain starts discovering how a site authenticates.
func (f *FetchyrNamespace) DiscoverAuthChain(ctx context.Context, targetURL string) (map[string]interface{}, error) {
	return f.t.post(ctx, "fetchyr/auth-chain/discover", map[string]interface{}{"target_url": targetURL})
}

// GetAuthChainStatus gets an auth-chain discovery's status.
func (f *FetchyrNamespace) GetAuthChainStatus(ctx context.Context, discoveryID string) (map[string]interface{}, error) {
	return f.t.get(ctx, fmt.Sprintf("fetchyr/auth-chain/%s/status", discoveryID), nil)
}

// ─── parsers ─────────────────────────────────────────────────────────────────

func parseSessionResult(raw map[string]interface{}) *SessionResult {
	return &SessionResult{
		Success:      boolField(raw, "success"),
		SessionID:    strField(raw, "session_id"),
		ArtifactID:   optStrField(raw, "artifact_id"),
		TargetDomain: optStrField(raw, "target_domain"),
		AuthMethod:   optStrField(raw, "auth_method"),
		Error:        optStrField(raw, "error"),
	}
}

func parseWorkflowResult(raw map[string]interface{}) *WorkflowResult {
	return &WorkflowResult{
		WorkflowID:   strField(raw, "id"),
		Name:         strField(raw, "name"),
		Description:  optStrField(raw, "description"),
		WorkflowType: strField(raw, "workflow_type"),
		Steps:        sliceMapField(raw, "steps"),
		IsActive:     boolField(raw, "is_active"),
		CreatedAt:    strField(raw, "created_at"),
		Error:        optStrField(raw, "error"),
	}
}

func parseExecutionResult(raw map[string]interface{}) *ExecutionResult {
	return &ExecutionResult{
		ExecutionID: strField(raw, "execution_id"),
		WorkflowID:  strField(raw, "workflow_id"),
		Status:      strField(raw, "status"),
		StartedAt:   optStrField(raw, "started_at"),
		CompletedAt: optStrField(raw, "completed_at"),
		Result:      mapField(raw, "result"),
		Error:       optStrField(raw, "error"),
	}
}

func parseFormDetectionResult(raw map[string]interface{}) *FormDetectionResult {
	return &FormDetectionResult{
		URL:               strField(raw, "url"),
		FormsFound:        intField(raw, "forms_found"),
		LoginFormDetected: boolField(raw, "login_form_detected"),
		Fields:            sliceMapField(raw, "fields"),
	}
}

func parseMfaDetectionResult(raw map[string]interface{}) *MfaDetectionResult {
	return &MfaDetectionResult{
		MfaDetected: boolField(raw, "mfa_detected"),
		ChallengeID: optStrField(raw, "challenge_id"),
		MfaType:     optStrField(raw, "mfa_type"),
		Message:     optStrField(raw, "message"),
	}
}

func parseDeduplicationResult(raw map[string]interface{}) *DeduplicationResult {
	return &DeduplicationResult{
		IsDuplicate:     boolField(raw, "is_duplicate"),
		OriginalURL:     optStrField(raw, "original_url"),
		SimilarityScore: float64Field(raw, "similarity_score"),
		MatchType:       optStrField(raw, "match_type"),
	}
}
