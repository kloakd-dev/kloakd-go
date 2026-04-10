# kloakd-go

Official Go SDK for the [KLOAKD API](https://kloakd.dev) — the anti-bot intelligence platform.

## Requirements

- Go 1.21+
- KLOAKD API key and organization ID

## Installation

```bash
go get github.com/kloakd/kloakd-go
```

## Quickstart

```go
package main

import (
    "context"
    "fmt"
    "log"

    kloakd "github.com/kloakd/kloakd-go"
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

    // Site crawl (reuse fetch artifact)
    crawl, err := client.Webgrph.Crawl(ctx, "https://books.toscrape.com", &kloakd.CrawlOptions{
        MaxDepth: 2,
        MaxPages: 50,
    })
    fmt.Printf("Found %d pages\n", crawl.TotalPages)

    // Extract structured data
    data, err := client.Kolektr.Page(ctx, "https://books.toscrape.com", &kloakd.PageOptions{
        Schema: map[string]interface{}{
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
})
```

## Modules

### `client.Evadr` — Anti-Bot Intelligence

```go
// Fetch a URL through anti-bot bypass
result, err := client.Evadr.Fetch(ctx, url, nil)

// Async job
jobID, err := client.Evadr.FetchAsync(ctx, url, nil)

// SSE streaming
ch, err := client.Evadr.FetchStream(ctx, url, nil)
for event := range ch {
    fmt.Println(event.Type, event.Tier)
}

// Analyze anti-bot defenses
analysis, err := client.Evadr.Analyze(ctx, url, nil)
```

### `client.Webgrph` — Site Mapping

```go
// Crawl site hierarchy
crawl, err := client.Webgrph.Crawl(ctx, url, &kloakd.CrawlOptions{MaxDepth: 3, MaxPages: 100})

// Auto-paginate all pages
pages, err := client.Webgrph.CrawlAll(ctx, url, nil)

// SSE streaming
ch, err := client.Webgrph.CrawlStream(ctx, url, nil)
for event := range ch {
    fmt.Println(event.Type, event.URL)
}
```

### `client.Skanyr` — API Discovery

```go
result, err := client.Skanyr.Discover(ctx, url, nil)
endpoints, err := client.Skanyr.DiscoverAll(ctx, url, nil)

ch, err := client.Skanyr.DiscoverStream(ctx, url, nil)
for event := range ch {
    fmt.Println(event.Type, event.EndpointURL)
}
```

### `client.Nexus` — Strategy Engine

```go
perception, _ := client.Nexus.Analyze(ctx, url, nil)
strategy, _   := client.Nexus.Synthesize(ctx, perception.PerceptionID, nil)
verified, _   := client.Nexus.Verify(ctx, strategy.StrategyID)
result, _     := client.Nexus.Execute(ctx, strategy.StrategyID, url)
knowledge, _  := client.Nexus.Knowledge(ctx, result.ExecutionResultID)
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
session, err := client.Fetchyr.Login(ctx, loginURL, "#user", "#pass", "u", "p", nil)
page, err    := client.Fetchyr.Fetch(ctx, protectedURL, *session.ArtifactID, nil)
```

### `client.Kolektr` — Data Extraction

```go
data, err := client.Kolektr.Page(ctx, url, &kloakd.PageOptions{
    Schema: map[string]interface{}{"title": "css:h1", "price": "css:.price"},
})

// Auto-paginate
records, err := client.Kolektr.PageAll(ctx, url, nil)

// From raw HTML
data, err = client.Kolektr.ExtractHTML(ctx, html, url, nil)
```

## Error Handling

```go
import "errors"

result, err := client.Evadr.Fetch(ctx, url, nil)
if err != nil {
    var rlErr *kloakd.RateLimitError
    var authErr *kloakd.AuthenticationError
    switch {
    case errors.As(err, &rlErr):
        time.Sleep(time.Duration(rlErr.RetryAfter) * time.Second)
    case errors.As(err, &authErr):
        log.Fatal("invalid API key")
    default:
        log.Printf("error: %v", err)
    }
}
```

**Error types:** `KloakdError`, `AuthenticationError` (401), `NotEntitledError` (403), `RateLimitError` (429), `UpstreamError` (502), `ApiError` (other 4xx/5xx).

## Retry Policy

- Retryable status codes: 429, 500, 502, 503, 504
- Strategy: exponential backoff — `1s × 2^attempt`, capped at 60s
- 429: respects `Retry-After` header / `retry_after` body field
- Non-retryable: 400, 401, 403, 404
- Default max retries: 3 (override via `Config.MaxRetries`)

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
