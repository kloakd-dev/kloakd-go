package kloakd

import "context"

// FetchyrNamespace exposes the RPA & Authentication module.
// Access via client.Fetchyr.
type FetchyrNamespace struct {
	t *transport
}

// Authenticate logs in to a site with credentials stored under
// credentialName (StoreCredentials) and returns the authenticated session.
// It replaces Login, whose route was removed: the API no longer takes
// selectors or a password per call. Like DetectForms, FillForm and
// DetectMFA, a request the API hands off as a job (202, after about 85 s) is
// followed to its result; see Config.PollInterval.
func (f *FetchyrNamespace) Authenticate(ctx context.Context, credentialName, targetURL string) (*SessionResult, error) {
	raw, err := f.t.postFollowing(ctx, "fetchyr/authenticate", map[string]interface{}{
		"credential_name": credentialName,
		"target_url":      targetURL,
	}, nil)
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
	raw, err := f.t.post(ctx, pathf("fetchyr/workflows/%s/execute", workflowID), body)
	if err != nil {
		return nil, err
	}
	return parseExecutionResult(raw), nil
}

// GetExecution retrieves the result of a workflow execution.
func (f *FetchyrNamespace) GetExecution(ctx context.Context, workflowID, executionID string) (*ExecutionResult, error) {
	raw, err := f.t.get(ctx, pathf("fetchyr/workflows/%s/executions/%s", workflowID, executionID), nil)
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
	raw, err := f.t.post(ctx, pathf("fetchyr/workflows/%s/executions/%s/resume", workflowID, executionID), resume)
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

	raw, err := f.t.postFollowing(ctx, "fetchyr/form/detect", body, nil)
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

	raw, err := f.t.postFollowing(ctx, "fetchyr/mfa/detect", body, nil)
	if err != nil {
		return nil, err
	}
	return parseMfaDetectionResult(raw), nil
}

// ResolveMFA resolves a queued MFA challenge (the code, or "approved").
// It replaces SubmitMFA, whose route was removed.
func (f *FetchyrNamespace) ResolveMFA(ctx context.Context, challengeID, resolution string) (map[string]interface{}, error) {
	return f.t.post(ctx, pathf("fetchyr/mfa-queue/%s/resolve", challengeID), map[string]interface{}{
		"resolution": resolution,
	})
}

// StoreCredentials stores login credentials securely.
func (f *FetchyrNamespace) StoreCredentials(ctx context.Context, name string, credentials map[string]interface{}) (map[string]interface{}, error) {
	body := make(map[string]interface{}, len(credentials)+1)
	for k, v := range credentials {
		body[k] = v
	}
	body["name"] = name
	return f.t.post(ctx, "fetchyr/account/credentials", body)
}

// ListCredentials lists stored credential names (no secret values).
func (f *FetchyrNamespace) ListCredentials(ctx context.Context) (map[string]interface{}, error) {
	return f.t.get(ctx, "fetchyr/account/credentials", nil)
}

// DeleteCredentials deletes stored credentials by name.
func (f *FetchyrNamespace) DeleteCredentials(ctx context.Context, name string) error {
	return f.t.delete(ctx, pathf("fetchyr/account/credentials/%s", name))
}

// ListSessions lists active browser sessions.
func (f *FetchyrNamespace) ListSessions(ctx context.Context) (map[string]interface{}, error) {
	return f.t.get(ctx, "fetchyr/sessions", nil)
}

// TerminateSession terminates a session and releases Chrome slot.
func (f *FetchyrNamespace) TerminateSession(ctx context.Context, artifactID string) error {
	return f.t.delete(ctx, pathf("fetchyr/sessions/%s", artifactID))
}

// LoginCookie is one cookie of your own browser session, as a browser
// exports it (DevTools, cookie-export extensions). Name and Value are
// required; the other fields are sent only when set.
type LoginCookie struct {
	Name   string
	Value  string
	Domain string
	Path   string
	// Expires and ExpirationDate are epoch seconds (extension exports use
	// the second name); 0 sends none.
	Expires        float64
	ExpirationDate float64
	Secure         bool
	HTTPOnly       bool
	SameSite       string
}

func (c LoginCookie) body() map[string]interface{} {
	m := map[string]interface{}{"name": c.Name, "value": c.Value}
	if c.Domain != "" {
		m["domain"] = c.Domain
	}
	if c.Path != "" {
		m["path"] = c.Path
	}
	if c.Expires != 0 {
		m["expires"] = c.Expires
	}
	if c.ExpirationDate != 0 {
		m["expirationDate"] = c.ExpirationDate
	}
	if c.Secure {
		m["secure"] = true
	}
	if c.HTTPOnly {
		m["httpOnly"] = true
	}
	if c.SameSite != "" {
		m["sameSite"] = c.SameSite
	}
	return m
}

