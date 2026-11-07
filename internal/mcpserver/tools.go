package mcpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/open-ai-sdk/pocketfuse/internal/store"
)

const threadSessionPrefix = "thread:"

type tools struct{ store *store.Store }

func registerTools(server *mcp.Server, t *tools) {
	addTool(server, "list_traces", "List traces",
		"Search traces, newest first. Filter by name or id prefix, session or thread, user, status, time. Routine polling is hidden unless include_noise is true. Paginated.", t.listTraces)
	addTool(server, "list_sessions", "List sessions",
		"List conversations, most recently active first, with each one's opening message and failed span count. Paginated.", t.listSessions)
	addTool(server, "get_session_timeline", "Get session timeline",
		"Read one conversation in order: each turn, wake turn and background job with its opening message, reply, model calls, tool calls with arguments and results, latency, tokens and errors. Text is clipped; paginated by item.", t.getSessionTimeline)
	addTool(server, "get_trace", "Get trace as a tree",
		"Read one trace as a compact span tree with timing, tokens, levels and tool calls. Nested spans recording one provider call show as one model call unless raw_spans is true. Paginated by node.", t.getTrace)
	addTool(server, "get_observation", "Get one observation",
		"Read one span's input, output and metadata as text. Each field is returned in chunks: pass field and offset to continue, or from_end to read the end of a long prompt, where the latest messages are.", t.getObservation)
	addTool(server, "recent_errors", "Recent errors",
		"List failed spans, newest first, from the last 24 hours by default. A tool error the model recovered from is not a failed span: see failedTools in get_session_timeline. Paginated.", t.recentErrors)
	addTool(server, "slow_calls", "Slow call summary",
		"Summarize latency by kind and name (count, p50, p95, max, total), slowest p95 first, and list the slowest individual spans, from the last 24 hours by default. Long-lived sockets count as requests: use kind to narrow.", t.slowCalls)
}

// page is the pagination every list response carries.
type page struct {
	Total      int    `json:"total"`
	Offset     int    `json:"offset"`
	Returned   int    `json:"returned"`
	NextOffset *int   `json:"nextOffset,omitempty"`
	Truncated  string `json:"truncated,omitempty"`
}

func newPage(total, offset, returned, cutBySize int) page {
	result := page{Total: total, Offset: offset, Returned: returned}
	next := offset + returned
	if next < total {
		result.NextOffset = &next
		result.Truncated = fmt.Sprintf("Showing %d of %d from offset %d. Pass offset=%d for more.", returned, total, offset, next)
		if cutBySize > 0 {
			result.Truncated = fmt.Sprintf("Response size limit reached: showing %d of %d from offset %d. Pass offset=%d for more.", returned, total, offset, next)
		}
	}
	return result
}

// fitItems keeps the leading items that fit the response budget, and reports how
// many were cut for size. One item is kept regardless: render enforces the limit.
func fitItems[T any](items []T, budget int) ([]T, int) {
	used := 0
	for position := range items {
		encoded, err := json.Marshal(items[position])
		if err != nil {
			return items[:position], len(items) - position
		}
		used += len(encoded) + 1
		if used > budget && position > 0 {
			return items[:position], len(items) - position
		}
	}
	return items, 0
}

func notFound(kind, id string, err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%s %q not found", kind, id)
	}
	return err
}

type listTracesInput struct {
	Name         string `json:"name,omitempty" jsonschema:"Case-insensitive part of the trace name, or a trace id prefix"`
	SessionID    string `json:"session_id,omitempty" jsonschema:"Exact session id, as returned by list_sessions"`
	ThreadID     string `json:"thread_id,omitempty" jsonschema:"Thread id, the same as session_id thread:<id>"`
	UserID       string `json:"user_id,omitempty" jsonschema:"Exact user id"`
	Project      string `json:"project,omitempty" jsonschema:"Exact project id"`
	Status       string `json:"status,omitempty" jsonschema:"error, pending (a span is still open) or finished (all closed, none failed)"`
	ErrorsOnly   bool   `json:"errors_only,omitempty" jsonschema:"Only traces with a failed span. Same as status=error"`
	From         string `json:"from,omitempty" jsonschema:"Start of the window: RFC 3339, or an age such as 90m, 24h, 7d"`
	To           string `json:"to,omitempty" jsonschema:"End of the window, in the same forms"`
	IncludeNoise bool   `json:"include_noise,omitempty" jsonschema:"Include routine polling and maintenance traces"`
	Offset       int    `json:"offset,omitempty" jsonschema:"Rows to skip. Use nextOffset from the previous response"`
	Limit        int    `json:"limit,omitempty" jsonschema:"Rows to return. Default 20, at most 50"`
}

