package store

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

const summaryChunkSize = 400

// applyEffectiveTiming extends each span's end to cover its descendants. A span
// that closes before its child finishes (a streaming call whose parent exits
// once the stream opens) otherwise reports a sub-millisecond duration for a
// multi-second call. The value sent is kept in ReportedEndTime.
func applyEffectiveTiming(observations []Observation) {
	index := make(map[string]int, len(observations))
	for position := range observations {
		index[observations[position].ID] = position
	}
	for position := range observations {
		childEnd, ok := parseTimestamp(observations[position].EndTime)
		if !ok {
			continue
		}
		visited := map[string]bool{observations[position].ID: true}
		parentID := observations[position].ParentObservationID
		for parentID != "" && !visited[parentID] {
			visited[parentID] = true
			parentPosition, found := index[parentID]
			if !found {
				break
			}
			parent := &observations[parentPosition]
			parentEnd, hasEnd := parseTimestamp(parent.EndTime)
			if hasEnd && parentEnd.Before(childEnd) {
				if parent.ReportedEndTime == "" {
					parent.ReportedEndTime = parent.EndTime
				}
				parent.EndTime = childEnd.UTC().Format(time.RFC3339Nano)
				parent.Duration = durationMillis(parent.StartTime, parent.EndTime)
			}
			parentID = parent.ParentObservationID
		}
	}
}

// summarizeTrace derives the list and header fields of a trace from its spans.
func summarizeTrace(trace *Trace, observations []Observation) {
	applyEffectiveTiming(observations)
	trace.ObservationCount = len(observations)
	trace.Status = traceStatus(observations)

	byID := make(map[string]*Observation, len(observations))
	for position := range observations {
		byID[observations[position].ID] = &observations[position]
	}
	models := make(map[string]bool)
	var tokens, cost float64
	var hasTokens, hasCost bool
	var earliest, latest time.Time
	for position := range observations {
		observation := &observations[position]
		if observation.Model != "" {
			models[observation.Model] = true
		}
		if strings.EqualFold(observation.Type, "GENERATION") {
			trace.hasGeneration = true
		}
		if start, ok := parseTimestamp(observation.StartTime); ok && (earliest.IsZero() || start.Before(earliest)) {
			earliest = start
		}
		if end, ok := parseTimestamp(observation.EndTime); ok && end.After(latest) {
			latest = end
		}
		if value, ok := usageTokens(observation.Usage); ok && !ancestorReports(observation, byID, func(o *Observation) bool {
			_, has := usageTokens(o.Usage)
			return has
		}) {
			tokens += value
			hasTokens = true
		}
		if observation.Cost != nil && !ancestorReports(observation, byID, func(o *Observation) bool { return o.Cost != nil }) {
			cost += *observation.Cost
			hasCost = true
		}
	}
	if hasTokens {
		trace.TotalTokens = &tokens
	}
	if hasCost {
		trace.TotalCost = &cost
	}
	trace.Models = sortedKeys(models)
	if !earliest.IsZero() && !latest.IsZero() && !latest.Before(earliest) {
		trace.StartTime = earliest.UTC().Format(time.RFC3339Nano)
		trace.EndTime = latest.UTC().Format(time.RFC3339Nano)
		trace.Duration = durationMillis(trace.StartTime, trace.EndTime)
		trace.Latency = trace.Duration
	}
}

// ancestorReports reports whether an ancestor already carries the measure. One
// LLM call is commonly recorded twice (the application's generation and the
// SDK's child span), and summing both would double every total.
func ancestorReports(observation *Observation, byID map[string]*Observation, has func(*Observation) bool) bool {
	visited := map[string]bool{observation.ID: true}
	for parentID := observation.ParentObservationID; parentID != "" && !visited[parentID]; {
		visited[parentID] = true
		parent, found := byID[parentID]
		if !found {
			return false
		}
		if has(parent) {
			return true
		}
		parentID = parent.ParentObservationID
	}
	return false
}

// traceStatus reports only what the spans establish: "error" when one failed,
// "pending" while one is open, "success" on an explicit OK or a 2xx/3xx HTTP
// response, "completed" when every span closed without reporting an outcome,
// and "" (unknown) without spans.
func traceStatus(observations []Observation) string {
	if len(observations) == 0 {
		return ""
	}
	pending, succeeded := false, false
	for position := range observations {
		observation := &observations[position]
		if strings.EqualFold(observation.Level, "error") || strings.EqualFold(observation.Level, "fatal") {
			return "error"
		}
		if observation.EndTime == "" && !strings.EqualFold(observation.Type, "EVENT") {
			pending = true
		}
		if strings.EqualFold(observation.Level, "ok") || successfulHTTP(observation) {
			succeeded = true
		}
	}
	switch {
	case pending:
		return "pending"
	case succeeded:
		return "success"
	default:
		return "completed"
	}
}

func successfulHTTP(observation *Observation) bool {
	values := metadataValues(observation.Metadata)
	status, found := numberIn(values, "status_code", "http.response.status_code")
	return found && status >= 200 && status < 400
}

// attachSummaries fills the summary fields of each listed trace.
func (s *Store) attachSummaries(ctx context.Context, traces []Trace) error {
	byTrace := make(map[string][]Observation, len(traces))
	for start := 0; start < len(traces); start += summaryChunkSize {
		end := start + summaryChunkSize
		if end > len(traces) {
			end = len(traces)
		}
		args := make([]any, 0, end-start)
		for _, trace := range traces[start:end] {
			args = append(args, trace.ID)
		}
		rows, err := s.db.QueryContext(ctx, `
			SELECT id, trace_id, parent_observation_id, type, model, usage, metadata, level, start_time, end_time
			FROM observations WHERE trace_id IN (`+placeholders(len(args))+`)`, args...)
		if err != nil {
			return fmt.Errorf("summarize traces: %w", err)
		}
		for rows.Next() {
			var observation Observation
			var parent, typ, model, usage, metadata, level, startTime, endTime nullableString
			if err := rows.Scan(&observation.ID, &observation.TraceID, &parent, &typ, &model, &usage, &metadata, &level, &startTime, &endTime); err != nil {
				_ = rows.Close()
				return fmt.Errorf("scan trace summary: %w", err)
			}
			observation.ParentObservationID = parent.String
			observation.Type = typ.String
			observation.Model = model.String
			observation.Usage = NormalizeUsage(rawJSON(usage))
			observation.Metadata = rawJSON(metadata)
			observation.Cost = metadataCost(observation.Metadata)
			observation.Level = level.String
			observation.StartTime = startTime.String
			observation.EndTime = endTime.String
			byTrace[observation.TraceID] = append(byTrace[observation.TraceID], observation)
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return fmt.Errorf("iterate trace summaries: %w", err)
		}
	}
	for position := range traces {
		summarizeTrace(&traces[position], byTrace[traces[position].ID])
	}
	return nil
}

func placeholders(count int) string {
	return strings.TrimSuffix(strings.Repeat("?,", count), ",")
}

func sortedKeys(values map[string]bool) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
