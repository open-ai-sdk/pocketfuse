package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/open-ai-sdk/pocketfuse/internal/store"
)

const (
	defaultFieldChars  = 4000
	maxFieldChars      = 20000
	maxAllFieldsChars  = 8000
	defaultErrorWindow = "24h"
)

type getObservationInput struct {
	ObservationID string `json:"observation_id" jsonschema:"Observation (span) id"`
	Field         string `json:"field,omitempty" jsonschema:"input, output, metadata or model_parameters. Default all of them, each from its start"`
	Offset        int    `json:"offset,omitempty" jsonschema:"Characters to skip in field. Use nextOffset from the previous response. Needs field"`
	FromEnd       bool   `json:"from_end,omitempty" jsonschema:"Read the last max_chars of field instead of the first. Needs field"`
	MaxChars      int    `json:"max_chars,omitempty" jsonschema:"Characters per field. Default 4000, at most 20000 with field and 8000 without"`
}

// fieldSlice is a window onto one field. TotalChars counts the field after
// redaction, so offsets stay stable across calls.
type fieldSlice struct {
	Text       string `json:"text"`
	TotalChars int    `json:"totalChars"`
	Offset     int    `json:"offset"`
	NextOffset *int   `json:"nextOffset,omitempty"`
	Truncated  string `json:"truncated,omitempty"`
}

func sliceField(raw json.RawMessage, offset, maxChars int, fromEnd bool, filter *redactor) *fieldSlice {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	runes := []rune(filter.Text(string(raw)))
	total := len(runes)
	if fromEnd {
		offset = max(total-maxChars, 0)
	}
	start := min(offset, total)
	end := min(start+maxChars, total)
	slice := &fieldSlice{Text: string(runes[start:end]), TotalChars: total, Offset: start}
	if end < total {
		slice.NextOffset = &end
	}
	if start > 0 || end < total {
		slice.Truncated = fmt.Sprintf("Characters %d to %d of %d.", start, end, total)
	}
	return slice
}

func (t *tools) getObservation(ctx context.Context, input getObservationInput, filter *redactor) (any, error) {
	if strings.TrimSpace(input.ObservationID) == "" {
		return nil, errors.New("observation_id is required")
	}
	switch input.Field {
	case "":
		if input.Offset > 0 || input.FromEnd {
			return nil, errors.New("offset and from_end need field")
		}
	case "input", "output", "metadata", "model_parameters":
	default:
		return nil, fmt.Errorf("field %q is not one of input, output, metadata, model_parameters", input.Field)
	}
	ceiling := maxFieldChars
	if input.Field == "" {
		ceiling = maxAllFieldsChars
	}
	maxChars := bound(input.MaxChars, defaultFieldChars, ceiling)
	observation, step, err := t.store.GetObservation(ctx, input.ObservationID, store.TurnOptions{MessageChars: 400, DetailChars: 200, Filter: filter.Text})
	if err != nil {
		return nil, notFound("observation", input.ObservationID, err)
	}
	slice := func(name string, raw json.RawMessage) *fieldSlice {
		if input.Field != "" && input.Field != name {
			return nil
		}
		return sliceField(raw, max(input.Offset, 0), maxChars, input.FromEnd, filter)
	}
	result := struct {
		ID              string          `json:"id"`
		TraceID         string          `json:"traceId"`
		ParentID        string          `json:"parentId,omitempty"`
		Role            string          `json:"role"`
		Label           string          `json:"label"`
		Name            string          `json:"name,omitempty"`
		Type            string          `json:"type,omitempty"`
		Level           string          `json:"level,omitempty"`
		StatusMessage   string          `json:"statusMessage,omitempty"`
		Model           string          `json:"model,omitempty"`
		StartTime       string          `json:"startTime,omitempty"`
		EndTime         string          `json:"endTime,omitempty"`
		ReportedEndTime string          `json:"reportedEndTime,omitempty"`
		DurationMs      float64         `json:"durationMs"`
		MergedInto      string          `json:"mergedInto,omitempty"`
		MergedSpans     []string        `json:"mergedSpans,omitempty"`
		Usage           json.RawMessage `json:"usage,omitempty"`
		Cost            *float64        `json:"cost,omitempty"`
		Summary         *store.TurnStep `json:"summary,omitempty"`
		Input           *fieldSlice     `json:"input,omitempty"`
		Output          *fieldSlice     `json:"output,omitempty"`
		Metadata        *fieldSlice     `json:"metadata,omitempty"`
		ModelParameters *fieldSlice     `json:"modelParameters,omitempty"`
	}{
		ID: observation.ID, TraceID: observation.TraceID, ParentID: observation.ParentObservationID, Role: observation.Role, Label: observation.Label,
		Name: observation.Name, Type: observation.Type, Level: observation.Level, StatusMessage: observation.StatusMessage, Model: observation.Model,
		StartTime: observation.StartTime, EndTime: observation.EndTime, ReportedEndTime: observation.ReportedEndTime, DurationMs: observation.Duration,
		MergedInto: observation.MergedInto, MergedSpans: observation.MergedSpans, Usage: observation.Usage, Cost: observation.Cost, Summary: step,
		Input: slice("input", observation.Input), Output: slice("output", observation.Output),
		Metadata: slice("metadata", observation.Metadata), ModelParameters: slice("model_parameters", observation.ModelParameters),
	}
	return result, nil
}

