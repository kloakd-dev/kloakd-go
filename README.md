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
`FillForm`, `DetectMFA` and `Nexus.Analyze`, `Synthesize`. A
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
// crawl.TotalPages and len(crawl.Pages) count fetched pages only

// Or start it and wait for it by id (nil: poll every 3 s, for 60 min)
started, err := client.Webgrph.StartCrawl(ctx, url, nil)
crawl, err = client.Webgrph.WaitForCrawl(ctx, started.CrawlID, nil)
status, err := client.Webgrph.GetCrawlStatus(ctx, started.CrawlID) // the raw status, once

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

// The pages listing, one page at a time (Limit 1-1000, default 100). Only
// fetched pages are listed; IncludeGrouping also returns the tree's
// grouping folders (PageNode.IsGrouping), which were not fetched.
pages, err := client.Webgrph.ListCrawlPages(ctx, crawl.CrawlID, &kloakd.CrawlPagesOptions{Limit: 500})
for pages.NextCursor != "" {
    pages, err = client.Webgrph.ListCrawlPages(ctx, crawl.CrawlID,
        &kloakd.CrawlPagesOptions{Cursor: pages.NextCursor, Limit: 500})
}

// Discovery sessions: active only (true) or all (false)
sessions, err := client.Webgrph.ListSessions(ctx, true)

// Reader view, from a crawl's cache or with an authenticated session
view, err := client.Webgrph.ReaderView(ctx, url, &kloakd.ReaderViewOptions{CrawlID: crawl.CrawlID})
```

#### Held crawls

When the organization's quota runs out, the API pauses a running crawl: its
status becomes `held_quota`. This is a pause, not a failure. `Crawl` and
`WaitForCrawl` stop waiting and return, without an error, a result with
`Status` `"held_quota"`, `Held` true, `Success` false, `Error` nil, `Message`
with the API's text, and no pages.

The crawl resumes by itself, on the same crawl id, when quota returns (an
upgrade, a top-up or the next billing period). Call `WaitForCrawl` with that
id again later. A crawl held for 30 days ends as `failed` with `ErrorCode`
`"held_quota_expired"`.

```go
crawl, err := client.Webgrph.Crawl(ctx, url, nil)
if err != nil {
    return err
}
if crawl.Held {
    fmt.Println(*crawl.Message)
    // later, with the same id:
    crawl, err = client.Webgrph.WaitForCrawl(ctx, crawl.CrawlID, nil)
}
```

`CrawlAll` returns only the pages: for a held crawl (as for a failed or
cancelled one) that is no pages and no error, so use `Crawl` when you need
to tell them apart. `client.Crawl` returns a `SiteCrawlResult` with `Held`
true, `Status`, `Message` and `CrawlID`, and fetches no page. The crawl's
event stream (`CrawlStream`, `CrawlEvents`) ends when the crawl is held.

#### What a crawl did, and continuing it

A crawl can end `completed` without having fetched every page it found.
The result says what happened:

| Field | Meaning |
|---|---|
| `IsPartial` | true: the crawl stopped before asking for every page it found |
| `PartialReason` | `time_budget`, `max_pages` or `error` |
| `PagesDiscovered`, `PagesFetched`, `PagesUnfetched` | pages found, pages fetched, pages asked for and not obtained |
| `Resumable` | true: `ContinueCrawl` can fetch the rest |
| `ErrorCode` | the API's code for a failed crawl |

The counts and `IsPartial` are nil until the crawl has reported them. Each
`PageNode` has `FetchStatus` and `FetchError` when the API sends them.

`ContinueCrawl` is only for a crawl that has ended and whose result has
`Resumable` true (it stopped at its time budget). The crawl keeps its id;
wait for it again. For any other crawl the API answers 409, returned as an
`*kloakd.ApiError`.

```go
if crawl.Resumable {
    if _, err := client.Webgrph.ContinueCrawl(ctx, crawl.CrawlID); err != nil {
        return err
    }
    crawl, err = client.Webgrph.WaitForCrawl(ctx, crawl.CrawlID, nil)
}
```

### Crawl, extract and download in one job

One job crawls a site, extracts every page and keeps the rows for download.
It needs a plan with Kolektr, and a live key (a test key is refused).

```go
job, err := client.Webgrph.StartCrawlExtract(ctx, url, &kloakd.CrawlExtractOptions{
    MaxPages: 200,
    Extract:  &kloakd.CrawlExtractSpec{Fields: []string{"title", "price"}},
})
done, err := client.Webgrph.WaitForCrawlExtract(ctx, job.JobID, nil)
fmt.Println(done.Status, done.Progress.Items, done.Shapes)

// Rows, 100 a page by default (1-1000), following NextCursor
rows, err := client.Webgrph.AllCrawlExtractResults(ctx, job.JobID,
    &kloakd.CrawlExtractResultsOptions{Kind: "item"})

// The whole set as a file (json, ndjson or csv), streamed to disk
saved, err := client.Webgrph.DownloadCrawlExtract(ctx, job.JobID, "rows.csv",
    &kloakd.CrawlExtractExportOptions{Format: kloakd.ExportCSV})
