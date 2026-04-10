# Changelog

All notable changes to the KLOAKD Go SDK are documented here.

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