// LoginSessionOptions configures Fetchyr.StartLoginSession. Only the fields
// that are set are sent.
type LoginSessionOptions struct {
	// Method is "interactive" (the API's default when ""): KLOAKD opens the
	// site's login in a browser and asks for the credentials, and an MFA
	// code, through the auth-chain methods. Or "cookies": use a session you
	// already have in your own browser.
	Method string
	// Cookies are your browser's cookies for the site (Method "cookies").
	Cookies []LoginCookie
	// UserAgent is your browser's navigator.userAgent. The API requires it
	// with Method "cookies": the site issued the cookies to that browser.
	UserAgent string
	// Country is the two-letter country the session's exit is in (where
	// the account usually logs in from).
	Country string
}

// LoginSession is the answer to Fetchyr.StartLoginSession.
type LoginSession struct {
	Method string
	// Status is "waiting_for_login" (interactive) or "ready" (cookies).
	Status    string
	TargetURL string
	// DiscoveryID is the auth-chain discovery of an interactive login: the
	// id GetAuthChainStatus, ResumeAuthChainCredentials and
	// ResumeAuthChainMFA take. nil for a cookies login.
	DiscoveryID *string
	// SessionArtifactID is the session, once there is one: the id to pass
	// as SessionArtifactID to fetch, crawl and extract calls, and to
	// GetSession, ResumeSession and TerminateSession.
	SessionArtifactID *string
	// Next names the routes of the next steps, as the API sends them.
	Next map[string]string
	// CookiesKept is how many of the cookies sent the session kept (nil for
	// an interactive login); CookiesDropped names those it left out.
	CookiesKept    *int
	CookiesDropped []string
	// Protections are the limits the session runs under.
	Protections map[string]interface{}
	Message     string
	// Raw is the answer as received.
	Raw map[string]interface{}
}

// SessionStatus is the state of a login session (Fetchyr.GetSession,
// Fetchyr.ResumeSession). It carries no cookie values.
type SessionStatus struct {
	ArtifactID string
	// Status is active, paused, EXPIRED or INVALIDATED.
	Status       string
	TargetDomain *string
	AuthMethod   *string
	CreatedAt    *string
	ExpiresAt    *string
	LastUsedAt   *string
	PausedAt     *string
	// PauseReason is logged_out, rate_limited or challenged; PauseMessage
	// says what to do.
	PauseReason  *string
	PauseMessage *string
	// Identity is what the session presents to the site.
	Identity         map[string]interface{}
	RequestsThisHour *int
	Protections      map[string]interface{}
	// Raw is the answer as received.
	Raw map[string]interface{}
}

// StartLoginSession logs in to a site with your own account
// (POST fetchyr/login-sessions). targetURL is a page of the site that needs
// the login. With Method "interactive" (the default) the API answers 202
// with Status "waiting_for_login" and a DiscoveryID: supply the credentials
// and an MFA code with ResumeAuthChainCredentials and ResumeAuthChainMFA.
// With Method "cookies" it answers 201 with Status "ready" and the
// SessionArtifactID. A request the API refuses (cookies without a user
// agent, no usable cookie, a URL that is not allowed) is a 422 *ApiError.
func (f *FetchyrNamespace) StartLoginSession(ctx context.Context, targetURL string, opts *LoginSessionOptions) (*LoginSession, error) {
	body := map[string]interface{}{"target_url": targetURL}
	if opts != nil {
		if opts.Method != "" {
			body["method"] = opts.Method
		}
		if opts.Cookies != nil {
			cookies := make([]map[string]interface{}, 0, len(opts.Cookies))
			for _, c := range opts.Cookies {
				cookies = append(cookies, c.body())
			}
			body["cookies"] = cookies
		}
		if opts.UserAgent != "" {
			body["user_agent"] = opts.UserAgent
		}
		if opts.Country != "" {
			body["country"] = opts.Country
		}
	}
	raw, err := f.t.post(ctx, "fetchyr/login-sessions", body)
	if err != nil {
		return nil, err
	}
	next := map[string]string{}
	for k, v := range mapField(raw, "next") {
		if s, ok := v.(string); ok {
			next[k] = s
		}
	}
	return &LoginSession{
		Method:            strField(raw, "method"),
		Status:            strField(raw, "status"),
		TargetURL:         strField(raw, "target_url"),
		DiscoveryID:       optStrField(raw, "discovery_id"),
		SessionArtifactID: optStrField(raw, "session_artifact_id"),
		Next:              next,
		CookiesKept:       optIntField(raw, "cookies_kept"),
		CookiesDropped:    strSliceField(raw, "cookies_dropped"),
		Protections:       mapField(raw, "protections"),
		Message:           strField(raw, "message"),
		Raw:               raw,
	}, nil
}

