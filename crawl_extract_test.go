package kloakd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The crawl-extract job and the exports, checked against kobe-scraper
// 6bd19e9b: modules/webgrph/crawl_extract_router.py and
// kernel/batch_export_router.py.

const ceJob = "b7a1c0de-0000-4000-8000-000000000001"

func ceStatus(status string) map[string]interface{} {
	return map[string]interface{}{
		"job_id": ceJob, "crawl_id": "c-1", "url": "https://shop.example/", "status": status, "cancel_requested": false,
		"progress": map[string]interface{}{"pages_crawled": float64(12), "pages_total": float64(12),
			"pages_extracted": float64(5), "pages_failed": float64(1), "items": float64(40)},
		"error": nil, "error_code": nil, "created_at": "2026-11-12T10:00:00+00:00", "completed_at": nil,
		"links": map[string]interface{}{
			"status": "/s", "results": "/s/results", "pages": "/s/pages", "cancel": "/s/cancel",
			"export": map[string]interface{}{"json": "/s/export?format=json", "csv": "/s/export?format=csv"},
		},
		"pages_by_method": map[string]interface{}{"engine": float64(4), "failed": float64(1)},
		"shapes": []interface{}{map[string]interface{}{"shape": "ab12", "kind": "item", "rows": float64(40),
			"page_types": []interface{}{"listing"}, "fields": []interface{}{"price", "title"}}},
	}
}

func ceRow(i int) map[string]interface{} {
	return map[string]interface{}{"page_url": "https://shop.example/p", "page_type": "listing", "row_kind": "item",
		"item_index": float64(i), "data": map[string]interface{}{"title": fmt.Sprintf("t%d", i)}, "shape": "ab12"}
}

