package kloakd

import (
	"context"
	"net/http"
	"testing"
)

func TestNexus_Analyze_SendsHTML(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST nexus/analyze": jsonHandler(200, map[string]interface{}{
			"url": "https://example.com", "page_type": "listing", "confidence": 0.9,
			"strategy": map[string]interface{}{"type": "css"}, "perception_id": "p1", "reasoning_id": "r1",
			"artifact": map[string]interface{}{"artifact_id": "art-n"},
		}),
	})
	defer server.Close()

	result, err := newTestClient(server).Nexus.Analyze(context.Background(), "https://example.com", "<html/>", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.PageType != "listing" || result.Confidence != 0.9 || result.PerceptionID != "p1" || *result.ArtifactID != "art-n" {
		t.Errorf("unexpected result: %+v", result)
	}
	if rs.bodies["POST nexus/analyze"]["html"] != "<html/>" {
		t.Error("html not sent")
	}
}

func TestNexus_Synthesize(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST nexus/synthesize": jsonHandler(200, map[string]interface{}{
			"url": "https://example.com", "goal": "prices", "program": "extract(...)",
			"strategy_id": "st-1", "strategy_name": "css", "synthesis_time_ms": float64(40),
		}),
	})
	defer server.Close()

	result, err := newTestClient(server).Nexus.Synthesize(context.Background(), "https://example.com", "<html/>", "prices")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.StrategyID != "st-1" || result.Program != "extract(...)" || result.SynthesisTimeMs != 40 {
		t.Errorf("unexpected result: %+v", result)
	}
	body := rs.bodies["POST nexus/synthesize"]
	if body["goal"] != "prices" || body["html"] != "<html/>" {
		t.Errorf("unexpected body: %v", body)
	}
}

func TestNexus_VerifyExecuteKnowledge(t *testing.T) {
	_, server := newRouteServer(map[string]http.HandlerFunc{
		"POST nexus/verify": jsonHandler(200, map[string]interface{}{
			"strategy_id": "st-1", "verified": true, "safety_score": 0.95, "violations": []interface{}{},
		}),
		"POST nexus/execute": jsonHandler(200, map[string]interface{}{
			"url": "https://example.com", "strategy_id": "st-1", "success": true,
			"records": []interface{}{map[string]interface{}{"a": 1}}, "duration_ms": float64(12),
		}),
		"POST nexus/knowledge": jsonHandler(200, map[string]interface{}{
			"execution_result_id": "ex-1", "learned_concepts": []interface{}{}, "patterns": []interface{}{map[string]interface{}{}},
			"total_patterns": float64(1),
		}),
	})
	defer server.Close()
	client := newTestClient(server)
	ctx := context.Background()

	verify, err := client.Nexus.Verify(ctx, "st-1")
	if err != nil || !verify.Verified || verify.SafetyScore != 0.95 {
		t.Fatalf("verify: %+v %v", verify, err)
	}
	exec, err := client.Nexus.Execute(ctx, "st-1", "https://example.com", nil)
	if err != nil || !exec.Success || len(exec.Records) != 1 || exec.DurationMs != 12 {
		t.Fatalf("execute: %+v %v", exec, err)
	}
	knowledge, err := client.Nexus.Knowledge(ctx, "ex-1")
	if err != nil || knowledge.TotalPatterns != 1 || len(knowledge.Patterns) != 1 {
		t.Fatalf("knowledge: %+v %v", knowledge, err)
	}
}

func TestNexus_Reason(t *testing.T) {
	rs, server := newRouteServer(map[string]http.HandlerFunc{
		"POST nexus/reason": jsonHandler(200, map[string]interface{}{"success": true, "results": []interface{}{"a", "b"},
			"confidence": 0.8, "reasoning_id": "r1"}),
	})
	defer server.Close()
	n := newTestClient(server).Nexus
	r, err := n.Reason(context.Background(), "page_type", []string{"https://example.com"}, &ReasonOptions{TimeoutSecs: 5, PerceptionID: "p1"})
	if err != nil || !r.Success || len(r.Results) != 2 || r.ReasoningID == nil || *r.ReasoningID != "r1" {
		t.Fatalf("unexpected: %+v %v", r, err)
	}
	body := rs.bodies["POST nexus/reason"]
	if body["query_type"] != "page_type" || body["parameters"].([]interface{})[0] != "https://example.com" ||
		body["timeout_secs"] != float64(5) || body["perception_id"] != "p1" {
		t.Errorf("unexpected body: %v", body)
	}
	if _, err := n.Reason(context.Background(), "q", nil, nil); err != nil {
		t.Fatal(err)
	}
	if params, ok := rs.bodies["POST nexus/reason"]["parameters"].([]interface{}); !ok || len(params) != 0 {
		t.Errorf("nil parameters must be sent as []: %v", rs.bodies["POST nexus/reason"])
	}
}