// GetSession gets a login session's state (GET fetchyr/sessions/{id}):
// whether it is active or paused, why it was paused, and what it presents
// to the site. An unknown session is a 404 *ApiError.
func (f *FetchyrNamespace) GetSession(ctx context.Context, artifactID string) (*SessionStatus, error) {
	raw, err := f.t.get(ctx, pathf("fetchyr/sessions/%s", artifactID), nil)
	if err != nil {
		return nil, err
	}
	return parseSessionStatus(raw), nil
}

// ResumeSession puts a paused login session back to use
// (POST fetchyr/sessions/{id}/resume) and returns its state. A session that
// is not paused, or that was paused because the site logged it out
// (PauseReason "logged_out": log in again with StartLoginSession), is a 409
// *ApiError.
func (f *FetchyrNamespace) ResumeSession(ctx context.Context, artifactID string) (*SessionStatus, error) {
	raw, err := f.t.post(ctx, pathf("fetchyr/sessions/%s/resume", artifactID), nil)
	if err != nil {
		return nil, err
	}
	return parseSessionStatus(raw), nil
}

func parseSessionStatus(raw map[string]interface{}) *SessionStatus {
	return &SessionStatus{
		ArtifactID:       strField(raw, "artifact_id"),
		Status:           strField(raw, "status"),
		TargetDomain:     optStrField(raw, "target_domain"),
		AuthMethod:       optStrField(raw, "auth_method"),
		CreatedAt:        optStrField(raw, "created_at"),
		ExpiresAt:        optStrField(raw, "expires_at"),
		LastUsedAt:       optStrField(raw, "last_used_at"),
		PausedAt:         optStrField(raw, "paused_at"),
		PauseReason:      optStrField(raw, "pause_reason"),
		PauseMessage:     optStrField(raw, "pause_message"),
		Identity:         mapField(raw, "identity"),
		RequestsThisHour: optIntField(raw, "requests_this_hour"),
		Protections:      mapField(raw, "protections"),
		Raw:              raw,
	}
}

// FillForm fills and submits a form on a page.
func (f *FetchyrNamespace) FillForm(ctx context.Context, targetURL string, formData map[string]string, sessionArtifactID string) (map[string]interface{}, error) {
	body := map[string]interface{}{"url": targetURL, "form_data": formData}
	if sessionArtifactID != "" {
		body["session_artifact_id"] = sessionArtifactID
	}
	return f.t.postFollowing(ctx, "fetchyr/form/fill", body, nil)
}

// ListMFAChallenges lists pending MFA challenges.
func (f *FetchyrNamespace) ListMFAChallenges(ctx context.Context) (map[string]interface{}, error) {
	return f.t.get(ctx, "fetchyr/mfa-queue", nil)
}

// GetMFAChallenge gets a specific MFA challenge by ID.
func (f *FetchyrNamespace) GetMFAChallenge(ctx context.Context, challengeID string) (map[string]interface{}, error) {
	return f.t.get(ctx, pathf("fetchyr/mfa/challenges/%s", challengeID), nil)
}

// GetMFAStatistics gets MFA statistics for a domain.
func (f *FetchyrNamespace) GetMFAStatistics(ctx context.Context, domain string) (map[string]interface{}, error) {
	return f.t.get(ctx, pathf("fetchyr/mfa/statistics/%s", domain), nil)
}

// ListWorkflowsOptions pages Fetchyr.ListWorkflows (zero values: the API's defaults).
type ListWorkflowsOptions struct {
	Page     int
	PageSize int
}

// ListWorkflows lists RPA workflow definitions.
func (f *FetchyrNamespace) ListWorkflows(ctx context.Context, opts *ListWorkflowsOptions) (map[string]interface{}, error) {
	params := map[string]string{}
	if opts != nil {
		setInt(params, "page", opts.Page)
		setInt(params, "page_size", opts.PageSize)
	}
	return f.t.get(ctx, "fetchyr/workflows", params)
}

// GetWorkflow gets a workflow definition by ID.
func (f *FetchyrNamespace) GetWorkflow(ctx context.Context, workflowID string) (map[string]interface{}, error) {
	return f.t.get(ctx, pathf("fetchyr/workflows/%s", workflowID), nil)
}

// UpdateWorkflow updates an existing workflow.
func (f *FetchyrNamespace) UpdateWorkflow(ctx context.Context, workflowID string, updates map[string]interface{}) (map[string]interface{}, error) {
	return f.t.patch(ctx, pathf("fetchyr/workflows/%s", workflowID), updates)
}

// DeleteWorkflow deletes a workflow definition.
func (f *FetchyrNamespace) DeleteWorkflow(ctx context.Context, workflowID string) error {
	return f.t.delete(ctx, pathf("fetchyr/workflows/%s", workflowID))
}