func TestCrawlExtract_Start_SendsTheRequestAndReadsThe202(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST webgrph/crawl-extract": jsonHandler(202, map[string]interface{}{
			"success": true, "job_id": ceJob, "crawl_id": "c-1", "url": "https://shop.example/", "status": "queued",
			"limits": map[string]interface{}{"max_pages": float64(50), "max_depth": float64(3)},
			"links":  ceStatus("queued")["links"],
		}),
	})
	defer server.Close()

	job, err := newTestClient(server).Webgrph.StartCrawlExtract(context.Background(), "https://shop.example/", &CrawlExtractOptions{
		MaxPages: 50, MaxDepth: 3, Scope: map[string]interface{}{"same_host": true},
		Extract: &CrawlExtractSpec{EntityType: "products", Fields: []string{"title", "price"},
			Schema: &CrawlExtractSchema{ItemSelector: "li.product", Fields: []CrawlExtractSelectorField{
				{Name: "title", Selector: "h3"}, {Name: "link", Selector: "a", Attribute: "href"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !job.Success || job.JobID != ceJob || job.CrawlID != "c-1" || job.Status != "queued" || job.MaxPages != 50 ||
		job.MaxDepth != 3 || job.Links.Export["csv"] != "/s/export?format=csv" || job.Links.Results != "/s/results" {
		t.Errorf("unexpected job: %+v", job)
	}
	body := rs.bodies["POST webgrph/crawl-extract"]
	extract, _ := body["extract"].(map[string]interface{})
	schema, _ := extract["schema"].(map[string]interface{})
	fields, _ := schema["fields"].([]interface{})
	if body["url"] != "https://shop.example/" || body["max_pages"] != float64(50) || body["max_depth"] != float64(3) ||
		extract["entity_type"] != "products" || schema["item_selector"] != "li.product" || len(fields) != 2 ||
		fields[1].(map[string]interface{})["attribute"] != "href" {
		t.Errorf("unexpected body: %v", body)
	}
	if _, sent := fields[0].(map[string]interface{})["attribute"]; sent {
		t.Errorf("an unset attribute must not be sent: %v", fields[0])
	}

	if _, err := newTestClient(server).Webgrph.StartCrawlExtract(context.Background(), "https://shop.example/", nil); err != nil {
		t.Fatal(err)
	}
	if b := rs.bodies["POST webgrph/crawl-extract"]; len(b) != 1 || b["url"] != "https://shop.example/" {
		t.Errorf("with no options only the url is sent: %v", b)
	}
}

func TestCrawlExtract_Get_ReadsTheState(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"GET webgrph/crawl-extract/" + ceJob: jsonHandler(200, ceStatus("extracting")),
	})
	defer server.Close()
	s, err := newTestClient(server).Webgrph.GetCrawlExtract(context.Background(), ceJob)
	if err != nil {
		t.Fatal(err)
	}
	if s.Status != "extracting" || s.Finished() || s.Progress != (CrawlExtractProgress{12, 12, 5, 1, 40}) ||
		s.PagesByMethod["engine"] != 4 || len(s.Shapes) != 1 || s.Shapes[0].Rows != 40 || s.Shapes[0].Fields[1] != "title" ||
		s.CompletedAt != nil || s.CreatedAt == nil || s.Error != nil || s.Links.Cancel != "/s/cancel" {
		t.Errorf("unexpected state: %+v", s)
	}
}

func TestCrawlExtract_WaitAndRun(t *testing.T) {
	polls := 0
	final := "completed"
	routes := map[string]http.HandlerFunc{
		"POST webgrph/crawl-extract": jsonHandler(202, map[string]interface{}{"job_id": ceJob, "crawl_id": "c-1", "status": "queued"}),
		"GET webgrph/crawl-extract/" + ceJob: func(w http.ResponseWriter, r *http.Request) {
			polls++
			if polls < 3 {
				jsonHandler(200, ceStatus("crawling"))(w, r)
				return
			}
			body := ceStatus(final)
			if final == "failed" {
				body["error"] = "Crawl failed: blocked"
			}
			jsonHandler(200, body)(w, r)
		},
	}
	_, server := newRouteServer(routes)
	defer server.Close()
	w := newTestClient(server).Webgrph
	ctx := context.Background()
	fast := &CrawlExtractOptions{PollInterval: time.Millisecond}

	s, err := w.CrawlExtract(ctx, "https://shop.example/", fast)
	if err != nil || s.Status != "completed" || !s.Finished() || polls != 3 {
		t.Fatalf("expected the completed job after 3 polls, got %+v %v (polls=%d)", s, err, polls)
	}

	polls, final = 0, "cancelled"
	if s, err = w.CrawlExtract(ctx, "https://shop.example/", fast); err != nil || s.Status != "cancelled" {
		t.Fatalf("a cancelled job is returned, got %+v %v", s, err)
	}

	polls, final = 0, "failed"
	_, err = w.CrawlExtract(ctx, "https://shop.example/", fast)
	var apiErr *ApiError
	if !errorsAs(err, &apiErr) || apiErr.Code != "job_failed" || apiErr.JobID != ceJob || apiErr.Message != "Crawl failed: blocked" {
		t.Fatalf("expected job_failed, got %v", err)
	}

	polls = -1 << 30 // never ends
	_, err = w.WaitForCrawlExtract(ctx, ceJob, &PollOptions{Interval: time.Millisecond, Timeout: 5 * time.Millisecond})
	if !errorsAs(err, &apiErr) || apiErr.Code != "job_timeout" || apiErr.JobID != ceJob {
		t.Fatalf("expected job_timeout, got %v", err)
	}
}

func TestCrawlExtract_Results_PagingAndFilters(t *testing.T) {
	var queries []string
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"GET webgrph/crawl-extract/" + ceJob + "/results": func(w http.ResponseWriter, r *http.Request) {
			queries = append(queries, r.URL.Query().Encode())
			if r.URL.Query().Get("cursor") == "" {
				jsonHandler(200, map[string]interface{}{"job_id": ceJob, "status": "completed", "total": float64(3), "count": float64(2),
					"rows": []interface{}{ceRow(0), ceRow(1)}, "next_cursor": "0:1"})(w, r)
				return
			}
			jsonHandler(200, map[string]interface{}{"job_id": ceJob, "status": "completed", "total": float64(3), "count": float64(1),
				"rows": []interface{}{ceRow(2)}, "next_cursor": nil})(w, r)
		},
	})
	defer server.Close()
	w := newTestClient(server).Webgrph
	ctx := context.Background()

	page, err := w.ListCrawlExtractResults(ctx, ceJob, &CrawlExtractResultsOptions{Limit: 2, Shape: "ab12", PageType: "listing", Kind: "item"})
	if err != nil {
		t.Fatal(err)
	}
	if queries[0] != "kind=item&limit=2&page_type=listing&shape=ab12" {
		t.Errorf("unexpected query: %s", queries[0])
	}
	if page.Total != 3 || page.Count != 2 || page.NextCursor != "0:1" || page.Status != "completed" || len(page.Rows) != 2 ||
		page.Rows[1].ItemIndex != 1 || page.Rows[1].RowKind != "item" || *page.Rows[1].PageType != "listing" ||
		page.Rows[1].Data["title"] != "t1" || page.Rows[1].Shape != "ab12" {
		t.Errorf("unexpected page: %+v", page)
	}

	queries = nil
	rows, err := w.AllCrawlExtractResults(ctx, ceJob, nil)
	if err != nil || len(rows) != 3 || rows[2].ItemIndex != 2 {
		t.Fatalf("expected 3 rows, got %+v %v", rows, err)
	}
	if len(queries) != 2 || queries[0] != "" || queries[1] != "cursor=0%3A1" {
		t.Errorf("unexpected queries: %q", queries)
	}
}

func TestCrawlExtract_LimitIsCheckedBeforeSending(t *testing.T) {
	rs, server := newRouteServer(nil)
	defer server.Close()
	w := newTestClient(server).Webgrph
	_, err := w.ListCrawlExtractResults(context.Background(), ceJob, &CrawlExtractResultsOptions{Limit: CrawlExtractMaxLimit + 1})
	if !errors.Is(err, ErrInvalidCrawlExtractLimit) {
		t.Errorf("expected ErrInvalidCrawlExtractLimit, got %v", err)
	}
	_, err = w.ListCrawlExtractPages(context.Background(), ceJob, &CrawlExtractPagesOptions{Limit: -1})
	if !errors.Is(err, ErrInvalidCrawlExtractLimit) {
		t.Errorf("expected ErrInvalidCrawlExtractLimit, got %v", err)
	}
	if len(rs.seen) != 0 {
		t.Errorf("no request should be sent: %v", rs.seen)
	}
}

func TestCrawlExtract_Pages(t *testing.T) {
	var query string
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"GET webgrph/crawl-extract/" + ceJob + "/pages": func(w http.ResponseWriter, r *http.Request) {
			query = r.URL.Query().Encode()
			jsonHandler(200, map[string]interface{}{"job_id": ceJob, "status": "completed", "count": float64(1), "offset": float64(10),
				"pages": []interface{}{map[string]interface{}{"page_seq": float64(10), "page_url": "https://shop.example/x",
					"page_type": "detail", "title": "X", "status": "failed", "method": nil, "item_count": float64(0),
					"detail": map[string]interface{}{"error": "timeout"}, "extracted_at": "2026-11-12T10:02:00+00:00"}}})(w, r)
		},
	})
	defer server.Close()
	out, err := newTestClient(server).Webgrph.ListCrawlExtractPages(context.Background(), ceJob,
		&CrawlExtractPagesOptions{Limit: 50, Offset: 10, Status: "failed"})
	if err != nil {
		t.Fatal(err)
	}
	if query != "limit=50&offset=10&status=failed" {
		t.Errorf("unexpected query: %s", query)
	}
	p := out.Pages[0]
	if out.Count != 1 || out.Offset != 10 || p.PageSeq != 10 || p.Status != "failed" || p.Method != nil || *p.Title != "X" ||
		p.Detail["error"] != "timeout" || *p.ExtractedAt != "2026-11-12T10:02:00+00:00" {
		t.Errorf("unexpected pages: %+v", out)
	}
}

