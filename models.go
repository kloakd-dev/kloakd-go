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
	URL        string
	Depth      int
	Title      *string
	StatusCode *int
	Children   []string
}

// CrawlResult is returned by Webgrph.Crawl.
type CrawlResult struct {
	Success         bool
	CrawlID         string
	URL             string
	TotalPages      int
	MaxDepthReached int
	Pages           []PageNode
	HasMore         bool
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

// ApiEndpoint represents a discovered API endpoint.
type ApiEndpoint struct {
	URL        string
	Method     string
	ApiType    string
	Confidence float64
	Parameters map[string]interface{}
}

// DiscoverResult is returned by Skanyr.Discover.
type DiscoverResult struct {
	Success        bool
	DiscoveryID    string
	URL            string
	TotalEndpoints int
	Endpoints      []ApiEndpoint
	HasMore        bool
	Total          int
	ArtifactID     *string
	Error          *string
}

// DiscoverEvent is yielded by Skanyr.DiscoverStream.
type DiscoverEvent struct {
	Type        string
	EndpointURL *string
	ApiType     *string
	Metadata    map[string]interface{}
}

// ─── Nexus ───────────────────────────────────────────────────────────────────

// NexusAnalyzeResult is returned by Nexus.Analyze.
type NexusAnalyzeResult struct {
	PerceptionID    string
	Strategy        map[string]interface{}
	PageType        string
	ComplexityLevel string
	ArtifactID      *string
	DurationMs      int
	Error           *string
}

// NexusSynthesisResult is returned by Nexus.Synthesize.
type NexusSynthesisResult struct {
	StrategyID      string
	StrategyName    string
	GeneratedCode   string
	ArtifactID      *string
	SynthesisTimeMs int
	Error           *string
}

// NexusVerifyResult is returned by Nexus.Verify.
type NexusVerifyResult struct {
	VerificationResultID string
	IsSafe               bool
	RiskScore            float64
	SafetyScore          float64
	Violations           []string
	DurationMs           int
	Error                *string
}

// NexusExecuteResult is returned by Nexus.Execute.
type NexusExecuteResult struct {
	ExecutionResultID string
	Success           bool
	Records           []map[string]interface{}
	ArtifactID        *string
	DurationMs        int
	Error             *string
}

// NexusKnowledgeResult is returned by Nexus.Knowledge.
type NexusKnowledgeResult struct {
	LearnedConcepts []map[string]interface{}
	LearnedPatterns []map[string]interface{}
	DurationMs      int
	Error           *string
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

// SessionResult is returned by Fetchyr.Login.
type SessionResult struct {
	Success       bool
	SessionID     string
	URL           string
	ArtifactID    *string
	ScreenshotURL *string
	Error         *string
}

// FetchyrResult is returned by Fetchyr.Fetch.
type FetchyrResult struct {
	Success           bool
	URL               string
	StatusCode        int
	HTML              *string
	ArtifactID        *string
	SessionArtifactID string
	Error             *string
}

// WorkflowResult is returned by Fetchyr.CreateWorkflow.
type WorkflowResult struct {
	WorkflowID string
	Name       string
	Steps      []map[string]interface{}
	URL        *string
	CreatedAt  string
	Error      *string
}

// ExecutionResult is returned by Fetchyr.ExecuteWorkflow / GetExecution.
type ExecutionResult struct {
	ExecutionID string
	WorkflowID  string
	Status      string
	StartedAt   *string
	CompletedAt *string
	Records     []map[string]interface{}
	Error       *string
}

// FormDetectionResult is returned by Fetchyr.DetectForms.
type FormDetectionResult struct {
	Forms      []map[string]interface{}
	TotalForms int
	Error      *string
}

// MfaDetectionResult is returned by Fetchyr.DetectMFA.
type MfaDetectionResult struct {
	MfaDetected bool
	ChallengeID *string
	MfaType     *string
	Error       *string
}

// MfaResult is returned by Fetchyr.SubmitMFA.
type MfaResult struct {
	Success           bool
	SessionArtifactID *string
	Error             *string
}

// DeduplicationResult is returned by Fetchyr.CheckDuplicates.
type DeduplicationResult struct {
	UniqueRecords  []map[string]interface{}
	DuplicateCount int
	TotalInput     int
	Error          *string
}

// ─── Kolektr ─────────────────────────────────────────────────────────────────

// ExtractionResult is returned by Kolektr.Page and Kolektr.ExtractHTML.
type ExtractionResult struct {
	Success      bool
	URL          string
	Method       string
	Records      []map[string]interface{}
	TotalRecords int
	PagesScraped int
	HasMore      bool
	Total        int
	ArtifactID   *string
	JobID        *string
	Error        *string
}
