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
}

// CrawlResult is returned by Webgrph.Crawl.
type CrawlResult struct {
	Success         bool
	CrawlID         string
	URL             string
	Status          string
	TotalPages      int
	MaxDepthReached int
	Pages           []PageNode
	Total           int
	ArtifactID      *string
	Error           *string
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
}

// NexusExecuteResult is returned by Nexus.Execute.
type NexusExecuteResult struct {
	StrategyID string
	Success    bool
	Records    []map[string]interface{}
	ArtifactID *string
	DurationMs int
	Error      *string
	// ExecutionResultID identifies this execution; pass it to
	// Nexus.Knowledge. Nil when the API did not return one.
	ExecutionResultID *string
}

// NexusKnowledgeResult is returned by Nexus.Knowledge.
type NexusKnowledgeResult struct {
	LearnedConcepts []map[string]interface{}
	Patterns        []map[string]interface{}
	TotalPatterns   int
	DurationMs      int
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
}

// ─── High-level orchestrator ─────────────────────────────────────────────────

// CrawlPage represents a single page result from the high-level Crawl() orchestrator.
type CrawlPage struct {
	Success        bool
	URL            string
	StatusCode     int
	TierUsed       int
	HTML           *string
	StructuredData map[string]interface{}
	ArtifactID     *string
	Error          *string
	// ExtractionError is the error of the page's extraction, when
	// ExtractSchema / ExtractFields was given and it failed.
	ExtractionError *string
}

// Ok returns true if the page was fetched successfully.
func (p CrawlPage) Ok() bool {
	return p.Success && p.Error == nil
}

// SiteCrawlResult is returned by Client.Crawl. Success and Error are the
// discovery crawl's own.
type SiteCrawlResult struct {
	Success              bool
	URL                  string
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
	ExtractSchema        map[string]string
	ExtractFields        []SchemaField
	IncludeExternalLinks bool
	SessionArtifactID    string
	// Scope, Budget and CaptureMode are the crawl policy (see CrawlOptions).
	Scope       map[string]interface{}
	Budget      map[string]interface{}
	CaptureMode string
}