// exportHandler streams chunks, flushing each, with the export's headers.
func exportHandler(contentType, filename, jobStatus string, chunks ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
		if jobStatus != "" {
			w.Header().Set("X-Job-Status", jobStatus)
		}
		w.WriteHeader(200)
		for _, c := range chunks {
			_, _ = io.WriteString(w, c)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}
}

func TestCrawlExtract_Export_StreamsTheBody(t *testing.T) {
	var query string
	inner := exportHandler("application/x-ndjson", "crawl-extract-"+ceJob+".ndjson", "extracting", "{\"a\":1}\n", "{\"a\":2}\n")
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"GET webgrph/crawl-extract/" + ceJob + "/export": func(w http.ResponseWriter, r *http.Request) {
			query = r.URL.Query().Encode()
			assertHeader(t, r, "Authorization", "Bearer "+testAPIKey)
			inner(w, r)
		},
	})
	defer server.Close()
	w := newTestClient(server).Webgrph

	dl, err := w.ExportCrawlExtract(context.Background(), ceJob,
		&CrawlExtractExportOptions{Format: ExportNDJSON, Shape: "ab12", PageType: "listing", Kind: "item"})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(dl.Body)
	dl.Body.Close()
	if query != "format=ndjson&kind=item&page_type=listing&shape=ab12" {
		t.Errorf("unexpected query: %s", query)
	}
	if string(body) != "{\"a\":1}\n{\"a\":2}\n" || dl.ContentType != "application/x-ndjson" ||
		dl.Filename != "crawl-extract-"+ceJob+".ndjson" || dl.JobStatus != "extracting" {
		t.Errorf("unexpected download: %+v %q", dl, body)
	}

	dl, err = w.ExportCrawlExtract(context.Background(), ceJob, nil)
	if err != nil {
		t.Fatal(err)
	}
	dl.Body.Close()
	if query != "format=json" {
		t.Errorf("the default format is json: %s", query)
	}

	if _, err = w.ExportCrawlExtract(context.Background(), ceJob, &CrawlExtractExportOptions{Format: "xlsx"}); !errors.Is(err, ErrInvalidExportFormat) {
		t.Errorf("expected ErrInvalidExportFormat, got %v", err)
	}
}

