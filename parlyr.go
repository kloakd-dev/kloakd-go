package kloakd

import "context"

// ParlyrNamespace exposes the Conversational NLP module.
// Access via client.Parlyr.
type ParlyrNamespace struct {
	t *transport
}

// ParseOptions configures optional parameters for Parlyr.Parse.
type ParseOptions struct {
	SessionID string
}

// Parse interprets a natural language message.
func (p *ParlyrNamespace) Parse(ctx context.Context, message string, opts *ParseOptions) (*ParseResult, error) {
	body := map[string]interface{}{"message": message}
	if opts != nil && opts.SessionID != "" {
		body["session_id"] = opts.SessionID
	}

	raw, err := p.t.post(ctx, "parlyr/parse", body)
	if err != nil {
		return nil, err
	}
	return parseParseResult(raw), nil
}

// Chat sends a message to a conversation session and blocks until the full
// response is collected from the SSE stream.
func (p *ParlyrNamespace) Chat(ctx context.Context, sessionID, message string) (*ChatTurn, error) {
	body := map[string]interface{}{
		"session_id": sessionID,
		"message":    message,
	}

	events, err := p.t.streamWithEvents(ctx, "parlyr/chat", body)
	if err != nil {
		return nil, err
	}

	turn := &ChatTurn{
		SessionID: sessionID,
		Entities:  map[string]interface{}{},
	}
	for event := range events {
		switch event.Event {
		case "intent":
			turn.Intent = strField(event.Data, "intent")
			turn.Confidence = float64Field(event.Data, "confidence")
			turn.Tier = intField(event.Data, "tier")
			if v, ok := event.Data["entities"]; ok {
				if m, ok := v.(map[string]interface{}); ok {
					turn.Entities = m
				}
			}
			turn.RequiresAction = boolField(event.Data, "requires_action")
		case "response":
			turn.Response = strField(event.Data, "content")
		case "clarification":
			s := strField(event.Data, "message")
			turn.ClarificationNeeded = &s
		}
	}
	return turn, nil
}

// ChatStream opens an SSE stream for a conversation turn.
// Returns a read-only channel of ChatEvent values.
func (p *ParlyrNamespace) ChatStream(ctx context.Context, sessionID, message string) (<-chan ChatEvent, error) {
	body := map[string]interface{}{
		"session_id": sessionID,
		"message":    message,
	}
	return p.t.streamWithEvents(ctx, "parlyr/chat", body)
}

// DeleteSession removes a conversation session.
func (p *ParlyrNamespace) DeleteSession(ctx context.Context, sessionID string) error {
	return p.t.delete(ctx, "parlyr/chat/"+sessionID)
}

// ─── parsers ─────────────────────────────────────────────────────────────────

func parseParseResult(raw map[string]interface{}) *ParseResult {
	return &ParseResult{
		Intent:              strField(raw, "intent"),
		Confidence:          float64Field(raw, "confidence"),
		Tier:                intField(raw, "tier"),
		Source:              strField(raw, "source"),
		Entities:            mapField(raw, "entities"),
		RequiresAction:      boolField(raw, "requires_action"),
		ClarificationNeeded: optStrField(raw, "clarification_needed"),
		Reasoning:           optStrField(raw, "reasoning"),
		DetectedURL:         optStrField(raw, "detected_url"),
	}
}
