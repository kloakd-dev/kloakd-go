package kloakd

// ─── Evadr ───────────────────────────────────────────────────────────────────

// FetchResult is returned by Evadr.Fetch.
type FetchResult struct {
	Success         bool
	URL             string
	StatusCode      int
	TierUsed        int
	HTML            *string
	VendorDetected  *string
	AntiBotBypassed bool
	ArtifactID      *string
	Error           *string
}

// FetchEvent is yielded by Evadr.FetchStream.
type FetchEvent struct {
	Type     string
	Tier     *int
	Vendor   *string
	Metadata map[string]interface{}
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

// CrawlEvent is yielded by Webgrph.CrawlStream.
type CrawlEvent struct {
	Type       string
	URL        *string
	Depth      *int
	PagesFound *int
	Metadata   map[string]interface{}
}

// ─── Skanyr ──────────────────────────────────────────────────────────────────

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
}

// NexusKnowledgeResult is returned by Nexus.Knowledge.
type NexusKnowledgeResult struct {
	LearnedConcepts []map[string]interface{}
	Patterns        []map[string]interface{}
	TotalPatterns   int
	DurationMs      int
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

// ChatTurn is returned by Parlyr.Chat.
type ChatTurn struct {
	SessionID           string
	Intent              string
	Confidence          float64
	Tier                int
	Response            string
	Entities            map[string]interface{}
	RequiresAction      bool
	ClarificationNeeded *string
}

// ChatEvent is yielded by Parlyr.ChatStream.
type ChatEvent struct {
	Event string
	Data  map[string]interface{}
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
}

// Ok returns true if the page was fetched successfully.
func (p CrawlPage) Ok() bool {
	return p.Success && p.Error == nil
}

// SiteCrawlResult is returned by Client.Crawl.
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

// CrawlProgressEvent is yielded by Client.CrawlStream.
type CrawlProgressEvent struct {
	Type     string
	URL      *string
	Page     *int
	Total    *int
	Success  *bool
	Error    *string
	Metadata map[string]interface{}
}

// CrawlOrchestratorOptions configures the high-level Crawl() and CrawlStream() methods.
type CrawlOrchestratorOptions struct {
	MaxDepth             int
	MaxPages             int
	ExtractSchema        map[string]string
	OutputFormat         string
	IncludeExternalLinks bool
	SessionArtifactID    string
}
