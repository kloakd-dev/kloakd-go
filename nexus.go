package kloakd

import (
	"context"
)

// NexusNamespace exposes the Strategy Engine module (5-layer cognitive pipeline).
// Access via client.Nexus.
type NexusNamespace struct {
	t *transport
}

// NexusAnalyzeOptions configures optional parameters for Nexus.Analyze.
type NexusAnalyzeOptions struct {
	Constraints map[string]interface{}
}

// Analyze classifies a page and proposes an extraction strategy. The API
// needs the page's HTML (fetch it first, e.g. with Evadr.Fetch).
func (n *NexusNamespace) Analyze(ctx context.Context, targetURL, html string, opts *NexusAnalyzeOptions) (*NexusAnalyzeResult, error) {
	body := map[string]interface{}{"url": targetURL, "html": html}
	if opts != nil && len(opts.Constraints) > 0 {
		body["constraints"] = opts.Constraints
	}

	raw, err := n.t.postFollowing(ctx, "nexus/analyze", body, nil)
	if err != nil {
		return nil, err
	}
	return parseNexusAnalyzeResult(raw), nil
}

// Synthesize generates an extraction program for a page and a goal.
func (n *NexusNamespace) Synthesize(ctx context.Context, targetURL, html, goal string) (*NexusSynthesisResult, error) {
	raw, err := n.t.postFollowing(ctx, "nexus/synthesize", map[string]interface{}{
		"url": targetURL, "html": html, "goal": goal,
	}, nil)
	if err != nil {
		return nil, err
	}
	return parseNexusSynthesisResult(raw), nil
}

// Verify verifies the safety of a strategy before execution.
func (n *NexusNamespace) Verify(ctx context.Context, strategyID string) (*NexusVerifyResult, error) {
	raw, err := n.t.post(ctx, "nexus/verify", map[string]interface{}{"strategy_id": strategyID})
	if err != nil {
		return nil, err
	}
	return parseNexusVerifyResult(raw), nil
}

// Execute runs a verified strategy against a URL. The result's
// ExecutionResultID is the id to pass to Knowledge.
//
// Analyze, Synthesize and Execute follow a request the API hands off as a job
// (202, after about 85 s) to its result; see Config.PollInterval.
func (n *NexusNamespace) Execute(ctx context.Context, strategyID, targetURL string, options map[string]interface{}) (*NexusExecuteResult, error) {
	body := map[string]interface{}{
		"strategy_id": strategyID,
		"url":         targetURL,
	}
	if len(options) > 0 {
		body["options"] = options
	}
	raw, err := n.t.postFollowing(ctx, "nexus/execute", body, nil)
	if err != nil {
		return nil, err
	}
	return parseNexusExecuteResult(raw), nil
}

// Knowledge stores learned concepts and patterns from an execution result.
// executionResultID is NexusExecuteResult.ExecutionResultID, returned by
// Execute (execution_result_id).
func (n *NexusNamespace) Knowledge(ctx context.Context, executionResultID string) (*NexusKnowledgeResult, error) {
	raw, err := n.t.post(ctx, "nexus/knowledge", map[string]interface{}{
		"execution_result_id": executionResultID,
	})
	if err != nil {
		return nil, err
	}
	return parseNexusKnowledgeResult(raw), nil
}

// Reason performs logical reasoning via the Strategy Engine.
func (n *NexusNamespace) Reason(ctx context.Context, context_ map[string]interface{}) (map[string]interface{}, error) {
	return n.t.post(ctx, "nexus/reason", context_)
}

// ─── parsers ─────────────────────────────────────────────────────────────────

func parseNexusAnalyzeResult(raw map[string]interface{}) *NexusAnalyzeResult {
	return &NexusAnalyzeResult{
		URL:          strField(raw, "url"),
		PerceptionID: strField(raw, "perception_id"),
		ReasoningID:  strField(raw, "reasoning_id"),
		Strategy:     mapField(raw, "strategy"),
		PageType:     strField(raw, "page_type"),
		Confidence:   float64Field(raw, "confidence"),
		ArtifactID:   nestedArtifactID(raw),
	}
}

func parseNexusSynthesisResult(raw map[string]interface{}) *NexusSynthesisResult {
	return &NexusSynthesisResult{
		StrategyID:      strField(raw, "strategy_id"),
		StrategyName:    strField(raw, "strategy_name"),
		StrategyType:    strField(raw, "strategy_type"),
		Program:         strField(raw, "program"),
		Confidence:      float64Field(raw, "confidence"),
		Steps:           sliceMapField(raw, "steps"),
		SynthesisTimeMs: intField(raw, "synthesis_time_ms"),
	}
}

func parseNexusVerifyResult(raw map[string]interface{}) *NexusVerifyResult {
	return &NexusVerifyResult{
		StrategyID:  strField(raw, "strategy_id"),
		Verified:    boolField(raw, "verified"),
		SafetyScore: float64Field(raw, "safety_score"),
		Violations:  strSliceField(raw, "violations"),
	}
}

func parseNexusExecuteResult(raw map[string]interface{}) *NexusExecuteResult {
	return &NexusExecuteResult{
		StrategyID: strField(raw, "strategy_id"),
		Success:    boolField(raw, "success"),
		Records:    sliceMapField(raw, "records"),
		ArtifactID: nestedArtifactID(raw),
		DurationMs: intField(raw, "duration_ms"),
		Error:      optStrField(raw, "error_message"),

		ExecutionResultID: optStrField(raw, "execution_result_id"),
	}
}

func parseNexusKnowledgeResult(raw map[string]interface{}) *NexusKnowledgeResult {
	return &NexusKnowledgeResult{
		LearnedConcepts: sliceMapField(raw, "learned_concepts"),
		Patterns:        sliceMapField(raw, "patterns"),
		TotalPatterns:   intField(raw, "total_patterns"),
		DurationMs:      intField(raw, "duration_ms"),
	}
}
