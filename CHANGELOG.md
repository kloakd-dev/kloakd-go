# Changelog

All notable changes to the KLOAKD Go SDK are documented here.

## [0.5.0] — unreleased

### Breaking changes
- `Parlyr.Chat` / `ChatStream` read the events the API sends
  ({type, content, data, actions, timestamp}). `ChatEvent` is now
  `{Type, Content, Data, Actions, Timestamp, Raw, Err}` (`Event` removed;
  `Data` is the payload's `data`). `ChatTurn` drops `Confidence`, `Tier`,
  `Entities` and `RequiresAction` (never sent) and gains `OK`,
  `Interpretation`, `Results`, `Warnings`, `Errors`, `Actions`, `Events`;
  `Response` joins the text and clarification content with "\n"; `Intent`
  is the interpretation data's `intent` when present (the API sends none
  today). An `error` event sets `OK` false and is collected in `Errors`.
  `ChatStream` ends at `end` (not yielded).
- `FetchEvent` is `{Type, JobID, Status, TierUsed, ArtifactID, Error,
  Pipeline, Raw, Err}` (`Tier`, `Vendor`, `Metadata` removed: never sent).
  `CrawlEvent` is now `PipelineEvent` ({EventType, StageID, Status,
  Message, ProgressPct, Metadata, Error, Sequence, ...}).
- `Evadr.JobEvents` returns `<-chan FetchEvent` and `Webgrph.CrawlEvents`
  `<-chan CrawlEvent` (were raw maps).
- `Nexus.Reason(ctx, queryType, parameters []string, *ReasonOptions)`
  returns `*NexusReasonResult` (was a free map in and out).
- `Skanyr.ListSites`, `SiteResources`, `SiteAPIs` return
  `[]map[string]interface{}` (the API answers arrays); `SiteResources` and
  `SiteAPIs` take options.
- `Webgrph.ReaderView(ctx, url, *ReaderViewOptions)` (was a crawlID string).
- `Fetchyr.CreateMultiSiteWorkflow` takes `*MultiSiteWorkflowOptions`;
  `ListWorkflows` takes `*ListWorkflowsOptions`.
- `Kolektr.ListJobs`, `ListScrapers`, `ListContent`, `GetAPIData`,
  `GetAPIDataPaginated` and `ExtractAllAPIData` take options (paging,
  endpoint_hash, max_records).
- 403: only `not_entitled` and `tier_access_denied` are `NotEntitledError`;
  any other 403 is the new `ForbiddenError` (`errors.As` with a
  `*ForbiddenError` matches both).
- Retries: a 5xx answer or a network error is retried only for GET, HEAD and
  DELETE (POST, PUT and PATCH are no longer re-sent). Network errors on
  GET/HEAD/DELETE are now retried.
- `Client.Crawl` returns the discovery crawl's `Success`/`Error` (was always
  true) and stops on billing, entitlement, authentication and rate-limit
  errors instead of recording them on every page. It no longer sends
  `max_depth` 3 / `max_pages` 100 unless set.
- `CrawlOrchestratorOptions.OutputFormat` removed (never sent).
- `go.mod` declares `go 1.18` (was 1.26.2; the README said 1.21).

### Added
- Default `Timeout` 100 s (was 30 s), above the API's 85 s synchronous wait,
  so the 202 hand-off arrives. SSE streams use the client without a
  `Timeout` (ctx ends them), and a stream read error is delivered as the
  last value's `Err` (was dropped). SSE lines up to 16 MiB.
- `Client.GetHandoff` and `Client.WaitForHandoff` (`GET handoff/{job_id}`);
  a `crawl_timeout` `ApiError` carries the crawl id in `JobID`.
- `ConcurrencyLimitError` (409 `concurrent_limit`).
- `NotEntitledError.Feature`, `CurrentTier`, `RequiredTier`;
  `QuotaExceededError.Tier`; `RateLimitError.QuotaExhausted`, `QuotaUsed`,
  `QuotaLimit`, `UpgradeURL` (the playground quota, not retried).
- 429 `rate_limited` is retried on any method, waiting for Retry-After /
  `retry_after` / `reset_at`; 503 `billing_unavailable` on any method.
- Error messages: a non-JSON body's text; a 422's `detail[]` as
  `field: message; ...`.
- Extraction schemas: the shorthand `{field: "css:selector"}` (prefix
  optional) is sent as `{fields: [{name, selector}]}`, in `Kolektr.Extract`,
  `ExtractHTML` and `Client.Crawl`; `SchemaField` and
  `ExtractOptions.Fields` / `CrawlOrchestratorOptions.ExtractFields` for an
  explicit list.
