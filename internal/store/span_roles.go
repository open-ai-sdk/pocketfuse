package store

import (
	"encoding/json"
	"strings"
)

// Span roles say what a span means to a reader, independent of its OTel type.
const (
	RoleModelCall = "model_call"
	RoleTool      = "tool"
	RoleTurn      = "turn"
	RoleWakeTurn  = "wake_turn"
	RoleJob       = "job"
	RoleRequest   = "request"
	RoleSpan      = "span"
)

// spanRole classifies one span and picks the name a reader should see: the
// model of a call, the tool of a tool span, the span name otherwise.
func spanRole(observation *Observation) (role, label string) {
	values := metadataValues(observation.Metadata)
	tool, _ := firstString(values, "tool", "gen_ai.tool.name")
	name := observation.Name
	switch {
	case strings.EqualFold(observation.Type, "GENERATION"):
		return RoleModelCall, firstNonEmpty(observation.Model, name)
	case strings.EqualFold(observation.Type, "TOOL"), tool != "":
		return RoleTool, firstNonEmpty(tool, name)
	case strings.HasSuffix(name, ".wake_turn"):
		return RoleWakeTurn, name
	case strings.HasSuffix(name, ".turn"):
		return RoleTurn, name
	case strings.HasSuffix(name, "job.run"):
		return RoleJob, name
	}
	if _, isHTTP := firstString(values, "http.request.method", "http.route"); isHTTP {
		return RoleRequest, name
	}
	return RoleSpan, name
}

func firstString(values map[string]any, keys ...string) (string, bool) {
	for _, key := range keys {
		if text, ok := values[key].(string); ok && text != "" {
			return text, true
		}
	}
	return "", false
}

// annotateObservations sets the reader-facing fields of a trace's spans. It
// expects effective timing to be applied already, so a folded call keeps the
// duration of the span that measured it.
func annotateObservations(observations []Observation) {
	for position := range observations {
		observations[position].Role, observations[position].Label = spanRole(&observations[position])
	}
	foldModelCalls(observations)
}

// foldModelCalls marks a nested pair recording one provider call as one model
// call. Instrumented clients open a generation holding the payload and an SDK
// span inside it measuring the stream. The pair folds only on evidence that it
// is one call: the parent's single child is a generation of the same model with
// identical input and output token counts and no payload of its own.
func foldModelCalls(observations []Observation) {
	index := make(map[string]int, len(observations))
	childCount := make(map[string]int, len(observations))
	for position := range observations {
		index[observations[position].ID] = position
		childCount[observations[position].ParentObservationID]++
	}
	for position := range observations {
		child := &observations[position]
		parentPosition, found := index[child.ParentObservationID]
		if !found || childCount[child.ParentObservationID] != 1 {
			continue
		}
		parent := &observations[parentPosition]
		if !sameModelCall(parent, child) {
			continue
		}
		child.MergedInto = parent.ID
		parent.MergedSpans = []string{child.ID}
		if isErrorLevel(child.Level) && !isErrorLevel(parent.Level) {
			parent.Level = child.Level
			parent.StatusMessage = firstNonEmpty(parent.StatusMessage, child.StatusMessage)
		}
	}
}

func sameModelCall(parent, child *Observation) bool {
	if !strings.EqualFold(parent.Type, "GENERATION") || !strings.EqualFold(child.Type, "GENERATION") {
		return false
	}
	if parent.Model != child.Model || child.hasPayload || len(child.Input) > 0 || len(child.Output) > 0 {
		return false
	}
	parentUsage, childUsage := readUsage(parent.Usage), readUsage(child.Usage)
	return parentUsage.Input != nil && parentUsage.Output != nil &&
		childUsage.Input != nil && childUsage.Output != nil &&
		*parentUsage.Input == *childUsage.Input && *parentUsage.Output == *childUsage.Output
}

func isErrorLevel(level string) bool {
	return strings.EqualFold(level, "error") || strings.EqualFold(level, "fatal")
}

// TokenUsage holds the token counts a span reported; nil means not reported.
type TokenUsage struct {
	Input       *float64 `json:"input,omitempty"`
	CachedInput *float64 `json:"cachedInput,omitempty"`
	Output      *float64 `json:"output,omitempty"`
	Reasoning   *float64 `json:"reasoning,omitempty"`
	Total       *float64 `json:"total,omitempty"`
}

func readUsage(raw json.RawMessage) TokenUsage {
	var values map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &values) != nil {
		return TokenUsage{}
	}
	pick := func(keys ...string) *float64 {
		if value, found := numberIn(values, keys...); found {
			return &value
		}
		return nil
	}
	usage := TokenUsage{
		Input:       pick("input", "promptTokens", "prompt_tokens"),
		CachedInput: pick("cached_input"),
		Output:      pick("output", "completionTokens", "completion_tokens"),
		Reasoning:   pick("reasoning"),
		Total:       pick("total", "totalTokens", "total_tokens"),
	}
	if total, ok := usageTokens(raw); ok && usage.Total == nil {
		usage.Total = &total
	}
	return usage
}

// add accumulates another usage; a measure stays nil until someone reports it.
func (u *TokenUsage) add(other TokenUsage) {
	merge := func(into **float64, value *float64) {
		if value == nil {
			return
		}
		if *into == nil {
			zero := 0.0
			*into = &zero
		}
		**into += *value
	}
	merge(&u.Input, other.Input)
	merge(&u.CachedInput, other.CachedInput)
	merge(&u.Output, other.Output)
	merge(&u.Reasoning, other.Reasoning)
	merge(&u.Total, other.Total)
}
