package store

import (
	"fmt"
	"sort"
	"strings"
)

// Preview bounds a text and records how long the original was.
type Preview struct {
	Text       string `json:"text"`
	TotalChars int    `json:"totalChars"`
}

// Truncated reports whether Text is shorter than the original.
func (p Preview) Truncated() bool { return len([]rune(p.Text)) < p.TotalChars }

// ToolCall is one tool the model asked for, paired with the result the next
// model call received, and with the tool's own span when the sender recorded one.
type ToolCall struct {
	ID         string  `json:"id,omitempty"`
	Name       string  `json:"name"`
	Args       string  `json:"args,omitempty"`
	Result     string  `json:"result,omitempty"`
	Error      string  `json:"error,omitempty"`
	SpanID     string  `json:"spanId,omitempty"`
	DurationMs float64 `json:"durationMs,omitempty"`
	rawResult  string
}

// TurnStep is one thing that happened inside a turn, in start order.
type TurnStep struct {
	ObservationID string  `json:"observationId"`
	Role          string  `json:"role"`
	Label         string  `json:"label"`
	Name          string  `json:"name,omitempty"`
	StartTime     string  `json:"startTime,omitempty"`
	OffsetMs      float64 `json:"offsetMs"`
	DurationMs    float64 `json:"durationMs"`
	Pending       bool    `json:"pending,omitempty"`
	Level         string  `json:"level,omitempty"`
	StatusMessage string  `json:"statusMessage,omitempty"`
	Model         string  `json:"model,omitempty"`
	// Side marks a model call outside the turn's conversation, such as a
	// title generator or a call a tool makes, with a different system prompt.
	Side         bool       `json:"side,omitempty"`
	Usage        TokenUsage `json:"usage"`
	Cost         *float64   `json:"cost,omitempty"`
	FinishReason string     `json:"finishReason,omitempty"`
	Text         *Preview   `json:"text,omitempty"`
	ToolCalls    []ToolCall `json:"toolCalls,omitempty"`
}

// Turn is one trace told as a story: what started it, what ran, what came back.
type Turn struct {
	TraceID       string     `json:"traceId"`
	Kind          string     `json:"kind"`
	Name          string     `json:"name,omitempty"`
	RootID        string     `json:"rootId,omitempty"`
	AgentID       string     `json:"agentId,omitempty"`
	JobID         string     `json:"jobId,omitempty"`
	PlanID        string     `json:"planId,omitempty"`
	StartTime     string     `json:"startTime,omitempty"`
	EndTime       string     `json:"endTime,omitempty"`
	DurationMs    float64    `json:"durationMs"`
	Status        string     `json:"status,omitempty"`
	ErrorCount    int        `json:"errorCount,omitempty"`
	StatusMessage string     `json:"statusMessage,omitempty"`
	Usage         TokenUsage `json:"usage"`
	Cost          *float64   `json:"cost,omitempty"`
	ModelCalls    int        `json:"modelCalls"`
	ToolCallCount int        `json:"toolCalls"`
	// FailedTools counts tool calls whose result carried an error. The span
	// level does not show these: the model saw the error and carried on.
	FailedTools int        `json:"failedTools,omitempty"`
	UserMessage *Preview   `json:"userMessage,omitempty"`
	Reply       *Preview   `json:"reply,omitempty"`
	Steps       []TurnStep `json:"steps"`
	// SpawnedBy is the earlier tool call whose result named this job.
	SpawnedBy *TurnLink `json:"spawnedBy,omitempty"`
	// Overlaps is true when this turn started before the previous one ended.
	Overlaps bool `json:"overlaps,omitempty"`
	results  map[string]toolResult
}

// TurnLink points at the span that caused a turn.
type TurnLink struct {
	TraceID       string `json:"traceId"`
	ObservationID string `json:"observationId"`
	Tool          string `json:"tool"`
}

// Trace detail repeats text the same response holds in full as span payloads,
// so its story keeps enough to read and clips the rest.
var traceDetailTurn = TurnOptions{MessageChars: 8000, DetailChars: 4000}

// TurnOptions bound the text copied into a turn. Filter runs on the full text
// before clipping, so a secret is never left half-visible at a cut.
type TurnOptions struct {
	MessageChars int
	DetailChars  int
	Filter       func(string) string
}

