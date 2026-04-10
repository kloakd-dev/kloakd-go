package kloakd

import (
	"context"
	"net/http/httptest"
	"testing"
)

func TestNexus_Analyze(t *testing.T) {
	server := httptest.NewServer(jsonHandler(200, map[string]interface{}{
		"perception_id":    "perc-001",
		"strategy":         map[string]interface{}{"type": "css"},
		"page_type":        "listing",
		"complexity_level": "medium",
		"artifact_id":      "art-nexus-001",
		"duration_ms":      float64(120),
	}))
	defer server.Close()

	client := newTestClient(server)
	result, err := client.Nexus.Analyze(context.Background(), "https://example.com", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.PerceptionID != "perc-001" {
		t.Errorf("expected perc-001, got %s", result.PerceptionID)
	}
	if result.PageType != "listing" {
		t.Errorf("expected listing, got %s", result.PageType)
	}
	if result.DurationMs != 120 {
		t.Errorf("expected 120ms, got %d", result.DurationMs)
	}
}

func TestNexus_Analyze_WithOptions(t *testing.T) {
	cap := &captureHandler{inner: jsonHandler(200, map[string]interface{}{
		"perception_id": "perc-002", "strategy": map[string]interface{}{},
		"page_type": "detail", "complexity_level": "low", "duration_ms": float64(80),
	})}
	server := httptest.NewServer(cap)
	defer server.Close()

	client := newTestClient(server)
	_, err := client.Nexus.Analyze(context.Background(), "https://example.com", &NexusAnalyzeOptions{
		HTML:        "<html>test</html>",
		Constraints: map[string]interface{}{"max_selectors": 10},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cap.lastBody["html"] != "<html>test</html>" {
		t.Errorf("unexpected html: %v", cap.lastBody["html"])
	}
}

func TestNexus_Synthesize(t *testing.T) {
	server := httptest.NewServer(jsonHandler(200, map[string]interface{}{
		"strategy_id":       "strat-001",
		"strategy_name":     "css_extractor",
		"generated_code":    "return document.querySelectorAll('h1')",
		"artifact_id":       "art-strat-001",
		"synthesis_time_ms": float64(250),
	}))
	defer server.Close()

	client := newTestClient(server)
	result, err := client.Nexus.Synthesize(context.Background(), "perc-001", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.StrategyID != "strat-001" {
		t.Errorf("expected strat-001, got %s", result.StrategyID)
	}
	if result.SynthesisTimeMs != 250 {
		t.Errorf("expected 250ms, got %d", result.SynthesisTimeMs)
	}
}

func TestNexus_Verify(t *testing.T) {
	server := httptest.NewServer(jsonHandler(200, map[string]interface{}{
		"verification_result_id": "verify-001",
		"is_safe":                true,
		"risk_score":             float64(0.1),
		"safety_score":           float64(0.95),
		"violations":             []interface{}{},
		"duration_ms":            float64(45),
	}))
	defer server.Close()

	client := newTestClient(server)
	result, err := client.Nexus.Verify(context.Background(), "strat-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsSafe {
		t.Error("expected IsSafe=true")
	}
	if result.RiskScore != 0.1 {
		t.Errorf("expected RiskScore=0.1, got %f", result.RiskScore)
	}
}

func TestNexus_Execute(t *testing.T) {
	server := httptest.NewServer(jsonHandler(200, map[string]interface{}{
		"execution_result_id": "exec-001",
		"success":             true,
		"records":             []interface{}{map[string]interface{}{"title": "Product A", "price": "$10"}},
		"artifact_id":         "art-exec-001",
		"duration_ms":         float64(800),
	}))
	defer server.Close()

	client := newTestClient(server)
	result, err := client.Nexus.Execute(context.Background(), "strat-001", "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExecutionResultID != "exec-001" {
		t.Errorf("expected exec-001, got %s", result.ExecutionResultID)
	}
	if len(result.Records) != 1 {
		t.Errorf("expected 1 record, got %d", len(result.Records))
	}
}

func TestNexus_Knowledge(t *testing.T) {
	server := httptest.NewServer(jsonHandler(200, map[string]interface{}{
		"learned_concepts": []interface{}{map[string]interface{}{"name": "product_listing"}},
		"learned_patterns": []interface{}{map[string]interface{}{"selector": "h3 a"}},
		"duration_ms":      float64(60),
	}))
	defer server.Close()

	client := newTestClient(server)
	result, err := client.Nexus.Knowledge(context.Background(), "exec-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.LearnedConcepts) != 1 {
		t.Errorf("expected 1 concept, got %d", len(result.LearnedConcepts))
	}
	if len(result.LearnedPatterns) != 1 {
		t.Errorf("expected 1 pattern, got %d", len(result.LearnedPatterns))
	}
}
