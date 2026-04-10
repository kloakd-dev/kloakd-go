// Quickstart: Discover and extract from a site in ~10 lines.
//
// This scenario is identical across all 4 KLOAKD SDKs per the design document §13.
//
//	go run ./examples/quickstart/main.go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	kloakd "github.com/kloakd-dev/kloakd-go"
)

func main() {
	client, err := kloakd.New(kloakd.Config{
		APIKey:         os.Getenv("KLOAKD_API_KEY"),
		OrganizationID: os.Getenv("KLOAKD_ORG_ID"),
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	targetURL := "https://books.toscrape.com"

	// Step 1: Anti-bot fetch
	fetch, err := client.Evadr.Fetch(ctx, targetURL, nil)
	if err != nil {
		log.Fatalf("evadr.Fetch: %v", err)
	}
	bypassed := "no"
	if fetch.AntiBotBypassed {
		bypassed = "yes"
	}
	fmt.Printf("Fetched via tier %d, anti-bot bypassed=%s\n", fetch.TierUsed, bypassed)

	// Step 2: Site hierarchy (reuse Evadr artifact)
	crawlOpts := &kloakd.CrawlOptions{MaxDepth: 2, MaxPages: 50}
	if fetch.ArtifactID != nil {
		crawlOpts.SessionArtifactID = *fetch.ArtifactID
	}
	crawl, err := client.Webgrph.Crawl(ctx, targetURL, crawlOpts)
	if err != nil {
		log.Fatalf("webgrph.Crawl: %v", err)
	}
	fmt.Printf("Found %d pages\n", crawl.TotalPages)

	// Step 3: Extract structured data (reuse Evadr artifact)
	pageOpts := &kloakd.PageOptions{
		Schema: map[string]interface{}{
			"title": "css:h3 a",
			"price": "css:p.price_color",
		},
	}
	if fetch.ArtifactID != nil {
		pageOpts.FetchArtifactID = *fetch.ArtifactID
	}
	data, err := client.Kolektr.Page(ctx, targetURL, pageOpts)
	if err != nil {
		log.Fatalf("kolektr.Page: %v", err)
	}
	fmt.Printf("Extracted %d records\n", data.TotalRecords)

	limit := 3
	if len(data.Records) < limit {
		limit = len(data.Records)
	}
	for _, record := range data.Records[:limit] {
		fmt.Printf("  %v\n", record)
	}
}