fmt.Println(saved.Bytes, saved.JobStatus)

// Or as a stream
dl, err := client.Webgrph.ExportCrawlExtract(ctx, job.JobID, nil)
defer dl.Body.Close()
```

`CrawlExtract` starts the job and waits for it: a failed job is an
`*kloakd.ApiError` with `Code` `"job_failed"`, a cancelled one is returned.
`ListCrawlExtractPages` lists each page with its status and method;
`CancelCrawlExtract` stops a job (409 once it has ended).

The job's status has a `Crawl` object: what the job's crawl did (`Status`,
`IsPartial`, `PartialReason`, `PagesDiscovered`, `PagesFetched`,
`PagesUnfetched`, `Cancelled`, `RootUnfetched`, `Error`, `ErrorCode`). A
completed job whose crawl is partial holds the rows of the pages that crawl
fetched, not of the whole site.

A crawl-extract job is not itself held, but its crawl can be (see "Held
crawls"). `WaitForCrawlExtract` and `CrawlExtract` then stop waiting and
return the unfinished job, without an error, with `Held` true and
`Crawl.Status` `"held_quota"`. The crawl resumes by itself when quota
returns and the job goes on under the same id; call `WaitForCrawlExtract`
again later.

```go
done, err := client.Webgrph.WaitForCrawlExtract(ctx, job.JobID, nil)
if err == nil && done.Held {
    // not finished: wait again later with the same job id
}
```

#### Export headers

Exports are not bound by `Timeout`; `ctx` ends them. A crawl-extract export
(`ExportCrawlExtract`, `DownloadCrawlExtract`) carries two things read from
the response headers:

- `JobStatus` (`X-Job-Status`): the job's status when the export started.
  An export of a job that is still running, or that failed, holds the rows
  written so far.
- `Crawl` (the `X-Crawl-*` headers): `Status`, `IsPartial`, `PartialReason`,
  `PagesDiscovered`, `PagesFetched`, `PagesUnfetched`, `Cancelled`,
  `RootUnfetched`. Each is a pointer and is nil when the API left its header
  out (the crawl has not reported that value). `Crawl` itself is nil when
  the answer has no `X-Crawl-*` header.

```go
saved, err := client.Webgrph.DownloadCrawlExtract(ctx, job.JobID, "rows.csv",
    &kloakd.CrawlExtractExportOptions{Format: kloakd.ExportCSV})
if saved.Crawl != nil && saved.Crawl.IsPartial != nil && *saved.Crawl.IsPartial {
    fmt.Println("rows of a partial crawl:", *saved.Crawl.PartialReason)
}
```

`client.ExportBatchJob(ctx, batchJobID, format)` and
`client.DownloadBatchJob(ctx, batchJobID, path, format)` download a batch
job's results in the same formats. The batch export sends neither
`X-Job-Status` nor `X-Crawl-*`: `JobStatus` is `""` and `Crawl` is nil.

### `client.Crawl` — discover, fetch and extract in one call

```go
site, err := client.Crawl(ctx, url, &kloakd.CrawlOrchestratorOptions{
    ExtractSchema: map[string]string{"title": "css:h1"},
})
fmt.Println(site.Success, site.PagesFetched, site.PagesFailed)

// One record per repeated item on each page (CrawlPage.Records)
site, err = client.Crawl(ctx, url, &kloakd.CrawlOrchestratorOptions{
    ExtractItemSelector: "article.product_pod",
    ExtractSchema:       map[string]string{"title": "css:h3 a", "price": "css:.price_color"},
})
```

A page that fails is recorded on the result; billing, entitlement,
authentication and rate-limit errors stop the crawl and are returned. When
the discovery crawl is held for quota, the result has `Held` true and no
pages (see "Held crawls").

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
fmt.Println(verified.Verified, verified.SafetyChecks)
```

The API has no execute route, and its knowledge route is not part of the
public API; the SDK has neither.

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

#### Login sessions: your own account

`StartLoginSession` logs in to a site with your own account
(`POST fetchyr/login-sessions`). There are two ways:

- `Method: "interactive"` (the default): KLOAKD opens the site's login in a
  browser. The answer (202) has `Status` `"waiting_for_login"` and a
  `DiscoveryID`; supply the credentials, and an MFA code if asked, with
  `ResumeAuthChainCredentials` and `ResumeAuthChainMFA`.
- `Method: "cookies"`: use a session you already have in your own browser.
  Send its cookies for the site and that browser's user agent (required).
  The answer (201) has `Status` `"ready"` and the `SessionArtifactID`;
  `CookiesKept` and `CookiesDropped` say which cookies were used.

Only the options you set are sent. A request the API refuses is a 422
`*kloakd.ApiError`.

