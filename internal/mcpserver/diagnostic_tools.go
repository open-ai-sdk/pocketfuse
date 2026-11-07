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
	Offset        int    `json:"offset,omitempty" jsonschema:"Redacted characters to skip in field. Use nextOffset. Needs field; cannot combine with from_end"`
	FromEnd       bool   `json:"from_end,omitempty" jsonschema:"Read a byte-bounded suffix of field. Offset reports its start; read earlier text with offset=0 without from_end. Needs field"`
	MaxChars      int    `json:"max_chars,omitempty" jsonschema:"Maximum redacted characters per field; the JSON byte budget may shorten this. Default 4000, at most 20000 with field and 8000 without"`
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

func fieldText(raw json.RawMessage, filter *redactor) []rune {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return []rune(filter.Text(string(raw)))
}

func sliceField(runes []rune, offset, maxChars int, fromEnd bool) *fieldSlice {
	if runes == nil {
		return nil
	}
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
		if fromEnd && start > 0 {
			slice.Truncated += " Read earlier text with field and offset=0, without from_end."
		}
	}
	return slice
}

// fitFields always measures the full escaped JSON and redaction report. Redact
// sources once before calling it so character offsets never depend on a cut.
func fitFields(maxChars int, filter *redactor, build func(int) any) (any, error) {
	value := build(maxChars)
	if filter.fits(value) {
		return value, nil
	}
	low, high := 0, maxChars-1
	for low < high {
		middle := (low + high + 1) / 2
		if filter.fits(build(middle)) {
			low = middle
		} else {
			high = middle - 1
		}
	}
	if low == 0 {
		return nil, errors.New("detail header exceeds the response byte limit; field payload cannot fit")
	}
	return build(low), nil
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
		if input.FromEnd && input.Offset != 0 {
			return nil, errors.New("offset and from_end cannot be combined")
		}
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
	type namedField struct {
		name string
		raw  json.RawMessage
	}
	fields := []namedField{{"input", observation.Input}, {"output", observation.Output}, {"metadata", observation.Metadata}, {"model_parameters", observation.ModelParameters}}
	texts := make(map[string][]rune, len(fields))
	for _, field := range fields {
		if input.Field == "" || input.Field == field.name {
			texts[field.name] = fieldText(field.raw, filter)
		}
	}
	return fitFields(maxChars, filter, func(chars int) any {
		slice := func(name string) *fieldSlice {
			return sliceField(texts[name], max(input.Offset, 0), chars, input.FromEnd)
		}
		body := struct {
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
		}{
			ID: observation.ID, TraceID: observation.TraceID, ParentID: observation.ParentObservationID, Role: observation.Role, Label: observation.Label,
			Name: observation.Name, Type: observation.Type, Level: observation.Level, StatusMessage: observation.StatusMessage, Model: observation.Model,
			StartTime: observation.StartTime, EndTime: observation.EndTime, ReportedEndTime: observation.ReportedEndTime, DurationMs: observation.Duration,
			MergedInto: observation.MergedInto, MergedSpans: observation.MergedSpans, Usage: observation.Usage, Cost: observation.Cost, Summary: step,
		}
		return newFieldResponse(body,
			fieldPayload{"input", slice("input")},
			fieldPayload{"output", slice("output")},
			fieldPayload{"metadata", slice("metadata")},
			fieldPayload{"modelParameters", slice("model_parameters")},
		)
	})
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

func (t *tools) recentErrors(ctx context.Context, input recentErrorsInput, filter *redactor) (any, error) {
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
		result.Data[position].StatusMessage = clipRunes(filter.Text(result.Data[position].StatusMessage), 500)
	}
	return fitResponseItems(result.Data, filter, func(rows []store.SpanError, cut int) any {
		return struct {
			ServerTime string `json:"serverTime"`
			From       string `json:"from"`
			page
			Errors []store.SpanError `json:"errors"`
		}{now.UTC().Format(time.RFC3339), from, newPage(result.Total, offset, len(rows), cut), rows}
	})
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

func (t *tools) slowCalls(ctx context.Context, input slowCallsInput, filter *redactor) (any, error) {
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
	build := func(groups, spans int) any {
		note := truncated
		groupsOmitted, spansOmitted := len(summary.Groups)-groups, len(summary.Slowest)-spans
		if groupsOmitted > 0 || spansOmitted > 0 {
			note += " Response byte limit reached; narrow name, kind, session_id, from or to for a smaller summary."
		}
		return struct {
			ServerTime     string `json:"serverTime"`
			From           string `json:"from"`
			Truncated      string `json:"truncated,omitempty"`
			GroupsOmitted  int    `json:"groupsOmitted,omitempty"`
			SlowestOmitted int    `json:"slowestOmitted,omitempty"`
			store.SlowSummary
		}{now.UTC().Format(time.RFC3339), from, note, groupsOmitted, spansOmitted,
			store.SlowSummary{ScannedSpans: summary.ScannedSpans, ScanCapped: summary.ScanCapped, Groups: summary.Groups[:groups], Slowest: summary.Slowest[:spans]}}
	}
	groups, spans := len(summary.Groups), len(summary.Slowest)
	for {
		value := build(groups, spans)
		if filter.fits(value) {
			return value, nil
		}
		switch {
		case groups > 1 && groups >= spans:
			groups = (groups + 1) / 2
		case spans > 1:
			spans = (spans + 1) / 2
		default:
			return nil, errors.New("one slow-call summary exceeds the byte limit; narrow name, kind, session_id, from or to")
		}
	}
}