// GetWorkflowStatistics gets execution statistics for a workflow.
func (f *FetchyrNamespace) GetWorkflowStatistics(ctx context.Context, workflowID string) (map[string]interface{}, error) {
	return f.t.get(ctx, pathf("fetchyr/workflows/%s/statistics", workflowID), nil)
}

// SiteDependency orders two sites of a multi-site workflow: DependentSite
// runs after DependencySite (site_id values). DependencyType is
// "sequential" (the API's default), "data" or "conditional".
type SiteDependency struct {
	DependentSite    string
	DependencySite   string
	DependencyType   string
	Condition        string
	DataRequirements []string
	// TimeoutSecs is the longest wait for the dependency; 0 for none.
	TimeoutSecs int
}

// MultiSiteWorkflowOptions configures optional parameters for CreateMultiSiteWorkflow.
type MultiSiteWorkflowOptions struct {
	Description  string
	Dependencies []SiteDependency
	GlobalConfig map[string]interface{}
}

// CreateMultiSiteWorkflow creates a multi-site orchestration workflow. Each
// site is {site_id, name, url, authentication_config?, workflow_steps?,
// timeout?, retry_count?, priority?, metadata?}.
func (f *FetchyrNamespace) CreateMultiSiteWorkflow(ctx context.Context, name string, sites []map[string]interface{}, opts *MultiSiteWorkflowOptions) (map[string]interface{}, error) {
	body := map[string]interface{}{"name": name, "sites": sites}
	if opts != nil {
		if opts.Description != "" {
			body["description"] = opts.Description
		}
		if len(opts.Dependencies) > 0 {
			deps := make([]map[string]interface{}, 0, len(opts.Dependencies))
			for _, d := range opts.Dependencies {
				dep := map[string]interface{}{"dependent_site": d.DependentSite, "dependency_site": d.DependencySite}
				if d.DependencyType != "" {
					dep["dependency_type"] = d.DependencyType
				}
				if d.Condition != "" {
					dep["condition"] = d.Condition
				}
				if len(d.DataRequirements) > 0 {
					dep["data_requirements"] = d.DataRequirements
				}
				if d.TimeoutSecs > 0 {
					dep["timeout"] = d.TimeoutSecs
				}
				deps = append(deps, dep)
			}
			body["dependencies"] = deps
		}
		if len(opts.GlobalConfig) > 0 {
			body["global_config"] = opts.GlobalConfig
		}
	}
	return f.t.post(ctx, "fetchyr/multi-site-workflows", body)
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
	return f.t.get(ctx, pathf("fetchyr/deduplication/sessions/%s", sessionID), nil)
}

// GetDedupSessionStatistics gets statistics for a deduplication session.
func (f *FetchyrNamespace) GetDedupSessionStatistics(ctx context.Context, sessionID string) (map[string]interface{}, error) {
	return f.t.get(ctx, pathf("fetchyr/deduplication/sessions/%s/statistics", sessionID), nil)
}

// GetDedupDomainStatistics gets deduplication statistics for a domain.
func (f *FetchyrNamespace) GetDedupDomainStatistics(ctx context.Context, domain string) (map[string]interface{}, error) {
	return f.t.get(ctx, pathf("fetchyr/deduplication/statistics/%s", domain), nil)
}

// DiscoverAuthChain starts discovering how a site authenticates.
func (f *FetchyrNamespace) DiscoverAuthChain(ctx context.Context, targetURL string) (map[string]interface{}, error) {
	return f.t.post(ctx, "fetchyr/auth-chain/discover", map[string]interface{}{"target_url": targetURL})
}

// GetAuthChainStatus gets an auth-chain discovery's status.
func (f *FetchyrNamespace) GetAuthChainStatus(ctx context.Context, discoveryID string) (map[string]interface{}, error) {
	return f.t.get(ctx, pathf("fetchyr/auth-chain/%s/status", discoveryID), nil)
}

// ResumeAuthChainCredentials gives the credentials a paused auth-chain
// discovery asked for. With remember true the API stores them encrypted
// for later workflow runs.
func (f *FetchyrNamespace) ResumeAuthChainCredentials(ctx context.Context, discoveryID, username, password string, remember bool) (map[string]interface{}, error) {
	return f.t.post(ctx, pathf("fetchyr/auth-chain/%s/resume-credentials", discoveryID), map[string]interface{}{
		"username": username, "password": password, "remember": remember,
	})
}

// ResumeAuthChainMFA gives the MFA code a paused auth-chain discovery asked for.
func (f *FetchyrNamespace) ResumeAuthChainMFA(ctx context.Context, discoveryID, mfaCode string) (map[string]interface{}, error) {
	return f.t.post(ctx, pathf("fetchyr/auth-chain/%s/resume-mfa", discoveryID), map[string]interface{}{
		"mfa_code": mfaCode,
	})
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
