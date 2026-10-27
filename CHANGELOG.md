# Changelog

All notable changes to the KLOAKD Go SDK are documented here.

## [0.3.0] — 2026-10-27

Every call now targets a live API route (`ROUTES.md`; a contract test checks
each call against `testdata/routes.tsv`). Of 125 calls in 0.2.0, 78 were
right, 32 had a wrong path, fields or behaviour, and 15 had no live route.

### Added
- `Evadr.Fetch` follows a hand-off: past about 85 s the API answers 202 with
  a job; the SDK polls `evadr/jobs/{id}` and returns the job's result.
  `FetchOptions.PollInterval` / `PollTimeout` (defaults 3 s / 15 min) and the
  context bound the wait.
- Typed errors: `PaymentRequiredError` (402), `QuotaExceededError` (429
  `gb_exhausted`, not retried), `BillingUnavailableError` (503
  `billing_unavailable`, honours `Retry-After`); every error has `Code`, the
  API's error word.
- `Webgrph.StartCrawl`, `CancelCrawl`, `CrawlEvents` (stream), and the
  session, detected-API and reader-view methods (moved from Skanyr).
- `Evadr.Profile`, `Evadr.JobEvents` (stream); `Fetchyr.Authenticate`,
  `ResolveMFA`, `ResumeExecution`, `DiscoverAuthChain`, `GetAuthChainStatus`;
  `Kolektr.Extract`; Skanyr's site API (`ListSites`, `CreateSiteFromCrawl`,
  `GetSite`, `SetSitePolicy`, `DeleteSite`, `SiteResources`, `SiteAPIs`,
  `Sync`, `Search`, `Extract`).

### Changed (breaking)
- `Webgrph.Crawl` waits for the crawl (crawls run asynchronously) and
  returns every page, read by cursor; `CrawlOptions` loses `Limit`/`Offset`
  and gains `PollInterval`/`PollTimeout`, `FastMode`, `CaptureMode`.
  `PageNode` follows the API's node (`ID`, `ParentURL`, `Category`,
  `Metadata`; `Children` from `children_urls`).
- `Evadr.FetchStream` starts an async fetch and streams its job events (no
  POST stream route exists). `Evadr.Scan(ctx, url)` takes the URL only.
- `Fetchyr.CreateWorkflow(ctx, name, steps, *CreateWorkflowOptions)`;
  `ExecuteWorkflow(ctx, id, targetURL, *ExecuteWorkflowOptions)`;
  `FillForm` loses `submit`; `CreateMultiSiteWorkflow(ctx, name, sites)`;
  `CheckDuplicates` → `CheckDuplicate(ctx, url, *CheckDuplicateOptions)`;
  result types follow the API's fields.
- `Kolektr.Page` → `Extract` (`Page` remains as a deprecated alias);
  `ExtractOptions.SchemaSpec` (sent as `schema_spec`); `PageAll` removed
  (extraction is not paginated); `ExtractHTML` posts to `kolektr/extract/html`.
- `Nexus.Analyze(ctx, url, html, opts)`; `Synthesize(ctx, url, html, goal)`;
  `Execute(ctx, strategyID, url, options)`; result types follow the API.
- A JSON array response is returned as `{"items": [...]}` (it was dropped).
- `Config.MaxRetries`: negative means no retries (0 still means the default, 3).

### Removed (no live route)
- `Fetchyr.Login` (use `StoreCredentials` + `Authenticate`), `Fetchyr.Fetch`
  (use `Evadr.Fetch` with `SessionArtifactID`), `Fetchyr.SubmitMFA`.
- `Webgrph.GetHierarchy`, `Webgrph.GetJob`, `Evadr.GetJobEvents` (now `JobEvents`).
- Skanyr's discovery API: `Discover`, `DiscoverAll`, `DiscoverStream`,
  `GetDiscovery`, `GetDiscoveryEvents`, `AnalyzeBundle`, `DiscoverPageLive`,
  `Hierarchy`, `ExpandNode`, `Retry`, `Health`, `GetApiMap`, `GetJob`.

## [0.2.0] — 2026-05-13

