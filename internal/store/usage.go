package store

import (
	"encoding/json"
	"math"
	"strconv"
)

const usageDetailsKey = "langfuse.observation.usage_details"

// genAIUsageKeys is ordered from preferred OTel attributes to legacy aliases.
// When both spellings are reported, input/output_tokens and the scoped
// reasoning.output_tokens take precedence over their older counterparts.
var genAIUsageKeys = []struct{ key, canonical string }{
	{"gen_ai.usage.input_tokens", "input"},
	{"gen_ai.usage.prompt_tokens", "input"},
	{"gen_ai.usage.output_tokens", "output"},
	{"gen_ai.usage.completion_tokens", "output"},
	{"gen_ai.usage.total_tokens", "total"},
	{"gen_ai.usage.cache_read.input_tokens", "cached_input"},
	{"gen_ai.usage.cache_creation.input_tokens", "cache_creation_input"},
	{"gen_ai.usage.reasoning.output_tokens", "reasoning"},
	{"gen_ai.usage.reasoning_tokens", "reasoning"},
	{"gen_ai.usage.tool_use_prompt_tokens", "tool_use_prompt"},
}

// NormalizeUsage flattens the usage shapes senders produce into canonical keys
// (input, output, total, ...). Rows stored by earlier versions keep the raw
// attribute names, so this also runs on read. Unrecognized keys are preserved
// and already-canonical input is returned unchanged.
func NormalizeUsage(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var values map[string]any
	if json.Unmarshal(raw, &values) != nil {
		return raw
	}
	result := make(map[string]any, len(values))
	for key, value := range values {
		if key != usageDetailsKey {
			result[key] = value
		}
	}
	changed := false
	for _, attribute := range genAIUsageKeys {
		value, found := values[attribute.key]
		if !found {
			continue
		}
		changed = true
		delete(result, attribute.key)
		// Explicit canonical usage wins over OTel attributes; the first
		// attribute in the ordered list wins over its legacy aliases.
		if _, exists := result[attribute.canonical]; !exists {
			result[attribute.canonical] = value
		}
	}
	if nested, ok := values[usageDetailsKey]; ok {
		changed = true
		if text, isText := nested.(string); isText {
			_ = json.Unmarshal([]byte(text), &nested)
		}
		if details, isMap := nested.(map[string]any); isMap {
			// Explicit Langfuse details have the highest precedence.
			for key, value := range details {
				result[key] = value
			}
		}
	}
	if !changed {
		return raw
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return raw
	}
	return encoded
}

// usageTokens is the total token count in a usage blob. ok is false when the
// blob reports none, so unknown usage never reads as zero.
func usageTokens(raw json.RawMessage) (total float64, ok bool) {
	var values map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &values) != nil {
		return 0, false
	}
	if value, found := numberIn(values, "total", "totalTokens", "total_tokens"); found {
		return value, true
	}
	input, hasInput := numberIn(values, "input", "promptTokens", "prompt_tokens")
	output, hasOutput := numberIn(values, "output", "completionTokens", "completion_tokens")
	return input + output, hasInput || hasOutput
}

// metadataCost reads the cost the sender reported in `cost_details`: its
// `total`, else the sum of its parts. Absent means unknown, never zero.
func metadataCost(raw json.RawMessage) *float64 {
	var values map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &values) != nil {
		return nil
	}
	details, ok := values["cost_details"].(map[string]any)
	if !ok {
		return nil
	}
	if total, found := numberIn(details, "total"); found {
		return &total
	}
	var sum float64
	var found bool
	for key := range details {
		if value, isNumber := numberIn(details, key); isNumber {
			sum += value
			found = true
		}
	}
	if !found {
		return nil
	}
	return &sum
}

func numberIn(values map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		switch value := values[key].(type) {
		case float64:
			if !math.IsNaN(value) && !math.IsInf(value, 0) {
				return value, true
			}
		case string:
			if parsed, err := strconv.ParseFloat(value, 64); err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0) {
				return parsed, true
			}
		}
	}
	return 0, false
}
