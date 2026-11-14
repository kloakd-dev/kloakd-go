package kloakd

// ─── Pipeline events ─────────────────────────────────────────────────────────

// PipelineEvent is one event of a pipeline's SSE stream (a crawl's
// webgrph/crawl/{id}/events, and an Evadr job's events when the job has a
// live pipeline). Its fields are those of the API's PipelineEvent
// (core/pipeline/models.py). Err is set only on the last value of a stream
// whose reading failed; the other fields are then empty.
type PipelineEvent struct {
	PipelineID  string
	EventType   string // e.g. "stage_started", "pipeline_complete", "pipeline_error"
	StageID     string
	Status      string
	Phase       string
	Message     string
	SubStatus   string
	ProgressPct *float64
	DurationMs  *float64
	Metadata    map[string]interface{}
	// Error is the event's PipelineError ({error_code, category, severity,
	// user_message, technical_message, recovery_actions, ...}), nil when none.
	Error     map[string]interface{}
	Timestamp string
	Sequence  int
	// Raw is the event's JSON as received.
	Raw map[string]interface{}
	Err error
}

// ─── Evadr ───────────────────────────────────────────────────────────────────

// FetchResult is returned by Evadr.Fetch.
type FetchResult struct {
	Success    bool
	URL        string
	StatusCode int
	TierUsed   int
	// HTML is nil when the API returned none (for example when
	// AuthRequired is true).
	HTML            *string
	VendorDetected  *string
	AntiBotBypassed bool
	ArtifactID      *string
	Error           *string
	// AuthRequired is true when a login page was detected at the URL;
	// DiscoveryID is then the auth-chain discovery started for it
	// (Fetchyr.GetAuthChainStatus).
	AuthRequired bool
	DiscoveryID  *string
}

// FetchEvent is yielded by Evadr.FetchStream and Evadr.JobEvents. A job with
// a live pipeline streams pipeline events (Pipeline is set). Otherwise the
// API polls the job and streams {type, job_id, status | tier_used,
// artifact_id | error}: Type is "STATUS", "COMPLETE", "FAILED" or "ERROR"
// (modules/evadr/router.py). Err is set only on the last value of a stream
// whose reading failed.
type FetchEvent struct {
	Type       string
	JobID      string
	Status     string
	TierUsed   *int
	ArtifactID *string
	Error      *string
	Pipeline   *PipelineEvent
	Raw        map[string]interface{}
	Err        error
}

// AnalyzeResult is returned by Evadr.Analyze.
type AnalyzeResult struct {
	Blocked            bool
	Vendor             *string
	Confidence         float64
	RecommendedActions []string
}

// ─── Webgrph ─────────────────────────────────────────────────────────────────

// PageNode represents a single page in the site hierarchy.
type PageNode struct {
	ID        string
	URL       string
	ParentURL *string
	Depth     int
	Category  string
	Title     *string
	Children  []string
	Metadata  map[string]interface{}
	// IsGrouping is true for a grouping folder of the displayed tree
	// (metadata.is_grouping): a display node that was never fetched. The
	// pages listing returns them only with IncludeGrouping (0.6.0).
	IsGrouping bool
	// CrawlParentURL is the page the crawl found this page on, when the
	// tree shows it under a grouping folder (crawl_parent_url). Only the
	// stored page records carry it (a crawl stream's discovery_response
	// pages); the pages listing does not, and by default already puts each
	// page under that parent (ParentURL). nil when absent (0.6.0).
	CrawlParentURL *string
	// FetchStatus is what the crawl got for this page
	// (metadata.fetch_status) and FetchError why a fetch failed
	// (metadata.fetch_error). nil when the API sent none.
	FetchStatus *string
	FetchError  *string
}

// CrawlResult is returned by Webgrph.Crawl, WaitForCrawl and StartCrawl.
type CrawlResult struct {
	// Success is true only for a crawl that ended "completed".
	Success bool
	CrawlID string
	// URL is the crawl's URL: sent by StartCrawl's answer and filled by
	// Crawl; empty from WaitForCrawl (the status route does not send it).
	URL string
	// Status is completed, failed, cancelled or held_quota. Empty from
	// StartCrawl, whose answer has no status.
	Status string
	// TotalPages is the pages the crawl found.
	TotalPages      int
	MaxDepthReached int
	Pages           []PageNode
	Total           int
	ArtifactID      *string
	Error           *string

	// Held is true when the crawl is paused because the organization's
	// quota ran out (Status "held_quota"). It is not a failure: Error is
	// nil and Message has the API's text. The crawl resumes by itself on
	// the same id when quota returns; wait for it again with
	// Webgrph.WaitForCrawl. After 30 days held it ends "failed" with
	// ErrorCode "held_quota_expired".
	Held bool
	// Message is the API's text for a held crawl; nil otherwise.
	Message *string
	// IsPartial is true when the crawl stopped before asking for every
	// page it found; PartialReason is then time_budget, max_pages or
	// error. nil until the crawl has reported it.
	IsPartial     *bool
	PartialReason *string
	// Resumable is true when Webgrph.ContinueCrawl can fetch the rest.
	Resumable bool
	// PagesDiscovered, PagesFetched and PagesUnfetched are what the crawl
	// did: pages found, pages fetched, and pages asked for and not
	// obtained. nil until the crawl has reported them.
	PagesDiscovered *int
	PagesFetched    *int
	PagesUnfetched  *int
	// ErrorCode is the API's code for a failed crawl, e.g.
	// "held_quota_expired".
	ErrorCode *string
	// TimeBudgetS is the crawl's time budget in seconds, ElapsedS the time
	// it has used and EstRemainingS the estimated time left. nil until the
	// crawl has reported them.
	TimeBudgetS   *int
	ElapsedS      *float64
	EstRemainingS *float64
}