// The export is read on the client without a Timeout: a body that takes
// longer than Config.Timeout to arrive is still read to the end.
func TestCrawlExtract_Export_IsNotBoundByTheRequestTimeout(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"GET webgrph/crawl-extract/" + ceJob + "/export": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/x-ndjson")
			w.WriteHeader(200)
			_, _ = io.WriteString(w, "{\"a\":1}\n")
			w.(http.Flusher).Flush()
			time.Sleep(150 * time.Millisecond)
			_, _ = io.WriteString(w, "{\"a\":2}\n")
		},
	})
	defer server.Close()
	client, err := New(Config{APIKey: testAPIKey, OrganizationID: testOrgID, BaseURL: server.URL, MaxRetries: -1,
		Timeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	dl, err := client.Webgrph.ExportCrawlExtract(context.Background(), ceJob, &CrawlExtractExportOptions{Format: ExportNDJSON})
	if err != nil {
		t.Fatal(err)
	}
	defer dl.Body.Close()
	body, err := io.ReadAll(dl.Body)
	if err != nil || string(body) != "{\"a\":1}\n{\"a\":2}\n" {
		t.Fatalf("expected the whole body, got %q %v", body, err)
	}
}

func TestCrawlExtract_Download_WritesTheFile(t *testing.T) {
	csv := []string{"\xef\xbb\xbfpage_url,page_type,row_kind,item_index,title\r\n", "https://shop.example/p,listing,item,0,\"café, bar\"\r\n"}
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"GET webgrph/crawl-extract/" + ceJob + "/export": exportHandler("text/csv; charset=utf-8", "crawl-extract-"+ceJob+".csv", "completed", csv...),
	})
	defer server.Close()
	target := filepath.Join(t.TempDir(), "rows.csv")

	saved, err := newTestClient(server).Webgrph.DownloadCrawlExtract(context.Background(), ceJob, target, &CrawlExtractExportOptions{Format: ExportCSV})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(target)
	want := csv[0] + csv[1]
	if string(got) != want {
		t.Errorf("file holds %q", got)
	}
	if saved.Path != target || saved.Bytes != int64(len(want)) || saved.ContentType != "text/csv; charset=utf-8" ||
		saved.Filename != "crawl-extract-"+ceJob+".csv" || saved.JobStatus != "completed" {
		t.Errorf("unexpected result: %+v", saved)
	}
}

func TestCrawlExtract_Download_ARefusedDownloadCreatesNoFile(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"GET webgrph/crawl-extract/" + ceJob + "/export": jsonHandler(404, map[string]interface{}{"detail": "Crawl-extract job not found"}),
	})
	defer server.Close()
	target := filepath.Join(t.TempDir(), "none.json")
	_, err := newTestClient(server).Webgrph.DownloadCrawlExtract(context.Background(), ceJob, target, nil)
	var apiErr *ApiError
	if !errorsAs(err, &apiErr) || apiErr.StatusCode != 404 {
		t.Fatalf("expected a 404 ApiError, got %v", err)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Errorf("no file should be created: %v", statErr)
	}
}