type traceRow struct {
	ID         string   `json:"id"`
	Name       string   `json:"name,omitempty"`
	SessionID  string   `json:"sessionId,omitempty"`
	UserID     string   `json:"userId,omitempty"`
	Status     string   `json:"status,omitempty"`
	StartTime  string   `json:"startTime,omitempty"`
	DurationMs float64  `json:"durationMs"`
	Spans      int      `json:"spans"`
	Tokens     *float64 `json:"tokens,omitempty"`
	Cost       *float64 `json:"cost,omitempty"`
	Models     []string `json:"models,omitempty"`
}

func (t *tools) listTraces(ctx context.Context, input listTracesInput, _ *redactor) (any, error) {
	now := time.Now()
	from, to, err := timeRange(input.From, input.To, "", now)
	if err != nil {
		return nil, err
	}
	status := input.Status
	if input.ErrorsOnly {
		status = store.StatusError
	}
	switch status {
	case "", store.StatusError, store.StatusPending, store.StatusFinished:
	default:
		return nil, fmt.Errorf("status %q is not one of error, pending, finished", status)
	}
	sessionID, err := resolveSession(input.SessionID, input.ThreadID, false)
	if err != nil {
		return nil, err
	}
	offset := max(input.Offset, 0)
	result, err := t.store.ListTraces(ctx, store.TraceFilter{
		Name: input.Name, SessionID: sessionID, UserID: input.UserID, Project: input.Project, Status: status,
		From: from, To: to, HideNoise: !input.IncludeNoise, Offset: offset, Limit: bound(input.Limit, 20, 50),
	})
	if err != nil {
		return nil, err
	}
	rows := make([]traceRow, 0, len(result.Data))
	for position := range result.Data {
		trace := &result.Data[position]
		rows = append(rows, traceRow{ID: trace.ID, Name: trace.Name, SessionID: trace.SessionID, UserID: trace.UserID, Status: trace.Status,
			StartTime: trace.StartTime, DurationMs: trace.Duration, Spans: trace.ObservationCount, Tokens: trace.TotalTokens, Cost: trace.TotalCost, Models: trace.Models})
	}
	rows, cut := fitItems(rows, responseBudgetBytes)
	return struct {
		ServerTime string `json:"serverTime"`
		page
		Traces []traceRow `json:"traces"`
	}{now.UTC().Format(time.RFC3339), newPage(result.Total, offset, len(rows), cut), rows}, nil
}

// resolveSession turns the two ways of naming a session into its id.
func resolveSession(sessionID, threadID string, required bool) (string, error) {
	sessionID, threadID = strings.TrimSpace(sessionID), strings.TrimSpace(threadID)
	switch {
	case sessionID != "" && threadID != "":
		return "", errors.New("pass session_id or thread_id, not both")
	case threadID != "":
		return threadSessionPrefix + strings.TrimPrefix(threadID, threadSessionPrefix), nil
	case sessionID == "" && required:
		return "", errors.New("session_id or thread_id is required")
	}
	return sessionID, nil
}

type listSessionsInput struct {
	UserID string `json:"user_id,omitempty" jsonschema:"Exact user id"`
	From   string `json:"from,omitempty" jsonschema:"Last active after this: RFC 3339, or an age such as 90m, 24h, 7d"`
	To     string `json:"to,omitempty" jsonschema:"Last active before this, in the same forms"`
	Offset int    `json:"offset,omitempty" jsonschema:"Rows to skip. Use nextOffset from the previous response"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Rows to return. Default 20, at most 50"`
}

func (t *tools) listSessions(ctx context.Context, input listSessionsInput, _ *redactor) (any, error) {
	from, to, err := timeRange(input.From, input.To, "", time.Now())
	if err != nil {
		return nil, err
	}
	offset := max(input.Offset, 0)
	limit := bound(input.Limit, 20, 50)
	result, err := t.store.ListSessions(ctx, store.SessionFilter{UserID: input.UserID, From: from, To: to, Offset: offset, Limit: limit})
	if err != nil {
		return nil, err
	}
	rows, cut := fitItems(result.Data, responseBudgetBytes)
	return struct {
		page
		Sessions []store.Session `json:"sessions"`
	}{newPage(result.Total, offset, len(rows), cut), rows}, nil
}

type sessionTimelineInput struct {
	SessionID    string `json:"session_id,omitempty" jsonschema:"Session id, as returned by list_sessions"`
	ThreadID     string `json:"thread_id,omitempty" jsonschema:"Thread id, the same as session_id thread:<id>"`
	IncludeNoise bool   `json:"include_noise,omitempty" jsonschema:"Include routine polling requests"`
	Offset       int    `json:"offset,omitempty" jsonschema:"Items to skip. Use nextOffset from the previous response"`
	Limit        int    `json:"limit,omitempty" jsonschema:"Items to return. Default 10, at most 30"`
}