### Added
- Full API coverage for all 117 kernel module operations across 7 modules
- **Transport**: `put()` and `patch()` methods for full CRUD HTTP support
- **EvadrNamespace**: `Scan`, `GetJob`, `GetJobEvents`, `ListVendors`, `ListProfiles`, `ListProxies`, `DeleteProxy`
- **FetchyrNamespace**: `StoreCredentials`, `ListCredentials`, `DeleteCredentials`, `ListSessions`, `TerminateSession`, `FillForm`, `ListMFAChallenges`, `GetMFAChallenge`, `GetMFAStatistics`, `ListWorkflows`, `GetWorkflow`, `UpdateWorkflow`, `DeleteWorkflow`, `GetWorkflowStatistics`, `CreateMultiSiteWorkflow`, `CreateDedupSession`, `ListDedupSessions`, `GetDedupSession`, `GetDedupSessionStatistics`, `GetDedupDomainStatistics`
- **KolektrNamespace**: `GetAPIData`, `GetAPIDataPaginated`, `ExtractAllAPIData`, `ListContent`, `GetContent`, `DeleteContent`, `ListJobs`, `CreateJob`, `GetJob`, `GetJobStatus`, `GetJobProgress`, `GetJobProgressEvents`, `GetJobProgressLatest`, `GetJobProgressSummary`, `GetPipelineEvents`, `GetPipelineStream`, `ListProgressPhases`, `GetProgressPhase`, `GetProgressPhaseSteps`, `GetProgressSummary`, `ListScrapers`, `CreateScraper`, `GetScraper`, `UpdateScraper`, `DeleteScraper`
- **NexusNamespace**: `Reason`, `RecommendAnalyze`, `ListRecommendationApplications`, `GetCacheStatistics`, `CleanupCache`, `InvalidateCache`, `GetHooksStatus`, `EnableHook`, `DisableHook`, `CreatePreference`, `GetPreferences`, `UpdatePreference`, `DeletePreference`, `GetRecommendationStatistics`
- **SkanyrNamespace**: `GetDiscovery`, `GetDiscoveryEvents`, `AnalyzeBundle`, `DiscoverPageLive`, `DetectedAPIs`, `Hierarchy`, `ExpandNode`, `ReaderView`, `Retry`, `Health`, `ListSessions`, `SaveSession`, `GetSession`, `DeleteSession`, `EndSession`, `UpdateSessionJob`
- **WebgrphNamespace**: `GetCrawlStatus`, `GetCrawlEvents`, `GetCrawlPages`, `GetDashboardSummary`, `GetErrorSummary`, `GetJobTrends`, `GetDiscoveryPatterns`, `GetEfficiencyMetrics`, `GetSiteMappingTrends`, `GetUserBehaviorInsights`

### Changed
- `CheckDuplicates` endpoint corrected to `fetchyr/deduplication/check`

## [0.1.0] — 2026-04-09

### Added

- **`kloakd.New(Config)`** — client constructor with validation; `MustNew` variant for panic-on-error
- **`Config`** — `APIKey`, `OrganizationID`, `BaseURL`, `Timeout`, `MaxRetries`, `HTTPClient`
- **Transport layer** (`transport.go`) — shared HTTP client with:
  - Retry with exponential backoff (base 1s, cap 60s, max 3 retries by default)
  - `Retry-After` header / body field respected for 429
  - Retryable: 429, 500, 502, 503, 504 — Non-retryable: 400, 401, 403, 404
  - Auth headers: `Authorization`, `X-Kloakd-Organization`, `X-Kloakd-SDK: go/0.1.0`
  - SSE streaming via `transport.stream` (data-only) and `transport.streamWithEvents` (event+data)
- **Error hierarchy** — `KloakdError`, `AuthenticationError`, `NotEntitledError`, `RateLimitError`, `UpstreamError`, `ApiError`
- **`EvadrNamespace`** — `Fetch`, `FetchAsync`, `FetchStream`, `Analyze`, `StoreProxy`
- **`WebgrphNamespace`** — `Crawl`, `CrawlAll`, `CrawlStream`, `GetHierarchy`, `GetJob`
- **`SkanyrNamespace`** — `Discover`, `DiscoverAll`, `DiscoverStream`, `GetApiMap`, `GetJob`
- **`NexusNamespace`** — `Analyze`, `Synthesize`, `Verify`, `Execute`, `Knowledge`
- **`ParlyrNamespace`** — `Parse`, `Chat`, `ChatStream`, `DeleteSession`
- **`FetchyrNamespace`** — `Login`, `Fetch`, `CreateWorkflow`, `ExecuteWorkflow`, `GetExecution`, `DetectForms`, `DetectMFA`, `SubmitMFA`, `CheckDuplicates`
- **`KolektrNamespace`** — `Page`, `PageAll`, `ExtractHTML`
- **All pagination** — `*All()` auto-paginators on Webgrph, Skanyr, Kolektr
- **Models** — typed result and event structs for all 7 modules
- **71 tests** — 82.7% statement coverage, all pass; `net/http/httptest` only, zero external test deps
- **`examples/quickstart/main.go`** — canonical 10-line scenario (Evadr → Webgrph → Kolektr)
