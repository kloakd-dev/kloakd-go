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
	HTML        string
	Constraints map[string]interface{}
}

// Analyze analyzes a URL using the Nexus perception layer.
func (n *NexusNamespace) Analyze(ctx context.Context, targetURL string, opts *NexusAnalyzeOptions) (*NexusAnalyzeResult, error) {
	body := map[string]interface{}{"url": targetURL}
	if opts != nil {
		if opts.HTML != "" {
			body["html"] = opts.HTML
		}
		if len(opts.Constraints) > 0 {
			body["constraints"] = opts.Constraints
		}
	}

	raw, err := n.t.post(ctx, "nexus/analyze", body)
	if err != nil {
		return nil, err
	}
	return parseNexusAnalyzeResult(raw), nil
}

// NexusSynthesizeOptions configures optional parameters for Nexus.Synthesize.
type NexusSynthesizeOptions struct {
	Strategy string
	Timeout  int
}

// Synthesize generates a scraping strategy from a perception ID.
func (n *NexusNamespace) Synthesize(ctx context.Context, perceptionID string, opts *NexusSynthesizeOptions) (*NexusSynthesisResult, error) {
	body := map[string]interface{}{"perception_id": perceptionID}
	if opts != nil {
		if opts.Strategy != "" {
			body["strategy"] = opts.Strategy
		}
		if opts.Timeout > 0 {
			body["timeout"] = opts.Timeout
		}
	}

	raw, err := n.t.post(ctx, "nexus/synthesize", body)
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
func (n *NexusNamespace) Execute(ctx context.Context, strategyID, targetURL string) (*NexusExecuteResult, error) {
	raw, err := n.t.post(ctx, "nexus/execute", map[string]interface{}{
		"strategy_id": strategyID,
		"url":         targetURL,
	})
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
	strategy := mapField(raw, "strategy")
	return &NexusAnalyzeResult{
		PerceptionID:    strField(raw, "perception_id"),
		Strategy:        strategy,
		PageType:        strField(raw, "page_type"),
		ComplexityLevel: strField(raw, "complexity_level"),
		ArtifactID:      optStrField(raw, "artifact_id"),
		DurationMs:      intField(raw, "duration_ms"),
		Error:           optStrField(raw, "error"),
	}
}

func parseNexusSynthesisResult(raw map[string]interface{}) *NexusSynthesisResult {
	return &NexusSynthesisResult{
		StrategyID:      strField(raw, "strategy_id"),
		StrategyName:    strField(raw, "strategy_name"),
		GeneratedCode:   strField(raw, "generated_code"),
		ArtifactID:      optStrField(raw, "artifact_id"),
		SynthesisTimeMs: intField(raw, "synthesis_time_ms"),
		Error:           optStrField(raw, "error"),
	}
}

func parseNexusVerifyResult(raw map[string]interface{}) *NexusVerifyResult {
	return &NexusVerifyResult{
		VerificationResultID: strField(raw, "verification_result_id"),
		IsSafe:               boolField(raw, "is_safe"),
		RiskScore:            float64Field(raw, "risk_score"),
		SafetyScore:          float64Field(raw, "safety_score"),
		Violations:           strSliceField(raw, "violations"),
		DurationMs:           intField(raw, "duration_ms"),
		Error:                optStrField(raw, "error"),
	}
}

func parseNexusExecuteResult(raw map[string]interface{}) *NexusExecuteResult {
	return &NexusExecuteResult{
		ExecutionResultID: strField(raw, "execution_result_id"),
		Success:           boolField(raw, "success"),
		Records:           sliceMapField(raw, "records"),
		ArtifactID:        optStrField(raw, "artifact_id"),
		DurationMs:        intField(raw, "duration_ms"),
		Error:             optStrField(raw, "error"),
	}
}

func parseNexusKnowledgeResult(raw map[string]interface{}) *NexusKnowledgeResult {
	return &NexusKnowledgeResult{
		LearnedConcepts: sliceMapField(raw, "learned_concepts"),
		LearnedPatterns: sliceMapField(raw, "learned_patterns"),
		DurationMs:      intField(raw, "duration_ms"),
		Error:           optStrField(raw, "error"),
	}
}
