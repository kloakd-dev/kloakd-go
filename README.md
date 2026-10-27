# kloakd-go

Official Go SDK for the [KLOAKD API](https://kloakd.dev) — the anti-bot intelligence platform.

## Requirements

- Go 1.21+
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
    fmt.Printf("Found %d pages\n", crawl.TotalPages)

    // Extract structured data
    data, err := client.Kolektr.Extract(ctx, "https://books.toscrape.com", &kloakd.ExtractOptions{
        SchemaSpec: map[string]interface{}{
            "title": "css:h3 a",
            "price": "css:p.price_color",
        },
    })
    fmt.Printf("Extracted %d records\n", data.TotalRecords)
}
```

## Configuration

```go
client, err := kloakd.New(kloakd.Config{
    APIKey:         "sk-live-...",   // required
    OrganizationID: "org-uuid",      // required
    BaseURL:        "https://api.kloakd.dev", // optional, default shown
    Timeout:        30 * time.Second,         // optional, default 30s
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

// Stream a fetch's progress (starts an async job, streams its events)
ch, err := client.Evadr.FetchStream(ctx, url, nil)
for event := range ch {
    fmt.Println(event.Type, event.Tier)
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

// Stream a crawl's events
ch, err := client.Webgrph.CrawlStream(ctx, url, nil)
for event := range ch {
    fmt.Println(event.Type, event.URL)
}
```

### `client.Skanyr` — Site Knowledge

```go
site, err := client.Skanyr.CreateSiteFromCrawl(ctx, crawl.CrawlID)
sync, err := client.Skanyr.Sync(ctx, siteID, &kloakd.SyncOptions{Policy: "changed"})
hits, err := client.Skanyr.Search(ctx, "pricing", &kloakd.SearchOptions{SiteID: siteID})
```

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

// Blocking chat (collects SSE internally)
turn, err := client.Parlyr.Chat(ctx, sessionID, "scrape example.com")

// SSE streaming
ch, err := client.Parlyr.ChatStream(ctx, sessionID, message)
for event := range ch {
    fmt.Println(event.Event, event.Data)
}
```

### `client.Fetchyr` — RPA & Authentication

```go
_, err := client.Fetchyr.StoreCredentials(ctx, "my-login", map[string]interface{}{"username": "u", "password": "p"})
session, err := client.Fetchyr.Authenticate(ctx, "my-login", loginURL)
page, err    := client.Evadr.Fetch(ctx, protectedURL, &kloakd.FetchOptions{SessionArtifactID: *session.ArtifactID})
```

### `client.Kolektr` — Data Extraction

```go
data, err := client.Kolektr.Extract(ctx, url, &kloakd.ExtractOptions{
    SchemaSpec: map[string]interface{}{"title": "css:h1", "price": "css:.price"},
})

// From raw HTML
data, err = client.Kolektr.ExtractHTML(ctx, html, url, nil)
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
    switch {
    case errors.As(err, &quotaErr):
        log.Printf("GB used up (%.1f of %.1f): buy a top-up", quotaErr.GBUsed, quotaErr.GBLimit)
    case errors.As(err, &payErr):
        log.Printf("payment failed: update it at %s", payErr.UpgradeURL)
    case errors.As(err, &rlErr):
        time.Sleep(time.Duration(rlErr.RetryAfter) * time.Second)
    case errors.As(err, &authErr):
        log.Fatal("invalid API key")
    default:
        log.Printf("error: %v", err)
    }
}
```

**Error types:** `KloakdError` (with `Code`, the API's error word), `AuthenticationError` (401), `PaymentRequiredError` (402), `NotEntitledError` (403), `QuotaExceededError` (429 `gb_exhausted`), `RateLimitError` (429), `UpstreamError` (502), `BillingUnavailableError` (503 `billing_unavailable`), `ApiError` (other 4xx/5xx).

## Retry Policy

- Retryable status codes: 429, 500, 502, 503, 504
- Strategy: exponential backoff — `1s × 2^attempt`, capped at 60s
- 429 and 503: respect the `Retry-After` header / `retry_after` field
- Not retried: 400, 401, 402, 403, 404, and 429 `gb_exhausted` (waiting does not clear it)
- Default max retries: 3 (override via `Config.MaxRetries`; negative for none)

## Running the Quickstart

```bash
export KLOAKD_API_KEY=sk-live-...
export KLOAKD_ORG_ID=your-org-uuid
go run ./examples/quickstart/main.go
```

## Running Tests

```bash
go test . -v
go test . -cover
```

## License

MIT