```go
login, err := client.Fetchyr.StartLoginSession(ctx, "https://shop.example/account",
    &kloakd.LoginSessionOptions{
        Method:    "cookies",
        Cookies:   []kloakd.LoginCookie{{Name: "sid", Value: sid, Domain: ".shop.example"}},
        UserAgent: browserUserAgent,
        Country:   "KE",
    })
page, err := client.Evadr.Fetch(ctx, protectedURL,
    &kloakd.FetchOptions{SessionArtifactID: *login.SessionArtifactID})

// The session's state: active, paused (PauseReason, PauseMessage), EXPIRED or INVALIDATED
state, err := client.Fetchyr.GetSession(ctx, *login.SessionArtifactID)
if state.Status == "paused" {
    state, err = client.Fetchyr.ResumeSession(ctx, state.ArtifactID)
}
```

A paused session has `PauseReason` `logged_out`, `rate_limited` or
`challenged`, and a `PauseMessage`. `ResumeSession` puts a paused session back to use; for a session that is not
paused, or that the site logged out (log in again), the API answers 409.
`ListSessions` and `TerminateSession` list and end sessions.

When an extraction meets a login page, `ExtractionResult.AuthRequired` is
true and `LoginOptions` says how to log in with your own account.

### `client.Kolektr` — Data Extraction

```go
data, err := client.Kolektr.Extract(ctx, url, &kloakd.ExtractOptions{
    SchemaSpec: map[string]interface{}{"title": "css:h1", "price": "css:.price"},
})

// The explicit field list
data, err = client.Kolektr.Extract(ctx, url, &kloakd.ExtractOptions{
    Fields: []kloakd.SchemaField{{Name: "image", Selector: "img", Attribute: "src"}},
})

// A list page: one record per repeated item. Each field's selector is read
// inside the item: the first match (every match for Type "list"), its text
// or Attribute; no match is nil; a field without Selector reads the item.
books, err := client.Kolektr.Extract(ctx, "https://books.toscrape.com", &kloakd.ExtractOptions{
    ItemSelector: "article.product_pod",
    Fields: []kloakd.SchemaField{
        {Name: "title", Selector: "h3 a", Attribute: "title"},
        {Name: "price", Selector: ".price_color"},
        {Name: "rating", Selector: "p.star-rating", Attribute: "class"},
    },
})
fmt.Println(books.TotalRecords) // 20 records, one per book

// The shorthand takes the item selector under "item_selector"
books, err = client.Kolektr.Extract(ctx, "https://books.toscrape.com", &kloakd.ExtractOptions{
    SchemaSpec: map[string]interface{}{
        "item_selector": "article.product_pod",
        "title":         "css:h3 a",
        "price":         "css:.price_color",
    },
})

// From raw HTML (ExtractHTMLOptions takes ItemSelector too)
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
        log.Printf("all %d work slots in use (%d crawls, %d requests); try again when one finishes",
            concErr.ConcurrentLimit, concErr.ActiveCrawls, concErr.ActiveRequests)
    case errors.As(err, &authErr):
        log.Fatal("invalid API key")
    default:
        log.Printf("error: %v", err)
    }
}
```

**Error types:** `KloakdError` (with `Code`, the API's error word), `AuthenticationError` (401), `PaymentRequiredError` (402), `NotEntitledError` (403 `not_entitled` / `tier_access_denied`, with `Module`, `Feature`, `CurrentTier`, `RequiredTier`), `ForbiddenError` (any other 403; `errors.As` with a `*ForbiddenError` also matches a `NotEntitledError`), `ConcurrencyLimitError` (409 `concurrent_limit`, with `Tier`, `ConcurrentLimit`, `ActiveCrawls`, `ActiveRequests`; not retried), `QuotaExceededError` (429 `gb_exhausted`), `RateLimitError` (429; `QuotaExhausted` for the playground quota), `UpstreamError` (502), `BillingUnavailableError` (503 `billing_unavailable`), `ApiError` (other 4xx/5xx; `JobID` on `job_failed` / `job_timeout` / `crawl_timeout`). A body that is not JSON becomes the message; a 422's `detail[]` is kept as `field: message; ...`.

## Concurrency (work slots)

Every metered call holds one of the organisation's work slots while it runs; a crawl or batch that is running or queued holds one too. The plan sets how many run at once: Free 2, Starter 5, Pro 10, Pro+ 20, Developer 50, Enterprise 100. A call made when every slot is in use is refused with 409 `concurrent_limit` (`ConcurrencyLimitError`: `ActiveCrawls` are the crawls and batches, `ActiveRequests` the other calls in flight). The SDK does not retry it; keep your own concurrency at or below the plan's limit, or try again when a call finishes.

## Retry Policy

- 5xx answers (500, 502, 503, 504) and network errors: retried for `GET`, `HEAD` and `DELETE` only; a `POST`, `PUT` or `PATCH` may already have done its work, so it is not sent again
- 429 `rate_limited`: retried on any method, waiting for the `Retry-After` header, else `retry_after`, else `reset_at`
- 503 `billing_unavailable`: retried on any method (the request was not started)
- Not retried: 4xx other than the above (including 409 `concurrent_limit`), 429 `gb_exhausted` and the playground quota (429 with `retry_after` 0 and no reset; waiting does not clear either)
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
