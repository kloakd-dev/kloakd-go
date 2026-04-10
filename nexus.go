package kloakd

import "context"

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
