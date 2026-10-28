package kloakd

import (
	"context"
	"strings"
)

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

// Chat sends a message to a conversation session and collects the turn's
// SSE events until "end" (see ChatTurn). An "error" event does not make
// Chat fail: it is collected in Errors and OK is false. The returned error
// is a request or stream error.
func (p *ParlyrNamespace) Chat(ctx context.Context, sessionID, message string) (*ChatTurn, error) {
	events, err := p.ChatStream(ctx, sessionID, message)
	if err != nil {
		return nil, err
	}

	turn := &ChatTurn{SessionID: sessionID, OK: true}
	var texts []string
	for event := range events {
		if event.Err != nil {
			return nil, event.Err
		}
		turn.Events = append(turn.Events, event)
		if len(event.Actions) > 0 {
			turn.Actions = event.Actions
		}
		content := ""
		if event.Content != nil {
			content = *event.Content
		}
		switch event.Type {
		case "text":
			if content != "" {
				texts = append(texts, content)
			}
		case "clarification":
			if content != "" {
				texts = append(texts, content)
			}
			c := content
			turn.ClarificationNeeded = &c
		case "interpretation":
			if event.Data != nil {
				turn.Interpretation = event.Data
				if intent := strField(event.Data, "intent"); intent != "" {
					turn.Intent = intent
				}
			}
		case "results":
			if event.Data != nil {
				turn.Results = append(turn.Results, event.Data)
			}
		case "warning":
			turn.Warnings = append(turn.Warnings, content)
		case "error":
			turn.OK = false
			turn.Errors = append(turn.Errors, event)
		}
	}
	turn.Response = strings.Join(texts, "\n")
	return turn, nil
}

// ChatStream opens an SSE stream for a conversation turn and yields its
// events as they arrive (see ChatEvent). The channel closes at the API's
// "end" event (not yielded), when the stream ends, or when ctx is
// cancelled; if reading the stream failed, the last event carries Err.
func (p *ParlyrNamespace) ChatStream(ctx context.Context, sessionID, message string) (<-chan ChatEvent, error) {
	body := map[string]interface{}{
		"session_id": sessionID,
		"message":    message,
	}
	ctx, cancel := context.WithCancel(ctx)
	raw, err := p.t.postStream(ctx, "parlyr/chat", body)
	if err != nil {
		cancel()
		return nil, err
	}
	out := make(chan ChatEvent, 16)
	go func() {
		defer cancel() // closes the response once "end" is read
		defer close(out)
		for m := range raw {
			if m.err == nil && m.event == "end" {
				return
			}
			select {
			case out <- parseChatEvent(m):
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// DeleteSession removes a conversation session.
func (p *ParlyrNamespace) DeleteSession(ctx context.Context, sessionID string) error {
	return p.t.delete(ctx, pathf("parlyr/chat/%s", sessionID))
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

func parseChatEvent(m sseMessage) ChatEvent {
	if m.err != nil {
		return ChatEvent{Err: m.err}
	}
	ev := ChatEvent{
		Type:      strField(m.data, "type"),
		Content:   optStrField(m.data, "content"),
		Actions:   sliceMapField(m.data, "actions"),
		Timestamp: strField(m.data, "timestamp"),
		Raw:       m.data,
	}
	if d, ok := m.data["data"].(map[string]interface{}); ok {
		ev.Data = d
	}
	if len(ev.Actions) == 0 {
		ev.Actions = nil
	}
	if ev.Type == "" {
		ev.Type = m.event
	}
	return ev
}