func (o TurnOptions) preview(text string, limit int) *Preview {
	if text == "" {
		return nil
	}
	if o.Filter != nil {
		text = o.Filter(text)
	}
	runes := []rune(text)
	preview := &Preview{Text: text, TotalChars: len(runes)}
	if limit > 0 && len(runes) > limit {
		preview.Text = string(runes[:limit])
	}
	return preview
}

// clip bounds a text that has no field of its own to state its length, so the
// cut is stated in the text.
func (o TurnOptions) clip(text string) string {
	preview := o.preview(text, o.DetailChars)
	if preview == nil {
		return ""
	}
	if preview.Truncated() {
		return fmt.Sprintf("%s… [+%d chars]", preview.Text, preview.TotalChars-len([]rune(preview.Text)))
	}
	return preview.Text
}

// buildTurn tells a trace as a turn, or returns nil for a plain request with
// nothing to tell. Spans must carry effective timing and annotations.
func buildTurn(trace *Trace, observations []Observation, options TurnOptions) *Turn {
	turn := &Turn{TraceID: trace.ID, Name: trace.Name, StartTime: trace.StartTime, EndTime: trace.EndTime,
		DurationMs: trace.Duration, Status: trace.Status, Cost: trace.TotalCost, Steps: []TurnStep{}}
	turn.Kind, turn.RootID = turnKind(observations)
	if turn.Kind == "" {
		return nil
	}
	values := metadataValues(trace.Metadata)
	turn.AgentID, _ = firstString(values, "agent_id")
	turn.JobID, _ = firstString(values, "job_id")
	turn.PlanID, _ = firstString(values, "plan_id")

	childCount := make(map[string]int, len(observations))
	for position := range observations {
		childCount[observations[position].ParentObservationID]++
	}
	exchanges := make(map[string]llmExchange)
	results := map[string]toolResult{}
	for position := range observations {
		observation := &observations[position]
		if observation.Role == RoleModelCall && observation.MergedInto == "" {
			exchange := parseLLMExchange(observation.Input, observation.Output)
			exchanges[observation.ID] = exchange
			for id, result := range exchange.ToolResults {
				results[id] = result
			}
		}
	}
	turn.results = results
	mainSystem := dominantSystem(observations, exchanges)
	origin, _ := parseTimestamp(trace.StartTime)
	var mainCalls []string
	for position := range observations {
		observation := &observations[position]
		if isErrorLevel(observation.Level) {
			turn.ErrorCount++
			turn.StatusMessage = firstNonEmpty(turn.StatusMessage, options.clip(observation.StatusMessage))
		}
		if observation.MergedInto != "" || (isWrapper(observation.Role) && childCount[observation.ID] > 0) {
			continue
		}
		step := TurnStep{ObservationID: observation.ID, Role: observation.Role, Label: observation.Label, Name: observation.Name,
			StartTime: observation.StartTime, DurationMs: observation.Duration, Level: nonDefaultLevel(observation.Level),
			StatusMessage: options.clip(observation.StatusMessage), Model: observation.Model, Usage: readUsage(observation.Usage), Cost: observation.Cost}
		if start, ok := parseTimestamp(observation.StartTime); ok && !origin.IsZero() {
			step.OffsetMs = float64(start.Sub(origin)) / 1e6
		}
		step.Pending = observation.EndTime == "" && !strings.EqualFold(observation.Type, "EVENT")
		if observation.Role == RoleModelCall {
			exchange := exchanges[observation.ID]
			step.Side = !exchange.HasMessages || exchange.System != mainSystem
			step.FinishReason = exchange.FinishReason
			step.Text = options.preview(exchange.Text, options.DetailChars)
			step.ToolCalls = pairToolCalls(exchange.ToolCalls, results, options)
			turn.ModelCalls++
			turn.Usage.add(step.Usage)
			if !step.Side {
				mainCalls = append(mainCalls, observation.ID)
			}
		}
		turn.Steps = append(turn.Steps, step)
	}
	if len(mainCalls) > 0 {
		turn.UserMessage = options.preview(exchanges[mainCalls[0]].UserText, options.MessageChars)
		for position := len(mainCalls) - 1; position >= 0; position-- {
			if text := exchanges[mainCalls[position]].Text; text != "" {
				turn.Reply = options.preview(text, options.MessageChars)
				break
			}
		}
	}
	attachToolSpans(turn)
	turn.countToolCalls()
	return turn
}