// timelineItem is a story item clipped for a model's context.
type timelineItem struct {
	Type            string               `json:"type"`
	Turn            *store.Turn          `json:"turn,omitempty"`
	StepsOmitted    int                  `json:"stepsOmitted,omitempty"`
	Activity        *store.StoryActivity `json:"activity,omitempty"`
	RequestsOmitted int                  `json:"requestsOmitted,omitempty"`
}

const (
	maxTimelineRequests = 5
	maxTurnSteps        = 25
)

func clipTimelineItem(item store.StoryItem) timelineItem {
	clipped := timelineItem{Type: item.Type, Turn: item.Turn, Activity: item.Activity}
	if item.Activity != nil && len(item.Activity.Requests) > maxTimelineRequests {
		clipped.RequestsOmitted = len(item.Activity.Requests) - maxTimelineRequests
		item.Activity.Requests = item.Activity.Requests[:maxTimelineRequests]
	}
	if item.Turn != nil && len(item.Turn.Steps) > maxTurnSteps {
		clipped.StepsOmitted = len(item.Turn.Steps) - maxTurnSteps
		item.Turn.Steps = item.Turn.Steps[:maxTurnSteps]
	}
	return clipped
}

func (t *tools) getSessionTimeline(ctx context.Context, input sessionTimelineInput, filter *redactor) (any, error) {
	sessionID, err := resolveSession(input.SessionID, input.ThreadID, true)
	if err != nil {
		return nil, err
	}
	story, err := t.store.GetSessionStory(ctx, sessionID, !input.IncludeNoise, store.TurnOptions{MessageChars: 500, DetailChars: 200, Filter: filter.Text})
	if err != nil {
		return nil, notFound("session", sessionID, err)
	}
	offset := max(input.Offset, 0)
	limit := bound(input.Limit, 10, 30)
	window := story.Items[min(offset, len(story.Items)):min(offset+limit, len(story.Items))]
	items := make([]timelineItem, 0, len(window))
	for _, item := range window {
		items = append(items, clipTimelineItem(item))
	}
	items, cut := fitItems(items, responseBudgetBytes)
	return struct {
		ID            string            `json:"id"`
		Title         string            `json:"title,omitempty"`
		UserID        string            `json:"userId,omitempty"`
		Totals        store.StoryTotals `json:"totals"`
		HiddenNoise   int               `json:"hiddenNoise,omitempty"`
		OmittedTraces int               `json:"omittedTraces,omitempty"`
		page
		Items []timelineItem `json:"items"`
	}{story.ID, story.Title, story.UserID, story.Totals, story.HiddenNoise, story.Omitted, newPage(len(story.Items), offset, len(items), cut), items}, nil
}

type getTraceInput struct {
	TraceID  string `json:"trace_id" jsonschema:"Trace id"`
	RawSpans bool   `json:"raw_spans,omitempty" jsonschema:"Show every span, without folding nested spans of one provider call"`
	Offset   int    `json:"offset,omitempty" jsonschema:"Nodes to skip. Use nextOffset from the previous response"`
	Limit    int    `json:"limit,omitempty" jsonschema:"Nodes to return. Default 80, at most 200"`
}

type treeNode struct {
	ID            string            `json:"id"`
	Depth         int               `json:"depth"`
	Role          string            `json:"role"`
	Label         string            `json:"label"`
	Name          string            `json:"name,omitempty"`
	OffsetMs      float64           `json:"offsetMs"`
	DurationMs    float64           `json:"durationMs"`
	Pending       bool              `json:"pending,omitempty"`
	Level         string            `json:"level,omitempty"`
	StatusMessage string            `json:"statusMessage,omitempty"`
	Side          bool              `json:"side,omitempty"`
	Usage         *store.TokenUsage `json:"usage,omitempty"`
	FinishReason  string            `json:"finishReason,omitempty"`
	Tools         []string          `json:"tools,omitempty"`
	FailedTools   []string          `json:"failedTools,omitempty"`
	MergedSpans   []string          `json:"mergedSpans,omitempty"`
}

