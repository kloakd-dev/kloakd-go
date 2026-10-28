# kloakd-go

Official Go SDK for the [KLOAKD API](https://kloakd.dev) — the anti-bot intelligence platform.

## Requirements

- Go 1.18+
- KLOAKD API key and organization ID

## Installation

```bash
go get github.com/kloakd-dev/kloakd-go
```

## Quickstart

```go
package main

import (
    "context"
    "fmt"
    "log"

    kloakd "github.com/kloakd-dev/kloakd-go"
)

func main() {
    client, err := kloakd.New(kloakd.Config{
        APIKey:         "sk-live-...",
        OrganizationID: "your-org-uuid",
    })
    if err != nil {
        log.Fatal(err)
    }

    ctx := context.Background()

    // Anti-bot fetch
    fetch, err := client.Evadr.Fetch(ctx, "https://books.toscrape.com", nil)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Fetched via tier %d\n", fetch.TierUsed)

    // Site crawl (waits for the crawl, then returns every page)
    crawl, err := client.Webgrph.Crawl(ctx, "https://books.toscrape.com", &kloakd.CrawlOptions{
        MaxDepth: 2,
        MaxPages: 50,
    })
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Found %d pages\n", crawl.TotalPages)

    // Extract structured data ({field: "css:selector"} is converted to the
    // API's {"fields": [{"name", "selector"}]})
    data, err := client.Kolektr.Extract(ctx, "https://books.toscrape.com", &kloakd.ExtractOptions{
        SchemaSpec: map[string]interface{}{
            "title": "css:h3 a",
            "price": "css:p.price_color",
        },
    })
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Extracted %d records\n", data.TotalRecords)
}
```

## Configuration

```go
client, err := kloakd.New(kloakd.Config{
    APIKey:         "sk-live-...",   // required
    OrganizationID: "org-uuid",      // required
    BaseURL:        "https://api.kloakd.dev", // optional, default shown
    Timeout:        100 * time.Second,        // optional, default 100 s (per attempt; not applied to SSE streams)
    MaxRetries:     3,                        // optional, default 3
    HTTPClient:     customHTTPClient,         // optional, for testing/proxies
    PollInterval:   3 * time.Second,          // optional: first wait when following a 202
    PollTimeout:    15 * time.Minute,         // optional: longest wait for a handed-off job
})
```

### Long requests (HTTP 202 hand-off)

The API answers synchronous requests within about 85 s (it sits behind
Cloudflare, which closes requests at 100 s). A request still running then
continues as a job: the API answers 202 with a `job_id` and `poll_url`, and
the SDK polls `poll_url` (waits grow 1.5× from `PollInterval`, at most 15 s)
and returns the same typed result a direct answer would have. This applies to
`Evadr.Fetch`, `Kolektr.Extract`, `Fetchyr.Authenticate`, `DetectForms`,
`FillForm`, `DetectMFA` and `Nexus.Analyze`, `Synthesize`, `Execute`. A
failed or expired job is an `*kloakd.ApiError` with `Code` `"job_failed"`; a
job still running at `PollTimeout` has `Code` `"job_timeout"`; `JobID` names
the job. Cancelling `ctx` stops the wait.

The default `Timeout` (100 s) is above the API's 85 s wait, so the 202
arrives before the client gives up. To resume a wait that ended with
`job_timeout`, call `client.WaitForHandoff(ctx, apiErr.JobID, nil)`;
`client.GetHandoff(ctx, jobID)` reads the job's state once
(`GET handoff/{job_id}`).

### Streams

`Evadr.FetchStream` / `JobEvents`, `Webgrph.CrawlStream` / `CrawlEvents`,
`Parlyr.ChatStream` and `Client.CrawlStream` return a channel. Streams are
not bound by `Timeout` (use `ctx`). If reading a stream fails, the last
value on the channel carries `Err`:

```go
for ev := range ch {
    if ev.Err != nil {
        log.Fatal(ev.Err)
    }
    // ...
}
```

## Modules

### `client.Evadr` — Anti-Bot Intelligence

```go
// Fetch a URL through anti-bot bypass
result, err := client.Evadr.Fetch(ctx, url, nil)

// A fetch handed off as a job (HTTP 202) is followed to its result.
// FetchOptions overrides Config.PollInterval / PollTimeout for one call:
fetch, err = client.Evadr.Fetch(ctx, url, &kloakd.FetchOptions{
    PollInterval: 3 * time.Second, PollTimeout: 15 * time.Minute,
})

// Async job
jobID, err := client.Evadr.FetchAsync(ctx, url, nil)

// Stream a fetch's progress (starts an async job, streams its events).
// A job with a live pipeline streams pipeline events (event.Pipeline);
// otherwise Type is STATUS, COMPLETE (TierUsed, ArtifactID), FAILED or ERROR.
ch, err := client.Evadr.FetchStream(ctx, url, nil)
for event := range ch {
    if event.Pipeline != nil {
        fmt.Println(event.Pipeline.EventType, event.Pipeline.Message)
    } else {
        fmt.Println(event.Type, event.Status)
    }
}

// Analyze anti-bot defenses
analysis, err := client.Evadr.Analyze(ctx, url, nil)
```

### `client.Webgrph` — Site Mapping

```go
// Crawl a site: starts the crawl, waits for it, returns every page
crawl, err := client.Webgrph.Crawl(ctx, url, &kloakd.CrawlOptions{MaxDepth: 3, MaxPages: 100})

// Or start it and follow it yourself
started, err := client.Webgrph.StartCrawl(ctx, url, nil)
status, err := client.Webgrph.GetCrawlStatus(ctx, started.CrawlID)

// Crawl policy: scope, budget and capture mode
crawl, err = client.Webgrph.Crawl(ctx, url, &kloakd.CrawlOptions{
    CaptureMode: "content",
    Scope:       map[string]interface{}{"same_domain": true},
    Budget:      map[string]interface{}{"max_resources": 500},
})

// Stream a crawl's pipeline events
ch, err := client.Webgrph.CrawlStream(ctx, url, nil)
for event := range ch {
    fmt.Println(event.EventType, event.StageID, event.Message)
}

// Discovery sessions: active only (true) or all (false)
sessions, err := client.Webgrph.ListSessions(ctx, true)

// Reader view, from a crawl's cache or with an authenticated session
view, err := client.Webgrph.ReaderView(ctx, url, &kloakd.ReaderViewOptions{CrawlID: crawl.CrawlID})
```

### `client.Crawl` — discover, fetch and extract in one call

```go
site, err := client.Crawl(ctx, url, &kloakd.CrawlOrchestratorOptions{
    ExtractSchema: map[string]string{"title": "css:h1"},
})
fmt.Println(site.Success, site.PagesFetched, site.PagesFailed)
```

A page that fails is recorded on the result; billing, entitlement,
authentication and rate-limit errors stop the crawl and are returned.

### `client.Skanyr` — Site Knowledge

```go
site, err := client.Skanyr.CreateSiteFromCrawl(ctx, crawl.CrawlID)
siteID := site["site_id"].(string)
sync, err := client.Skanyr.Sync(ctx, siteID, &kloakd.SyncOptions{Policy: "incremental"})
_, err = client.Skanyr.UpdateSite(ctx, siteID, "conditional") // default sync policy
hits, err := client.Skanyr.Search(ctx, "pricing", &kloakd.SearchOptions{SiteID: siteID})

resources, err := client.Skanyr.SiteResources(ctx, siteID, &kloakd.ResourceListOptions{Limit: 100})
techs, err := client.Skanyr.GetTechnologies(ctx, siteID)
graph, err := client.Skanyr.GetGraph(ctx, siteID, nil)
diff, err := client.Skanyr.Diff(ctx, siteID, "", "")
zip, err := client.Skanyr.ExportSite(ctx, siteID, nil) // ZIP bytes
```

Sync policies are `full`, `incremental`, `conditional` and `head_first`.

Detected APIs, the reader view and discovery sessions are on `client.Webgrph`.

### `client.Nexus` — Strategy Engine

```go
page, _       := client.Evadr.Fetch(ctx, url, nil)
perception, _ := client.Nexus.Analyze(ctx, url, *page.HTML, nil)
strategy, _   := client.Nexus.Synthesize(ctx, url, *page.HTML, "product prices")
verified, _   := client.Nexus.Verify(ctx, strategy.StrategyID)
result, _     := client.Nexus.Execute(ctx, strategy.StrategyID, url, nil)
if result.ExecutionResultID != nil {
    learned, _ := client.Nexus.Knowledge(ctx, *result.ExecutionResultID)
    fmt.Println(learned.TotalPatterns)
}
```

### `client.Parlyr` — Conversational NLP

```go
parse, err := client.Parlyr.Parse(ctx, "scrape example.com", nil)

// Blocking chat: collects the turn's events until "end"
turn, err := client.Parlyr.Chat(ctx, sessionID, "scrape example.com")
fmt.Println(turn.Response)       // the text and clarification content
fmt.Println(turn.Interpretation) // url, entity, fields, summary, ...
for _, r := range turn.Results {
    fmt.Println(r["count"], r["items"])
}
if !turn.OK {
    fmt.Println(*turn.Errors[0].Content) // error events are collected, not returned
}

// SSE streaming: thinking, interpretation, progress, results, error,
// warning, auth_required, mfa_required, clarification, export_ready, text
ch, err := client.Parlyr.ChatStream(ctx, sessionID, message)
for event := range ch {
    fmt.Println(event.Type, event.Content, event.Data)
}
```

### `client.Fetchyr` — RPA & Authentication

```go
_, err := client.Fetchyr.StoreCredentials(ctx, "my-login", map[string]interface{}{"username": "u", "password": "p"})
session, err := client.Fetchyr.Authenticate(ctx, "my-login", loginURL)
page, err    := client.Evadr.Fetch(ctx, protectedURL, &kloakd.FetchOptions{SessionArtifactID: *session.ArtifactID})

// A fetch that meets a login page reports it and starts an auth-chain discovery
if page.AuthRequired {
    status, _ := client.Fetchyr.GetAuthChainStatus(ctx, *page.DiscoveryID)
    _, _ = client.Fetchyr.ResumeAuthChainCredentials(ctx, *page.DiscoveryID, "user", "password", false)
    _, _ = client.Fetchyr.ResumeAuthChainMFA(ctx, *page.DiscoveryID, "123456")
    fmt.Println(status)
}
```

### `client.Kolektr` — Data Extraction

```go
data, err := client.Kolektr.Extract(ctx, url, &kloakd.ExtractOptions{
    SchemaSpec: map[string]interface{}{"title": "css:h1", "price": "css:.price"},
})

// The explicit field list
data, err = client.Kolektr.Extract(ctx, url, &kloakd.ExtractOptions{
    Fields: []kloakd.SchemaField{{Name: "image", Selector: "img", Attribute: "src"}},
})

// From raw HTML
data, err = client.Kolektr.ExtractHTML(ctx, html, url, nil)

// Lists take paging options
jobs, err := client.Kolektr.ListJobs(ctx, &kloakd.ListJobsOptions{Page: 1, Limit: 50, StatusFilter: "completed"})
```

## Error Handling

```go
import "errors"

result, err := client.Evadr.Fetch(ctx, url, nil)
if err != nil {
    var rlErr *kloakd.RateLimitError
    var quotaErr *kloakd.QuotaExceededError
    var payErr *kloakd.PaymentRequiredError
    var authErr *kloakd.AuthenticationError
    var concErr *kloakd.ConcurrencyLimitError
    switch {
    case errors.As(err, &quotaErr):
        log.Printf("GB used up (%.1f of %.1f): buy a top-up", quotaErr.GBUsed, quotaErr.GBLimit)
    case errors.As(err, &payErr):
        log.Printf("payment failed: update it at %s", payErr.UpgradeURL)
    case errors.As(err, &rlErr) && rlErr.QuotaExhausted:
        log.Printf("playground quota used (%d/%d): %s", rlErr.QuotaUsed, rlErr.QuotaLimit, rlErr.UpgradeURL)
    case errors.As(err, &rlErr):
        time.Sleep(time.Duration(rlErr.RetryAfter) * time.Second)
    case errors.As(err, &concErr):
        log.Printf("%d of %d concurrent crawls running; wait for one", concErr.ActiveCrawls, concErr.ConcurrentLimit)
    case errors.As(err, &authErr):
        log.Fatal("invalid API key")
    default:
        log.Printf("error: %v", err)
    }
}
```

**Error types:** `KloakdError` (with `Code`, the API's error word), `AuthenticationError` (401), `PaymentRequiredError` (402), `NotEntitledError` (403 `not_entitled` / `tier_access_denied`, with `Module`, `Feature`, `CurrentTier`, `RequiredTier`), `ForbiddenError` (any other 403; `errors.As` with a `*ForbiddenError` also matches a `NotEntitledError`), `ConcurrencyLimitError` (409 `concurrent_limit`), `QuotaExceededError` (429 `gb_exhausted`), `RateLimitError` (429; `QuotaExhausted` for the playground quota), `UpstreamError` (502), `BillingUnavailableError` (503 `billing_unavailable`), `ApiError` (other 4xx/5xx; `JobID` on `job_failed` / `job_timeout` / `crawl_timeout`). A body that is not JSON becomes the message; a 422's `detail[]` is kept as `field: message; ...`.

## Retry Policy

- 5xx answers (500, 502, 503, 504) and network errors: retried for `GET`, `HEAD` and `DELETE` only; a `POST`, `PUT` or `PATCH` may already have done its work, so it is not sent again
- 429 `rate_limited`: retried on any method, waiting for the `Retry-After` header, else `retry_after`, else `reset_at`
- 503 `billing_unavailable`: retried on any method (the request was not started)
- Not retried: 4xx other than the above, 429 `gb_exhausted` and the playground quota (429 with `retry_after` 0 and no reset; waiting does not clear either)
- Backoff without a hint: `1s × 2^attempt`, capped at 60 s
- Default max retries: 3 (override via `Config.MaxRetries`; negative for none)
- `Config.Timeout` applies to each attempt

## Running the Quickstart

```bash
export KLOAKD_API_KEY=sk-live-...
export KLOAKD_ORG_ID=your-org-uuid
go run ./examples/quickstart/main.go
```

## Running Tests

```bash
go vet ./...
go test ./...
```

## License

MIT