// CrawlEvent is yielded by Webgrph.CrawlStream and Webgrph.CrawlEvents: a
// crawl streams pipeline events.
type CrawlEvent = PipelineEvent

// ─── Nexus ───────────────────────────────────────────────────────────────────

// NexusAnalyzeResult is returned by Nexus.Analyze.
type NexusAnalyzeResult struct {
	URL          string
	PerceptionID string
	ReasoningID  string
	Strategy     map[string]interface{}
	PageType     string
	Confidence   float64
	ArtifactID   *string
}

// NexusSynthesisResult is returned by Nexus.Synthesize.
type NexusSynthesisResult struct {
	StrategyID      string
	StrategyName    string
	StrategyType    string
	Program         string
	Confidence      float64
	Steps           []map[string]interface{}
	SynthesisTimeMs int
}

// NexusVerifyResult is returned by Nexus.Verify.
type NexusVerifyResult struct {
	StrategyID  string
	Verified    bool
	SafetyScore float64
	Violations  []string

	// Status is "verified" or "unsafe"; nil when the API did not send it.
	Status    *string
	RiskScore float64
	// SafetyChecks is the result of each property the engine checked.
	SafetyChecks         []NexusSafetyCheck
	VerificationResultID *string
	DurationMs           int
}

// NexusSafetyCheck is one property checked by Nexus.Verify.
type NexusSafetyCheck struct {
	Property string
	Passed   bool
	Details  string
}

// NexusReasonResult is returned by Nexus.Reason.
type NexusReasonResult struct {
	Success     bool
	Results     []string
	Confidence  float64
	ReasoningID *string
}

// ─── Parlyr ──────────────────────────────────────────────────────────────────

// ParseResult is returned by Parlyr.Parse.
type ParseResult struct {
	Intent              string
	Confidence          float64
	Tier                int
	Source              string
	Entities            map[string]interface{}
	RequiresAction      bool
	ClarificationNeeded *string
	Reasoning           *string
	DetectedURL         *string
}

// ChatTurn is returned by Parlyr.Chat: one turn's events, collected.
type ChatTurn struct {
	SessionID string
	// OK is false when the turn emitted an "error" event (see Errors).
	OK bool
	// Response is the content of the turn's "text" and "clarification"
	// events, in order, joined with "\n".
	Response string
	// Intent is the "intent" of the interpretation event's data, when the
	// API sends one; "" otherwise. (The API's interpretation data has no
	// intent key today; Interpretation holds what it does send.)
	Intent string
	// Interpretation is the data of the last "interpretation" event
	// (url, entity, fields, summary, ...), nil when there was none.
	Interpretation map[string]interface{}
	// Results holds the data of each "results" event ({items, count, url,
	// job_id, ...}).
	Results []map[string]interface{}
	// ClarificationNeeded is the content of the last "clarification" event.
	ClarificationNeeded *string
	// Warnings holds the content of each "warning" event.
	Warnings []string
	// Errors holds each "error" event (Content is the message, Data and
	// Actions the details and recovery options).
	Errors []ChatEvent
	// Actions are the actions of the last event that offered any.
	Actions []map[string]interface{}
	// Events is every event of the turn, in order.
	Events []ChatEvent
}

// ChatEvent is one event of a Parlyr chat turn (Parlyr.ChatStream), with
// the API's payload fields {type, content, data, actions, timestamp}
// (application/services/chat_orchestrator.py). Type is thinking,
// interpretation, progress, results, error, warning, auth_required,
// mfa_required, clarification, export_ready or text. Err is set only on
// the last value of a stream whose reading failed.
type ChatEvent struct {
	Type      string
	Content   *string
	Data      map[string]interface{}
	Actions   []map[string]interface{}
	Timestamp string
	Raw       map[string]interface{}
	Err       error
}

// ─── Fetchyr ─────────────────────────────────────────────────────────────────

// SessionResult is returned by Fetchyr.Authenticate.
type SessionResult struct {
	Success      bool
	SessionID    string
	ArtifactID   *string
	TargetDomain *string
	AuthMethod   *string
	Error        *string
}

// WorkflowResult is returned by Fetchyr.CreateWorkflow.
type WorkflowResult struct {
	WorkflowID   string
	Name         string
	Description  *string
	WorkflowType string
	Steps        []map[string]interface{}
	IsActive     bool
	CreatedAt    string
	Error        *string
}