func (t *tools) getTrace(ctx context.Context, input getTraceInput, filter *redactor) (any, error) {
	if strings.TrimSpace(input.TraceID) == "" {
		return nil, errors.New("trace_id is required")
	}
	trace, err := t.store.GetTraceWith(ctx, input.TraceID, store.TurnOptions{MessageChars: 500, DetailChars: 160, Filter: filter.Text})
	if err != nil {
		return nil, notFound("trace", input.TraceID, err)
	}
	offset := max(input.Offset, 0)
	limit := bound(input.Limit, 80, 200)
	nodes := traceTree(trace, input.RawSpans)
	total := len(nodes)
	nodes = nodes[min(offset, total):min(offset+limit, total)]
	nodes, cut := fitItems(nodes, responseBudgetBytes-4000)
	scores, err := t.store.ListScores(ctx, store.ScoreFilter{TraceID: trace.ID, Limit: 20})
	if err != nil {
		return nil, err
	}
	type header struct {
		ID          string          `json:"id"`
		Name        string          `json:"name,omitempty"`
		SessionID   string          `json:"sessionId,omitempty"`
		UserID      string          `json:"userId,omitempty"`
		Status      string          `json:"status,omitempty"`
		StartTime   string          `json:"startTime,omitempty"`
		DurationMs  float64         `json:"durationMs"`
		Tokens      *float64        `json:"tokens,omitempty"`
		Cost        *float64        `json:"cost,omitempty"`
		Metadata    json.RawMessage `json:"metadata,omitempty"`
		UserMessage *store.Preview  `json:"userMessage,omitempty"`
		Reply       *store.Preview  `json:"reply,omitempty"`
		Scores      []store.Score   `json:"scores,omitempty"`
		ScoreCount  int             `json:"scoreCount,omitempty"`
	}
	head := header{ID: trace.ID, Name: trace.Name, SessionID: trace.SessionID, UserID: trace.UserID, Status: trace.Status, StartTime: trace.StartTime,
		DurationMs: trace.Duration, Tokens: trace.TotalTokens, Cost: trace.TotalCost, Metadata: trace.Metadata, Scores: scores.Data, ScoreCount: scores.Total}
	if trace.Turn != nil {
		head.UserMessage, head.Reply = trace.Turn.UserMessage, trace.Turn.Reply
	}
	return struct {
		Trace header `json:"trace"`
		page
		Nodes []treeNode `json:"nodes"`
	}{head, newPage(total, offset, len(nodes), cut), nodes}, nil
}

// traceTree lists a trace's spans depth-first in start order. A folded span's
// children attach to the span it folded into.
func traceTree(trace *store.Trace, raw bool) []treeNode {
	byID := make(map[string]*store.Observation, len(trace.Observations))
	for position := range trace.Observations {
		byID[trace.Observations[position].ID] = &trace.Observations[position]
	}
	visibleParent := func(id string) string {
		for seen := 0; seen < len(trace.Observations); seen++ {
			parent, found := byID[id]
			if !found {
				return ""
			}
			if raw || parent.MergedInto == "" {
				return id
			}
			id = parent.MergedInto
		}
		return ""
	}
	children := map[string][]*store.Observation{}
	for position := range trace.Observations {
		observation := &trace.Observations[position]
		if !raw && observation.MergedInto != "" {
			continue
		}
		parent := visibleParent(observation.ParentObservationID)
		children[parent] = append(children[parent], observation)
	}
	origin, _ := time.Parse(time.RFC3339Nano, trace.StartTime)
	steps := map[string]*store.TurnStep{}
	if trace.Turn != nil {
		for position := range trace.Turn.Steps {
			steps[trace.Turn.Steps[position].ObservationID] = &trace.Turn.Steps[position]
		}
	}
	nodes := make([]treeNode, 0, len(trace.Observations))
	visited := map[string]bool{}
	var walk func(parent string, depth int)
	walk = func(parent string, depth int) {
		for _, observation := range children[parent] {
			if visited[observation.ID] {
				continue
			}
			visited[observation.ID] = true
			nodes = append(nodes, newTreeNode(observation, depth, origin, steps[observation.ID]))
			walk(observation.ID, depth+1)
		}
	}
	walk("", 0)
	return nodes
}

func newTreeNode(observation *store.Observation, depth int, origin time.Time, step *store.TurnStep) treeNode {
	node := treeNode{ID: observation.ID, Depth: depth, Role: observation.Role, Label: observation.Label, DurationMs: observation.Duration,
		Pending: observation.EndTime == "" && !strings.EqualFold(observation.Type, "EVENT"), StatusMessage: observation.StatusMessage, MergedSpans: observation.MergedSpans}
	if observation.Name != observation.Label {
		node.Name = observation.Name
	}
	if !strings.EqualFold(observation.Level, "DEFAULT") {
		node.Level = observation.Level
	}
	if start, err := time.Parse(time.RFC3339Nano, observation.StartTime); err == nil && !origin.IsZero() {
		node.OffsetMs = float64(start.Sub(origin)) / 1e6
	}
	if step == nil {
		return node
	}
	node.Side, node.FinishReason = step.Side, step.FinishReason
	if step.Role == store.RoleModelCall {
		usage := step.Usage
		node.Usage = &usage
	}
	for _, call := range step.ToolCalls {
		node.Tools = append(node.Tools, call.Name)
		if call.Error != "" {
			node.FailedTools = append(node.FailedTools, call.Name)
		}
	}
	return node
}
