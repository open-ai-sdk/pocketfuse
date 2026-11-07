package store

import (
	"encoding/json"
	"strings"
)

// llmExchange is what one model call's payload says, parsed tolerantly: senders
// shape these differently, and anything unrecognized is simply left empty.
type llmExchange struct {
	System       string
	UserText     string
	HasMessages  bool
	ToolResults  map[string]toolResult
	Text         string
	ToolCalls    []ToolCall
	FinishReason string
}

type toolResult struct {
	Text  string
	Error string
}

func parseLLMExchange(input, output json.RawMessage) llmExchange {
	var exchange llmExchange
	exchange.ToolResults = map[string]toolResult{}
	parseLLMInput(input, &exchange)
	parseLLMOutput(output, &exchange)
	return exchange
}

func parseLLMInput(raw json.RawMessage, exchange *llmExchange) {
	var messages []json.RawMessage
	if len(raw) == 0 {
		return
	}
	if json.Unmarshal(raw, &messages) != nil {
		var envelope struct {
			Messages []json.RawMessage `json:"messages"`
			System   json.RawMessage   `json:"system"`
		}
		if json.Unmarshal(raw, &envelope) != nil {
			return
		}
		messages = envelope.Messages
		exchange.System = contentText(envelope.System)
	}
	for _, rawMessage := range messages {
		var message struct {
			Role       string          `json:"role"`
			Content    json.RawMessage `json:"content"`
			Name       string          `json:"name"`
			ToolCallID string          `json:"tool_call_id"`
		}
		if json.Unmarshal(rawMessage, &message) != nil || message.Role == "" {
			continue
		}
		exchange.HasMessages = true
		switch message.Role {
		case "system", "developer":
			exchange.System = firstNonEmpty(exchange.System, contentText(message.Content))
		case "tool":
			if message.ToolCallID != "" {
				exchange.ToolResults[message.ToolCallID] = resultFromValue(message.Content)
			}
		case "user":
			collectToolResults(message.Content, exchange.ToolResults)
			if text := contentText(message.Content); text != "" {
				exchange.UserText = text
			}
		}
	}
}

// contentText joins the text parts of a message; tool results and calls are not
// text a person wrote, so they contribute nothing.
func contentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) != nil {
		return ""
	}
	pieces := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.Text != "" && (part.Type == "" || part.Type == "text" || part.Type == "input_text" || part.Type == "output_text") {
			pieces = append(pieces, part.Text)
		}
	}
	return strings.Join(pieces, "\n")
}

func collectToolResults(raw json.RawMessage, results map[string]toolResult) {
	var parts []struct {
		Type    string          `json:"type"`
		Call    string          `json:"call"`
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(raw, &parts) != nil {
		return
	}
	for _, part := range parts {
		if part.Type == "toolresult" && part.Call != "" {
			results[part.Call] = resultFromValue(part.Content)
		}
	}
}

// resultFromValue reads a tool result as text. Results arrive as a list of
// {type, value|text} parts or as a bare value, and failures as an `error` field.
func resultFromValue(raw json.RawMessage) toolResult {
	var parts []struct {
		Value json.RawMessage `json:"value"`
		Text  string          `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil && len(parts) > 0 {
		if len(parts[0].Value) > 0 {
			raw = parts[0].Value
		} else if parts[0].Text != "" {
			return toolResult{Text: parts[0].Text}
		}
	}
	result := toolResult{Text: rawText(raw)}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) == nil {
		result.Error = rawText(fields["error"])
	}
	return result
}

// rawText is a JSON value as readable text: strings unquoted, null empty.
func rawText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	return string(raw)
}

func parseLLMOutput(raw json.RawMessage, exchange *llmExchange) {
	if len(raw) == 0 {
		return
	}
	var fields struct {
		Text         json.RawMessage   `json:"text"`
		ToolCalls    []json.RawMessage `json:"tool_calls"`
		Choice       json.RawMessage   `json:"choice"`
		Content      json.RawMessage   `json:"content"`
		FinishReason string            `json:"finish_reason"`
		Final        struct {
			FinishReason string `json:"finish_reason"`
		} `json:"final"`
	}
	if json.Unmarshal(raw, &fields) != nil {
		// A bare string, or a list of messages or content parts.
		exchange.Text = firstNonEmpty(contentText(raw), messagesText(raw))
		return
	}
	exchange.Text = firstNonEmpty(rawText(fields.Text), contentText(fields.Choice), contentText(fields.Content))
	exchange.FinishReason = firstNonEmpty(fields.Final.FinishReason, fields.FinishReason)
	for _, rawCall := range fields.ToolCalls {
		if call := parseToolCall(rawCall); call.Name != "" {
			exchange.ToolCalls = append(exchange.ToolCalls, call)
		}
	}
	exchange.ToolCalls = append(exchange.ToolCalls, choiceToolCalls(fields.Choice)...)
}

func messagesText(raw json.RawMessage) string {
	var messages []struct {
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(raw, &messages) != nil {
		return ""
	}
	pieces := make([]string, 0, len(messages))
	for _, message := range messages {
		if text := contentText(message.Content); text != "" {
			pieces = append(pieces, text)
		}
	}
	return strings.Join(pieces, "\n")
}

func parseToolCall(raw json.RawMessage) ToolCall {
	var call struct {
		ID       string          `json:"id"`
		Name     string          `json:"name"`
		Args     json.RawMessage `json:"args"`
		Function struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		} `json:"function"`
	}
	if json.Unmarshal(raw, &call) != nil {
		return ToolCall{}
	}
	return ToolCall{
		ID:   call.ID,
		Name: firstNonEmpty(call.Name, call.Function.Name),
		Args: firstNonEmpty(rawText(call.Args), rawText(call.Function.Arguments)),
	}
}

func choiceToolCalls(raw json.RawMessage) []ToolCall {
	var parts []json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &parts) != nil {
		return nil
	}
	var calls []ToolCall
	for _, part := range parts {
		var kind struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(part, &kind) == nil && kind.Type == "toolcall" {
			if call := parseToolCall(part); call.Name != "" {
				calls = append(calls, call)
			}
		}
	}
	return calls
}
