package kloakd

import (
	"context"
	"fmt"
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

	raw, err := n.t.post(ctx, "nexus/analyze", body)
	if err != nil {
		return nil, err
	}
	return parseNexusAnalyzeResult(raw), nil
}

// Synthesize generates an extraction program for a page and a goal.
func (n *NexusNamespace) Synthesize(ctx context.Context, targetURL, html, goal string) (*NexusSynthesisResult, error) {
	raw, err := n.t.post(ctx, "nexus/synthesize", map[string]interface{}{
		"url": targetURL, "html": html, "goal": goal,
	})
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

// Execute runs a verified strategy against a URL.
func (n *NexusNamespace) Execute(ctx context.Context, strategyID, targetURL string, options map[string]interface{}) (*NexusExecuteResult, error) {
	body := map[string]interface{}{
		"strategy_id": strategyID,
		"url":         targetURL,
	}
	if len(options) > 0 {
		body["options"] = options
	}
	raw, err := n.t.post(ctx, "nexus/execute", body)
	if err != nil {
		return nil, err
	}
	return parseNexusExecuteResult(raw), nil
}

// Knowledge stores learned concepts and patterns from an execution result.
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

// RecommendAnalyze analyzes and gets recommendations.
func (n *NexusNamespace) RecommendAnalyze(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
	return n.t.post(ctx, "nexus/recommendations/analyze", data)
}

// ListRecommendationApplications lists recommendation applications.
func (n *NexusNamespace) ListRecommendationApplications(ctx context.Context) (map[string]interface{}, error) {
	return n.t.get(ctx, "nexus/recommendations/applications", nil)
}

// GetCacheStatistics gets recommendation cache statistics.
func (n *NexusNamespace) GetCacheStatistics(ctx context.Context) (map[string]interface{}, error) {
	return n.t.get(ctx, "nexus/recommendations/cache/statistics", nil)
}

// CleanupCache cleans up recommendation cache.
func (n *NexusNamespace) CleanupCache(ctx context.Context) (map[string]interface{}, error) {
	return n.t.post(ctx, "nexus/recommendations/cache/cleanup", map[string]interface{}{})
}

// InvalidateCache invalidates recommendation cache.
func (n *NexusNamespace) InvalidateCache(ctx context.Context) (map[string]interface{}, error) {
	return n.t.post(ctx, "nexus/recommendations/cache/invalidate", map[string]interface{}{})
}

// GetHooksStatus gets hooks status.
func (n *NexusNamespace) GetHooksStatus(ctx context.Context) (map[string]interface{}, error) {
	return n.t.get(ctx, "nexus/recommendations/hooks/status", nil)
}

// EnableHook enables a recommendation hook.
func (n *NexusNamespace) EnableHook(ctx context.Context, hookName string) (map[string]interface{}, error) {
	return n.t.post(ctx, fmt.Sprintf("nexus/recommendations/hooks/%s/enable", hookName), map[string]interface{}{})
}

// DisableHook disables a recommendation hook.
func (n *NexusNamespace) DisableHook(ctx context.Context, hookName string) (map[string]interface{}, error) {
	return n.t.post(ctx, fmt.Sprintf("nexus/recommendations/hooks/%s/disable", hookName), map[string]interface{}{})
}

// CreatePreference creates a recommendation preference.
func (n *NexusNamespace) CreatePreference(ctx context.Context, preference map[string]interface{}) (map[string]interface{}, error) {
	return n.t.post(ctx, "nexus/recommendations/preferences", preference)
}

// GetPreferences gets user preferences.
func (n *NexusNamespace) GetPreferences(ctx context.Context, userID string) (map[string]interface{}, error) {
	return n.t.get(ctx, fmt.Sprintf("nexus/recommendations/preferences/%s", userID), nil)
}

// UpdatePreference updates a recommendation preference.
func (n *NexusNamespace) UpdatePreference(ctx context.Context, preferenceID string, data map[string]interface{}) (map[string]interface{}, error) {
	return n.t.put(ctx, fmt.Sprintf("nexus/recommendations/preferences/%s", preferenceID), data)
}

// DeletePreference deletes a recommendation preference.
func (n *NexusNamespace) DeletePreference(ctx context.Context, preferenceID string) error {
	return n.t.delete(ctx, fmt.Sprintf("nexus/recommendations/preferences/%s", preferenceID))
}

// GetRecommendationStatistics gets recommendation statistics.
func (n *NexusNamespace) GetRecommendationStatistics(ctx context.Context) (map[string]interface{}, error) {
	return n.t.get(ctx, "nexus/recommendations/statistics", nil)
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