func TestCrawlExtract_Export_A5xxIsRetried(t *testing.T) {
	calls := 0
	ok := exportHandler("application/json", "x.json", "completed", "[\n]\n")
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"GET webgrph/crawl-extract/" + ceJob + "/export": func(w http.ResponseWriter, r *http.Request) {
			calls++
			if calls == 1 {
				jsonHandler(503, map[string]interface{}{"detail": "temporary"})(w, r)
				return
			}
			ok(w, r)
		},
	})
	defer server.Close()
	client, _ := New(Config{APIKey: testAPIKey, OrganizationID: testOrgID, BaseURL: server.URL, MaxRetries: 2, HTTPClient: server.Client()})
	client.t.retryBase = time.Millisecond
	dl, err := client.Webgrph.ExportCrawlExtract(context.Background(), ceJob, nil)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(dl.Body)
	dl.Body.Close()
	if calls != 2 || string(body) != "[\n]\n" {
		t.Errorf("expected one retry and the body, got calls=%d body=%q", calls, body)
	}
}

func TestCrawlExtract_Cancel(t *testing.T) {
	status := 200
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST webgrph/crawl-extract/" + ceJob + "/cancel": func(w http.ResponseWriter, r *http.Request) {
			if status == 409 {
				jsonHandler(409, map[string]interface{}{"detail": "The job has already ended (completed)"})(w, r)
				return
			}
			jsonHandler(200, map[string]interface{}{"success": true, "job_id": ceJob, "crawl_id": "c-1", "status": "cancelling"})(w, r)
		},
	})
	defer server.Close()
	w := newTestClient(server).Webgrph

	out, err := w.CancelCrawlExtract(context.Background(), ceJob)
	if err != nil || *out != (CrawlExtractCancelResult{true, ceJob, "c-1", "cancelling"}) {
		t.Fatalf("unexpected: %+v %v", out, err)
	}
	if rs.seen[0] != "POST webgrph/crawl-extract/"+ceJob+"/cancel" {
		t.Errorf("unexpected calls: %v", rs.seen)
	}

	status = 409
	_, err = w.CancelCrawlExtract(context.Background(), ceJob)
	var apiErr *ApiError
	if !errorsAs(err, &apiErr) || apiErr.StatusCode != 409 || apiErr.Message != "The job has already ended (completed)" {
		t.Fatalf("expected a 409 ApiError, got %T %v", err, err)
	}
}

func TestBatchJob_ExportAndDownload(t *testing.T) {
	var query string
	inner := exportHandler("application/json", "batch-b-1.json", "",
		"[\n", `{"url":"https://e.com","job_id":"j","status":"completed","row_kind":"item","item_index":0,"data":{"a":1}}`, "\n]\n")
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"GET batch-jobs/b-1/export": func(w http.ResponseWriter, r *http.Request) {
			query = r.URL.Query().Encode()
			inner(w, r)
		},
	})
	defer server.Close()
	client := newTestClient(server)

	dl, err := client.ExportBatchJob(context.Background(), "b-1", ExportCSV)
	if err != nil {
		t.Fatal(err)
	}
	dl.Body.Close()
	if query != "format=csv" || dl.Filename != "batch-b-1.json" || dl.JobStatus != "" {
		t.Errorf("unexpected: query=%s %+v", query, dl)
	}

	target := filepath.Join(t.TempDir(), "batch.json")
	saved, err := client.DownloadBatchJob(context.Background(), "b-1", target, "")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(target)
	if query != "format=json" || saved.Bytes != int64(len(got)) || len(got) == 0 || got[0] != '[' {
		t.Errorf("unexpected: query=%s saved=%+v file=%q", query, saved, got)
	}
	if len(rs.seen) != 2 {
		t.Errorf("unexpected calls: %v", rs.seen)
	}

	if _, err := client.ExportBatchJob(context.Background(), "b-1", "zip"); !errors.Is(err, ErrInvalidExportFormat) {
		t.Errorf("expected ErrInvalidExportFormat, got %v", err)
	}
}
