# Routes called by the Go SDK

## 0.4.0 → 0.5.0

Checked against kobe-scraper e5f15f7d (`modules/*/router.py`, `kernel/sync_handoff.py`). Every call is still in `testdata/routes.tsv` (`TestContract_EverySDKCallIsALiveRoute`); `TestContract_ParityRoutesAreCalled` checks the new calls. Path arguments are now escaped as one segment each (`pathf`).

| Call | 0.5.0 |
|---|---|
| `GET skanyr/resources`, `GET skanyr/resources/{resource_id}` | new: `ListResources` (site_id, resource_type, limit, offset), `GetResource` |
| `GET skanyr/extractions/{content_hash}/text`, `…/structure` | new: `GetExtractionText` (text/plain answer, returned as a string), `GetExtractionStructure` |
| `POST skanyr/sites/{site_id}/export` | new: `ExportSite` (resource_ids, max_bytes); answers a ZIP, returned as bytes |
| `GET skanyr/sites/{site_id}/diff`, `findings`, `graph`, `graph/stats`, `graph/subgraph/{resource_id}`, `search`, `snapshots`, `technologies`, `technology-evidence`, `timeline`, `timeline/{snapshot_id}` | new: `Diff`, `GetFindings`, `GetGraph`, `GetGraphStats`, `GetSubgraph`, `SearchSite`, `ListSnapshots`, `GetTechnologies`, `GetTechnologyEvidence`, `GetTimeline`, `GetTimelineSnapshot` |
| `GET skanyr/sites`, `sites/{id}/resources`, `sites/{id}/apis`, `findings`, `technologies`, `GET skanyr/resources` | answer JSON arrays; returned as `[]map[string]interface{}` |
| `PATCH skanyr/sites/{site_id}` | `UpdateSite(siteID, policy)` (`SetSitePolicy` kept, deprecated) |
| `POST fetchyr/auth-chain/{discovery_id}/resume-credentials`, `…/resume-mfa` | new: `ResumeAuthChainCredentials` {username, password, remember}, `ResumeAuthChainMFA` {mfa_code} |
| `GET handoff/{job_id}` | now public: `Client.GetHandoff`, `Client.WaitForHandoff` |
| `POST nexus/reason` | sends {query_type, parameters: [string], timeout_secs?, perception_id?} (was a free map) |
| `POST fetchyr/multi-site-workflows` | sends description, dependencies [{dependent_site, dependency_site, dependency_type?, condition?, data_requirements?, timeout?}], global_config |
| `GET kolektr/jobs` | page, limit, status_filter |
| `GET kolektr/scrapers` | page, page_size, is_template |
| `GET kolektr/content` | page, page_size, search |
| `GET fetchyr/workflows` | page, page_size |
| `GET kolektr/api-data/{api_endpoint}`, `…/paginated`, `POST …/extract-all` | endpoint_hash; max_records on extract-all; the endpoint URL is sent escaped as one segment |
| `GET webgrph/sessions` | `active_only` always sent (the API's default is true) |
| `POST webgrph/crawl` | adds scope, budget (capture_mode already); `Client.Crawl` no longer sends max_depth 3 / max_pages 100 by default |
| `POST webgrph/reader-view` | adds session_artifact_id |
| `POST kolektr/extract`, `POST kolektr/extract/html` | the `{field: "css:selector"}` shorthand is sent as `{fields: [{name, selector}]}`; results read auth_required, discovery_id |
| `POST evadr/fetch` | results read auth_required, discovery_id |
| `GET evadr/jobs/{id}/events`, `GET webgrph/crawl/{id}/events` | events parsed as the API sends them (pipeline events; Evadr's fallback {type, job_id, status, tier_used, artifact_id, error}) |
| `POST parlyr/chat` | events parsed as {type, content, data, actions, timestamp}; the stream ends at `end` |

## 0.3.0 → 0.4.0

Checked against kobe-scraper's routers on 2026-10-28, including the route removals in kobe-scraper d7d25134 (branch fix/stubs-and-knowledge) (`testdata/routes.tsv` updated; `TestContract_DeletedRoutesAreGoneAndTheHandOffPollIsLive` checks the changes below).

| Call | 0.4.0 |
|---|---|
| `POST kolektr/extract`, `POST fetchyr/authenticate`, `POST fetchyr/form/detect`, `POST fetchyr/form/fill`, `POST fetchyr/mfa/detect`, `POST nexus/analyze`, `POST nexus/synthesize`, `POST nexus/execute` | can answer 202 (`@hands_off_after_limit`, `kernel/sync_handoff.py`); the SDK follows `poll_url` |
| `GET handoff/{job_id}` | new: the hand-off poll route (running / completed with `result` / failed with `error`; 404 when not found or expired) |
| `POST evadr/fetch` | 202 followed through the same follower, via its own `poll_url` (`evadr/jobs/{id}`) |
| `POST nexus/execute` | response gains `execution_result_id` (`NexusExecuteResult.ExecutionResultID`), the id for `POST nexus/knowledge` |
| the 13 `nexus/recommendations/*` routes | deleted from the API (invented results); `RecommendAnalyze`, `ListRecommendationApplications`, `GetCacheStatistics`, `CleanupCache`, `InvalidateCache`, `GetHooksStatus`, `EnableHook`, `DisableHook`, `CreatePreference`, `GetPreferences`, `UpdatePreference`, `DeletePreference`, `GetRecommendationStatistics` removed |
| `POST kolektr/jobs` | deleted from the API; `Kolektr.CreateJob` removed (`GET kolektr/jobs`, `GET kolektr/jobs/{id}` kept) |
| `GET kolektr/jobs/%s/progress`, `…/progress/summary`, `…/progress/events`, `…/progress/latest` | deleted from the API (d7d25134); `GetJobProgress`, `GetJobProgressSummary`, `GetJobProgressEvents`, `GetJobProgressLatest` removed |
| `GET kolektr/progress/phases`, `progress/phases/%s`, `progress/phases/%s/steps`, `progress/summary` | deleted from the API; `ListProgressPhases`, `GetProgressPhase`, `GetProgressPhaseSteps`, `GetProgressSummary` removed |
| `GET kolektr/pipeline/%s/stream`, `pipeline/%s/events` | deleted from the API; `GetPipelineStream`, `GetPipelineEvents` removed |
| `GET webgrph/analytics/job-trends`, `analytics/error-summary` | deleted from the API; `GetJobTrends`, `GetErrorSummary` removed |
| `GET webgrph/analytics/dashboard/summary`, `content/discovery-patterns`, `scraping/efficiency-metrics`, `site-maps/trends`, `users/behavior-insights` | still served, by the dashboard-only sub-router (`modules/webgrph/analytics_router.py`), and left out of the public API (`scripts/generate_public_openapi.py` `EXCLUDED`); `GetDashboardSummary`, `GetDiscoveryPatterns`, `GetEfficiencyMetrics`, `GetSiteMappingTrends`, `GetUserBehaviorInsights` removed (rows stay in the fixture) |
| `GET evadr/profiles` | deleted from the API; `Evadr.ListProfiles` removed (`GET evadr/vendors` kept) |
| standalone `recommendations/*` (13 rows) | dropped from the fixture: that router is not mounted (`api/app.py`, GAP-SEC-6); the SDK never called it |

## 0.2.0 → 0.3.0

The table records 0.3.0 as released; calls removed in 0.4.0 are listed above.

Checked against the backend's route list at kobe-scraper 4fb33ea4 (module routers mounted as `kernel/bootstrap.py` does), copied to `testdata/routes.tsv`; `TestContract_EverySDKCallIsALiveRoute` / the contract test keeps it so. Paths are relative to `/api/v1/organizations/{organization_id}/`.

**Counts:** fixed (fields or behaviour) 17, already right 78, fixed (path) 15, removed 15 (of 125 calls in 0.2.0).

| 0.2.0 call | Class | 0.3.0 |
|---|---|---|
| `POST evadr/fetch` | matches; behaviour fixed | 202 hand-off followed to the job result |
| `POST evadr/fetch/async` | matches a live route | — |
| `POST evadr/fetch/stream` | route removed | FetchStream: POST evadr/fetch/async, then GET evadr/jobs/{id}/events |
| `POST evadr/analyze` | matches a live route | — |
| `POST evadr/scan` | wrong fields | wrong fields: takes url only |
| `GET evadr/jobs/%s` | matches a live route | — |
| `GET evadr/jobs/%s/events` | matches; behaviour fixed | SSE: now a stream (JobEvents) |
| `GET evadr/vendors` | matches a live route | — |
| `GET evadr/profiles` | matches a live route | — |
| `GET evadr/proxies` | matches a live route | — |
| `POST evadr/proxies` | matches a live route | — |
| `DELETE evadr/proxies/%s` | matches a live route | — |
| `POST fetchyr/login` | route removed | Authenticate: POST fetchyr/authenticate {credential_name, target_url} |
| `POST fetchyr/fetch` | route removed | removed; use Evadr fetch with session_artifact_id |
| `POST fetchyr/workflows` | wrong fields | wrong fields: url removed; description, workflow_type; response id |
| `POST fetchyr/workflows/%s/execute` | wrong fields | wrong fields: target_url required; response result |
| `GET fetchyr/workflows/%s/executions/%s` | matches a live route | — |
| `POST fetchyr/forms/detect` | wrong path | POST fetchyr/form/detect |
| `POST fetchyr/mfa/detect` | matches a live route | — |
| `POST fetchyr/mfa/submit` | route removed | ResolveMFA: POST fetchyr/mfa-queue/{id}/resolve {resolution} |
| `POST fetchyr/account/credentials` | matches a live route | — |
| `GET fetchyr/account/credentials` | matches a live route | — |
| `DELETE fetchyr/account/credentials/%s` | matches a live route | — |
| `GET fetchyr/sessions` | matches a live route | — |
| `DELETE fetchyr/sessions/%s` | matches a live route | — |
| `POST fetchyr/form/fill` | wrong fields | wrong fields: submit removed |
| `GET fetchyr/mfa-queue` | matches; behaviour fixed | array response (was dropped): under items |
| `GET fetchyr/mfa/challenges/%s` | matches a live route | — |
| `GET fetchyr/mfa/statistics/%s` | matches a live route | — |
| `GET fetchyr/workflows` | matches a live route | — |
| `GET fetchyr/workflows/%s` | matches a live route | — |
| `PATCH fetchyr/workflows/%s` | matches a live route | — |
| `DELETE fetchyr/workflows/%s` | matches a live route | — |
| `GET fetchyr/workflows/%s/statistics` | matches a live route | — |
| `POST fetchyr/multi-site-workflows` | wrong fields | wrong fields: name required |
| `POST fetchyr/deduplication/check` | wrong fields | wrong fields: url (+content_hash, session_id); response is_duplicate |
| `POST fetchyr/deduplication/sessions` | matches a live route | — |
| `GET fetchyr/deduplication/sessions/active` | matches a live route | — |
| `GET fetchyr/deduplication/sessions/%s` | matches a live route | — |
| `GET fetchyr/deduplication/sessions/%s/statistics` | matches a live route | — |
| `GET fetchyr/deduplication/statistics/%s` | matches a live route | — |
| `POST kolektr/page` | wrong path | Extract: POST kolektr/extract (schema_spec) |
| `POST kolektr/extract-html` | wrong path | POST kolektr/extract/html (schema_spec) |
| `GET kolektr/api-data/%s` | matches a live route | — |
| `GET kolektr/api-data/%s/paginated` | matches a live route | — |
| `POST kolektr/api-data/%s/extract-all` | matches a live route | — |
| `GET kolektr/content` | matches a live route | — |
| `GET kolektr/content/%s` | matches a live route | — |
| `DELETE kolektr/content/%s` | matches a live route | — |
| `GET kolektr/jobs` | matches a live route | — |
| `POST kolektr/jobs` | matches a live route | — |
| `GET kolektr/jobs/%s` | matches a live route | — |
| `GET kolektr/extraction-jobs/%s/status` | matches a live route | — |
| `GET kolektr/jobs/%s/progress` | matches a live route | — |
| `GET kolektr/jobs/%s/progress/events` | matches a live route | — |
| `GET kolektr/jobs/%s/progress/latest` | matches a live route | — |
| `GET kolektr/jobs/%s/progress/summary` | matches a live route | — |
| `GET kolektr/pipeline/%s/events` | matches a live route | — |
| `GET kolektr/pipeline/%s/stream` | matches a live route | — |
| `GET kolektr/progress/phases` | matches a live route | — |
| `GET kolektr/progress/phases/%s` | matches a live route | — |
| `GET kolektr/progress/phases/%s/steps` | matches a live route | — |
| `GET kolektr/progress/summary` | matches a live route | — |
| `GET kolektr/scrapers` | matches a live route | — |
| `POST kolektr/scrapers` | matches a live route | — |
| `GET kolektr/scrapers/%s` | matches a live route | — |
| `PATCH kolektr/scrapers/%s` | matches a live route | — |
| `DELETE kolektr/scrapers/%s` | matches a live route | — |
| `POST nexus/analyze` | wrong fields | wrong fields: html required; response fields |
| `POST nexus/synthesize` | wrong fields | wrong fields: url, html, goal (not perception_id) |
| `POST nexus/verify` | matches; behaviour fixed | response fields: verified, safety_score |
| `POST nexus/execute` | matches; behaviour fixed | response fields; options |
| `POST nexus/knowledge` | matches; behaviour fixed | response fields: patterns, total_patterns |
| `POST nexus/reason` | matches a live route | — |
| `POST nexus/recommendations/analyze` | matches a live route | — |
| `GET nexus/recommendations/applications` | matches a live route | — |
| `GET nexus/recommendations/cache/statistics` | matches a live route | — |
| `POST nexus/recommendations/cache/cleanup` | matches a live route | — |
| `POST nexus/recommendations/cache/invalidate` | matches a live route | — |
| `GET nexus/recommendations/hooks/status` | matches a live route | — |
| `POST nexus/recommendations/hooks/%s/enable` | matches a live route | — |
| `POST nexus/recommendations/hooks/%s/disable` | matches a live route | — |
| `POST nexus/recommendations/preferences` | matches a live route | — |
| `GET nexus/recommendations/preferences/%s` | matches a live route | — |
| `PUT nexus/recommendations/preferences/%s` | matches a live route | — |
| `DELETE nexus/recommendations/preferences/%s` | matches a live route | — |
| `GET nexus/recommendations/statistics` | matches a live route | — |
| `POST parlyr/parse` | matches a live route | — |
| `POST parlyr/chat` | matches a live route | — |
| `POST parlyr/chat` | matches a live route | — |
| `DELETE parlyr/chat/%s` | matches a live route | — |
| `POST skanyr/discover` | route removed | removed (no live route) |
| `POST skanyr/discover/stream` | route removed | removed (no live route) |
| `GET skanyr/discover/%s` | route removed | removed (no live route) |
| `GET skanyr/discover/%s/events` | route removed | removed (no live route) |
| `POST skanyr/analyze-bundle` | route removed | removed (no live route) |
| `POST skanyr/discover-page/live` | route removed | removed (no live route) |
| `GET skanyr/detected-apis` | wrong path | moved to Webgrph: GET webgrph/detected-apis |
| `POST skanyr/hierarchy` | route removed | removed (no live route) |
| `POST skanyr/expand-node` | route removed | removed (no live route) |
| `POST skanyr/reader-view` | wrong path | moved to Webgrph: POST webgrph/reader-view |
| `POST skanyr/retry` | route removed | removed (no live route) |
| `GET skanyr/health` | route removed | removed (no live route) |
| `GET skanyr/sessions` | wrong path | moved to Webgrph: GET webgrph/sessions |
| `POST skanyr/sessions` | wrong path | moved to Webgrph: POST webgrph/sessions |
| `GET skanyr/sessions/%s` | wrong path | moved to Webgrph: GET webgrph/sessions/{id} |
| `DELETE skanyr/sessions/%s` | wrong path | moved to Webgrph: DELETE webgrph/sessions/{id} |
| `POST skanyr/sessions/%s/end` | wrong path | moved to Webgrph: POST webgrph/sessions/{id}/end |
| `PATCH skanyr/sessions/%s/job` | wrong path | moved to Webgrph: PATCH webgrph/sessions/{id}/job?job_id= |
| `GET skanyr/api-map/%s` | route removed | removed (no live route) |
| `GET skanyr/jobs/%s` | route removed | removed (no live route) |
| `POST webgrph/crawl` | matches; behaviour fixed | answers 202; Crawl now waits and reads pages by cursor; limit/offset removed |
| `POST webgrph/crawl/stream` | route removed | CrawlStream: POST webgrph/crawl, then GET webgrph/crawl/{id}/events |
| `GET webgrph/crawl/%s` | matches a live route | — |
| `GET webgrph/crawl/%s/events` | matches; behaviour fixed | SSE: now a stream (CrawlEvents) |
| `GET webgrph/crawl/%s/pages` | matches; behaviour fixed | cursor/limit params |
| `GET webgrph/hierarchy/%s` | route removed | removed (pages: GET webgrph/crawl/{id}/pages) |
| `GET webgrph/jobs/%s` | route removed | removed (status: GET webgrph/crawl/{id}) |
| `GET webgrph/analytics/dashboard/summary` | matches a live route | — |
| `GET webgrph/analytics/error-summary` | matches a live route | — |
| `GET webgrph/analytics/job-trends` | matches a live route | — |
| `GET webgrph/analytics/content/discovery-patterns` | matches a live route | — |
| `GET webgrph/analytics/scraping/efficiency-metrics` | matches a live route | — |
| `GET webgrph/analytics/site-maps/trends` | matches a live route | — |
| `GET webgrph/analytics/users/behavior-insights` | matches a live route | — |