- `FetchResult` and `ExtractionResult`: `AuthRequired`, `DiscoveryID`.
- Skanyr: `UpdateSite`, `ListResources`, `GetResource`, `GetExtractionText`,
  `GetExtractionStructure`, `ExportSite` (ZIP bytes), `Diff`, `GetFindings`,
  `GetGraph`, `GetGraphStats`, `GetSubgraph`, `SearchSite`, `ListSnapshots`,
  `GetTechnologies`, `GetTechnologyEvidence`, `GetTimeline`,
  `GetTimelineSnapshot`.
- Fetchyr: `ResumeAuthChainCredentials`, `ResumeAuthChainMFA`; multi-site
  workflow `Dependencies`, `Description`, `GlobalConfig`.
- Crawl `Scope` and `Budget` (`CrawlOptions`, `CrawlStreamOptions`,
  `CrawlOrchestratorOptions`, with `CaptureMode`); reader view
  `SessionArtifactID`.
- `User-Agent: kloakd-sdk-go/0.5.0`; `X-Kloakd-SDK: go/0.5.0`.

### Fixed
- `Webgrph.ListSessions(ctx, false)` lists every session (it sent nothing,
  and the API defaults to active only).
- Path arguments are escaped as one segment (IDs, credential and proxy
  names, domains, the api-data endpoint URL).
- `Fetchyr.StoreCredentials` no longer adds `name` to the caller's map.
- README: errors checked in the quickstart, real sync policies, stream and
  event fields that exist.

### Deprecated
- `Skanyr.SetSitePolicy`: use `UpdateSite`.

## [0.4.0] — 2026-10-28

### Added
- Every route that can hand off to a job is followed: past about 85 s
  `Kolektr.Extract`, `Fetchyr.Authenticate`, `DetectForms`, `FillForm`,
  `DetectMFA` and `Nexus.Analyze`, `Synthesize`, `Execute` (as well as
  `Evadr.Fetch`) answer 202 with `job_id` and `poll_url`; the SDK polls
  `poll_url` with backoff (×1.5 from the poll interval, at most 15 s) and
  returns the same typed result as a direct answer. One follower serves all
  nine.
- `Config.PollInterval` / `PollTimeout` (defaults 3 s / 15 min) for every
  follow; `FetchOptions.PollInterval` / `PollTimeout` still override them
  for one fetch.
- `ApiError.JobID`: the job of a `job_failed` / `job_timeout` error. A poll
  that finds the job gone (404, expired after an hour) is `job_failed`; a
  202 without `job_id`/`poll_url` is `unexpected_handoff`.
- `NexusExecuteResult.ExecutionResultID` (`execution_result_id`, the same
  value as the execution's artifact id), the id to pass to `Nexus.Knowledge`.
- `DefaultPollInterval`, `DefaultPollTimeout`, `PollOptions`.

### Changed
- `Evadr.Fetch` follows the 202's `poll_url` rather than building
  `evadr/jobs/{id}` itself (the same route), and its poll waits now grow
  1.5× per poll instead of staying fixed.

### Removed (breaking)
- The 13 `nexus/recommendations/*` methods (`RecommendAnalyze`,
  `ListRecommendationApplications`, `GetCacheStatistics`, `CleanupCache`,
  `InvalidateCache`, `GetHooksStatus`, `EnableHook`, `DisableHook`,
  `CreatePreference`, `GetPreferences`, `UpdatePreference`,
  `DeletePreference`, `GetRecommendationStatistics`) and
  `Kolektr.CreateJob`: the API deletes these routes, which returned invented
  results.
- Kolektr's progress and pipeline methods: `GetJobProgress`,
  `GetJobProgressSummary`, `GetJobProgressEvents`, `GetJobProgressLatest`,
  `ListProgressPhases`, `GetProgressPhase`, `GetProgressPhaseSteps`,
  `GetProgressSummary`, `GetPipelineStream`, `GetPipelineEvents` (routes
  deleted, invented results). `ListJobs` and `GetJob` stay.
- `Webgrph.GetJobTrends`, `GetErrorSummary` (routes deleted) and
  `GetDashboardSummary`, `GetDiscoveryPatterns`, `GetEfficiencyMetrics`,
  `GetSiteMappingTrends`, `GetUserBehaviorInsights` (dashboard-only routes,
  not part of the public API).
- `Evadr.ListProfiles` (route deleted). `ListVendors` stays.

### Fixed
- README: the module path is `github.com/kloakd-dev/kloakd-go` (as in
  `go.mod`).

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