type recentErrorsInput struct {
	Name      string `json:"name,omitempty" jsonschema:"Case-insensitive part of the span name"`
	SessionID string `json:"session_id,omitempty" jsonschema:"Exact session id"`
	ThreadID  string `json:"thread_id,omitempty" jsonschema:"Thread id, the same as session_id thread:<id>"`
	From      string `json:"from,omitempty" jsonschema:"Start of the window: RFC 3339, or an age such as 90m, 24h, 7d. Default 24h"`
	To        string `json:"to,omitempty" jsonschema:"End of the window, in the same forms"`
	Offset    int    `json:"offset,omitempty" jsonschema:"Rows to skip. Use nextOffset from the previous response"`
	Limit     int    `json:"limit,omitempty" jsonschema:"Rows to return. Default 20, at most 50"`
}

func (t *tools) recentErrors(ctx context.Context, input recentErrorsInput, _ *redactor) (any, error) {
	now := time.Now()
	from, to, err := timeRange(input.From, input.To, defaultErrorWindow, now)
	if err != nil {
		return nil, err
	}
	sessionID, err := resolveSession(input.SessionID, input.ThreadID, false)
	if err != nil {
		return nil, err
	}
	offset := max(input.Offset, 0)
	result, err := t.store.ListSpanErrors(ctx, store.SpanFilter{Name: input.Name, SessionID: sessionID, From: from, To: to, Offset: offset, Limit: bound(input.Limit, 20, 50)})
	if err != nil {
		return nil, err
	}
	for position := range result.Data {
		result.Data[position].StatusMessage = clipRunes(result.Data[position].StatusMessage, 500)
	}
	rows, cut := fitItems(result.Data, responseBudgetBytes)
	return struct {
		ServerTime string `json:"serverTime"`
		From       string `json:"from"`
		page
		Errors []store.SpanError `json:"errors"`
	}{now.UTC().Format(time.RFC3339), from, newPage(result.Total, offset, len(rows), cut), rows}, nil
}

func clipRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "..."
}

type slowCallsInput struct {
	Kind      string `json:"kind,omitempty" jsonschema:"model_call, tool, turn, wake_turn, job, request or span. Default all"`
	Name      string `json:"name,omitempty" jsonschema:"Case-insensitive part of the span name"`
	SessionID string `json:"session_id,omitempty" jsonschema:"Exact session id"`
	ThreadID  string `json:"thread_id,omitempty" jsonschema:"Thread id, the same as session_id thread:<id>"`
	From      string `json:"from,omitempty" jsonschema:"Start of the window: RFC 3339, or an age such as 90m, 24h, 7d. Default 24h"`
	To        string `json:"to,omitempty" jsonschema:"End of the window, in the same forms"`
	TopGroups int    `json:"top_groups,omitempty" jsonschema:"Groups to return. Default 10, at most 30"`
	TopSpans  int    `json:"top_spans,omitempty" jsonschema:"Individual slow spans to return. Default 10, at most 30"`
}

func (t *tools) slowCalls(ctx context.Context, input slowCallsInput, _ *redactor) (any, error) {
	switch input.Kind {
	case "", store.RoleModelCall, store.RoleTool, store.RoleTurn, store.RoleWakeTurn, store.RoleJob, store.RoleRequest, store.RoleSpan:
	default:
		return nil, fmt.Errorf("kind %q is not one of model_call, tool, turn, wake_turn, job, request, span", input.Kind)
	}
	now := time.Now()
	from, to, err := timeRange(input.From, input.To, defaultErrorWindow, now)
	if err != nil {
		return nil, err
	}
	sessionID, err := resolveSession(input.SessionID, input.ThreadID, false)
	if err != nil {
		return nil, err
	}
	topGroups, topSpans := bound(input.TopGroups, 10, 30), bound(input.TopSpans, 10, 30)
	summary, err := t.store.SummarizeSlowSpans(ctx, store.SpanFilter{Name: input.Name, SessionID: sessionID, From: from, To: to}, input.Kind, topGroups, topSpans)
	if err != nil {
		return nil, err
	}
	truncated := ""
	if summary.ScanCapped {
		truncated = fmt.Sprintf("Only the newest %d spans in the window were measured. Narrow the window for a complete summary.", summary.ScannedSpans)
	}
	return struct {
		ServerTime string `json:"serverTime"`
		From       string `json:"from"`
		Truncated  string `json:"truncated,omitempty"`
		store.SlowSummary
	}{now.UTC().Format(time.RFC3339), from, truncated, summary}, nil
}