func (turn *Turn) countToolCalls() {
	turn.ToolCallCount, turn.FailedTools = 0, 0
	for position := range turn.Steps {
		for _, call := range turn.Steps[position].ToolCalls {
			turn.ToolCallCount++
			if call.Error != "" {
				turn.FailedTools++
			}
		}
		if turn.Steps[position].Role == RoleTool {
			turn.ToolCallCount++
		}
	}
}

// pairAcrossTurns fills tool results that arrived in a later turn: a tool that
// ends a turn, such as waiting for a job, is answered in the turn that wakes up.
func pairAcrossTurns(turns []*Turn, options TurnOptions) {
	for position, turn := range turns {
		for stepPosition := range turn.Steps {
			calls := turn.Steps[stepPosition].ToolCalls
			for index := range calls {
				if calls[index].rawResult != "" {
					continue
				}
				for _, later := range turns[position+1:] {
					if result, found := later.results[calls[index].ID]; found {
						calls[index].rawResult = result.Text
						calls[index].Result = options.clip(result.Text)
						calls[index].Error = options.clip(result.Error)
						break
					}
				}
			}
		}
		turn.countToolCalls()
	}
}

// turnKind names the kind of a trace from its spans. A trace holding model
// calls under no recognized wrapper is still a run worth reading.
func turnKind(observations []Observation) (kind, rootID string) {
	priority := map[string]int{RoleTurn: 1, RoleWakeTurn: 2, RoleJob: 3}
	best := 0
	hasModel := false
	for position := range observations {
		observation := &observations[position]
		if rank := priority[observation.Role]; rank > best {
			best, kind, rootID = rank, observation.Role, observation.ID
		}
		hasModel = hasModel || observation.Role == RoleModelCall
	}
	if kind == "" && hasModel {
		return "run", ""
	}
	return kind, rootID
}

// isWrapper reports spans that only frame others, so they are not steps.
func isWrapper(role string) bool {
	switch role {
	case RoleTurn, RoleWakeTurn, RoleJob, RoleRequest, RoleSpan:
		return true
	}
	return false
}

func nonDefaultLevel(level string) string {
	if strings.EqualFold(level, "DEFAULT") {
		return ""
	}
	return level
}

// dominantSystem finds the turn's conversation: the system prompt most of its
// model calls share, the earliest winning a tie.
func dominantSystem(observations []Observation, exchanges map[string]llmExchange) string {
	counts := map[string]int{}
	best, bestCount := "", 0
	for position := range observations {
		exchange, found := exchanges[observations[position].ID]
		if !found || !exchange.HasMessages {
			continue
		}
		counts[exchange.System]++
		if counts[exchange.System] > bestCount {
			best, bestCount = exchange.System, counts[exchange.System]
		}
	}
	return best
}

func pairToolCalls(calls []ToolCall, results map[string]toolResult, options TurnOptions) []ToolCall {
	paired := make([]ToolCall, 0, len(calls))
	for _, call := range calls {
		result := results[call.ID]
		call.rawResult = result.Text
		call.Args = options.clip(call.Args)
		call.Result = options.clip(result.Text)
		call.Error = options.clip(result.Error)
		paired = append(paired, call)
	}
	return paired
}

// attachToolSpans folds a tool span into the model's matching tool call, the
// latest one before it starts, so one tool execution reads as one step.
func attachToolSpans(turn *Turn) {
	sort.SliceStable(turn.Steps, func(left, right int) bool { return turn.Steps[left].OffsetMs < turn.Steps[right].OffsetMs })
	kept := turn.Steps[:0]
	for position := range turn.Steps {
		step := turn.Steps[position]
		if step.Role != RoleTool || step.Level != "" || !attachToolSpan(kept, step) {
			kept = append(kept, step)
		}
	}
	turn.Steps = kept
}

func attachToolSpan(earlier []TurnStep, tool TurnStep) bool {
	for position := len(earlier) - 1; position >= 0; position-- {
		calls := earlier[position].ToolCalls
		for index := range calls {
			if calls[index].Name == tool.Label && calls[index].SpanID == "" {
				calls[index].SpanID = tool.ObservationID
				calls[index].DurationMs = tool.DurationMs
				return true
			}
		}
	}
	return false
}