// ExecutionResult is returned by Fetchyr.ExecuteWorkflow / GetExecution.
type ExecutionResult struct {
	ExecutionID string
	WorkflowID  string
	Status      string
	StartedAt   *string
	CompletedAt *string
	Result      map[string]interface{}
	Error       *string
}

// FormDetectionResult is returned by Fetchyr.DetectForms.
type FormDetectionResult struct {
	URL               string
	FormsFound        int
	LoginFormDetected bool
	Fields            []map[string]interface{}
}

// MfaDetectionResult is returned by Fetchyr.DetectMFA.
type MfaDetectionResult struct {
	MfaDetected bool
	ChallengeID *string
	MfaType     *string
	Message     *string
}

// DeduplicationResult is returned by Fetchyr.CheckDuplicate.
type DeduplicationResult struct {
	IsDuplicate     bool
	OriginalURL     *string
	SimilarityScore float64
	MatchType       *string
}

// ─── Kolektr ─────────────────────────────────────────────────────────────────

// SchemaField is one field of an extraction schema (schema_spec.fields).
type SchemaField struct {
	Name      string
	Selector  string // CSS selector
	Attribute string // attribute to read; "" for the text
	Type      string // "" means the API's default, "text"
}

// ExtractionResult is returned by Kolektr.Extract and Kolektr.ExtractHTML.
type ExtractionResult struct {
	Success      bool
	URL          string
	Method       string
	Records      []map[string]interface{}
	TotalRecords int
	PagesScraped int
	ArtifactID   *string
	JobID        *string
	Error        *string
	// AuthRequired is true when a login page was detected at the URL;
	// DiscoveryID is then the auth-chain discovery started for it.
	AuthRequired bool
	DiscoveryID  *string
	// Level is the level a fresh scrape ran at ("level1" or "level2") and
	// LevelReason why: "requested", or what automatic mode chose it for
	// (prefixed "auto:"). nil when the API sent none.
	Level       *string
	LevelReason *string
	// LoginOptions says, when AuthRequired is true, how to log in with
	// your own account (Fetchyr.StartLoginSession) and pass the session to
	// the next call. nil otherwise.
	LoginOptions map[string]interface{}
}

// ─── High-level orchestrator ─────────────────────────────────────────────────

// CrawlPage represents a single page result from the high-level Crawl() orchestrator.
type CrawlPage struct {
	Success    bool
	URL        string
	StatusCode int
	TierUsed   int
	HTML       *string
	// StructuredData is the page's first extracted record.
	StructuredData map[string]interface{}
	// Records is every extracted record: one per item with
	// ExtractItemSelector, otherwise at most one (0.6.0).
	Records    []map[string]interface{}
	ArtifactID *string
	Error      *string
	// ExtractionError is the error of the page's extraction, when
	// ExtractSchema / ExtractFields was given and it failed.
	ExtractionError *string
}

// Ok returns true if the page was fetched successfully.
func (p CrawlPage) Ok() bool {
	return p.Success && p.Error == nil
}

// SiteCrawlResult is returned by Client.Crawl. Success, Error, CrawlID,
// Status, Held and Message are the discovery crawl's own (see CrawlResult).
// When Held is true the discovery crawl is paused for quota: no page was
// fetched, and Webgrph.WaitForCrawl(CrawlID) picks the crawl up later.
type SiteCrawlResult struct {
	Success              bool
	URL                  string
	CrawlID              string
	Status               string
	Held                 bool
	Message              *string
	TotalPagesDiscovered int
	PagesFetched         int
	PagesFailed          int
	Pages                []CrawlPage
	CrawlArtifactID      *string
	Error                *string
}

// CrawlProgressEvent is yielded by Client.CrawlStream. Type is
// discovery_complete, page_fetching, page_fetched, page_failed,
// crawl_complete or, when the crawl stops on an error (billing, auth or
// rate limit, ctx), crawl_error with Err set.
type CrawlProgressEvent struct {
	Type     string
	URL      *string
	Page     *int
	Total    *int
	Success  *bool
	Error    *string
	Metadata map[string]interface{}
	Err      error
}

// CrawlOrchestratorOptions configures the high-level Crawl() and CrawlStream() methods.
type CrawlOrchestratorOptions struct {
	// MaxDepth and MaxPages are sent only when set; otherwise the API
	// applies the tier's defaults.
	MaxDepth int
	MaxPages int
	// ExtractSchema is the shorthand {field: "css:selector"} ("css:" is
	// optional); ExtractFields is the explicit list. Either one extracts
	// each fetched page with Kolektr.
	ExtractSchema map[string]string
	ExtractFields []SchemaField
	// ExtractItemSelector extracts one record per repeated item on each
	// page (schema_spec.item_selector; see ExtractOptions.ItemSelector).
	// The records are in CrawlPage.Records (0.6.0).
	ExtractItemSelector  string
	IncludeExternalLinks bool
	SessionArtifactID    string
	// Scope, Budget and CaptureMode are the crawl policy (see CrawlOptions).
	Scope       map[string]interface{}
	Budget      map[string]interface{}
	CaptureMode string
}
