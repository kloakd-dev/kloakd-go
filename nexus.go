package kloakd

import (
	"context"
)

// NexusNamespace exposes the Strategy Engine module: Analyze, Synthesize,
// Verify and Reason. Access via client.Nexus.
//
// The API has no execute route (POST nexus/execute was removed), and
// POST nexus/knowledge is not part of the public API, so the SDK has
// neither. Analyze and Synthesize follow a request the API hands off as a
// job (202, after about 85 s) to its result; see Config.PollInterval.
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

// Verify checks the safety of a synthesized strategy.
func (n *NexusNamespace) Verify(ctx context.Context, strategyID string) (*NexusVerifyResult, error) {
	raw, err := n.t.post(ctx, "nexus/verify", map[string]interface{}{"strategy_id": strategyID})
	if err != nil {
		return nil, err
	}
	return parseNexusVerifyResult(raw), nil
}

// ReasonOptions configures optional parameters for Nexus.Reason.
type ReasonOptions struct {
	// TimeoutSecs bounds the reasoning call (timeout_secs); 0 for the API's default.
	TimeoutSecs float64
	// PerceptionID is a perception from a prior Analyze.
	PerceptionID string
}

// Reason runs a logical query on the Strategy Engine: queryType with its
// parameters (a list of strings; nil sends []).
func (n *NexusNamespace) Reason(ctx context.Context, queryType string, parameters []string, opts *ReasonOptions) (*NexusReasonResult, error) {
	if parameters == nil {
		parameters = []string{}
	}
	body := map[string]interface{}{"query_type": queryType, "parameters": parameters}
	if opts != nil {
		if opts.TimeoutSecs > 0 {
			body["timeout_secs"] = opts.TimeoutSecs
		}
		if opts.PerceptionID != "" {
			body["perception_id"] = opts.PerceptionID
		}
	}
	raw, err := n.t.post(ctx, "nexus/reason", body)
	if err != nil {
		return nil, err
	}
	return &NexusReasonResult{
		Success:     boolField(raw, "success"),
		Results:     strSliceField(raw, "results"),
		Confidence:  float64Field(raw, "confidence"),
		ReasoningID: optStrField(raw, "reasoning_id"),
	}, nil
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

		Status:               optStrField(raw, "status"),
		RiskScore:            float64Field(raw, "risk_score"),
		SafetyChecks:         parseNexusSafetyChecks(raw),
		VerificationResultID: optStrField(raw, "verification_result_id"),
		DurationMs:           intField(raw, "duration_ms"),
	}
}

func parseNexusSafetyChecks(raw map[string]interface{}) []NexusSafetyCheck {
	checks := []NexusSafetyCheck{}
	for _, c := range sliceMapField(raw, "safety_checks") {
		checks = append(checks, NexusSafetyCheck{
			Property: strField(c, "property"),
			Passed:   boolField(c, "passed"),
			Details:  strField(c, "details"),
		})
	}
	return checks
}
