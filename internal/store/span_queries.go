package store

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

const errorLevelCondition = `level IN ('ERROR', 'FATAL', 'error', 'fatal')`

const maxTimingRows = 20000

// Trace status filters. Finished covers the "success" and "completed" statuses:
// every span closed and none failed.
const (
	StatusError    = "error"
	StatusPending  = "pending"
	StatusFinished = "finished"
)

func statusCondition(status string) string {
	failed := `id IN (SELECT trace_id FROM observations WHERE ` + errorLevelCondition + `)`
	open := `id IN (SELECT trace_id FROM observations WHERE end_time = '' AND type <> 'EVENT')`
	switch status {
	case StatusError:
		return failed
	case StatusPending:
		return open + ` AND NOT ` + failed
	case StatusFinished:
		return `id IN (SELECT trace_id FROM observations) AND NOT ` + failed + ` AND NOT ` + open
	}
	return ""
}

// GetObservation returns one span with its reader-facing fields and its step in
// the trace's story, or sql.ErrNoRows.
func (s *Store) GetObservation(ctx context.Context, id string, options TurnOptions) (*Observation, *TurnStep, error) {
	var traceID string
	err := s.db.QueryRowContext(ctx, `SELECT trace_id FROM observations WHERE id = ?`, id).Scan(&traceID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, sql.ErrNoRows
		}
		return nil, nil, fmt.Errorf("find observation %q: %w", id, err)
	}
	trace, err := s.GetTraceWith(ctx, traceID, options)
	if err != nil {
		return nil, nil, err
	}
	for position := range trace.Observations {
		if trace.Observations[position].ID != id {
			continue
		}
		observation := &trace.Observations[position]
		return observation, findStep(trace.Turn, firstNonEmpty(observation.MergedInto, id)), nil
	}
	return nil, nil, sql.ErrNoRows
}

func findStep(turn *Turn, id string) *TurnStep {
	if turn == nil {
		return nil
	}
	for position := range turn.Steps {
		if turn.Steps[position].ObservationID == id {
			return &turn.Steps[position]
		}
	}
	return nil
}

// SpanFilter selects spans across traces for diagnostics.
type SpanFilter struct {
	Name      string
	SessionID string
	From      string
	To        string
	Offset    int
	Limit     int
}

func (f SpanFilter) where(prefix string) (string, []any) {
	conditions := []string{"1 = 1"}
	var args []any
	if value := strings.TrimSpace(f.Name); value != "" {
		conditions = append(conditions, "unicode_contains("+prefix+"name, ?)")
		args = append(args, value)
	}
	if value := strings.TrimSpace(f.SessionID); value != "" {
		conditions = append(conditions, prefix+"trace_id IN (SELECT id FROM traces WHERE session_id = ?)")
		args = append(args, value)
	}
	appendRange(&conditions, &args, prefix+"start_time", f.From, f.To)
	return strings.Join(conditions, " AND "), args
}

// SpanError is one failed span with the trace context needed to follow it up.
type SpanError struct {
	ObservationID string  `json:"observationId"`
	TraceID       string  `json:"traceId"`
	TraceName     string  `json:"traceName,omitempty"`
	SessionID     string  `json:"sessionId,omitempty"`
	Name          string  `json:"name"`
	Level         string  `json:"level"`
	StatusMessage string  `json:"statusMessage,omitempty"`
	StartTime     string  `json:"startTime,omitempty"`
	DurationMs    float64 `json:"durationMs"`
}

// ListSpanErrors lists failed spans, newest first.
func (s *Store) ListSpanErrors(ctx context.Context, filter SpanFilter) (Page[SpanError], error) {
	_, limit := NormalizePage(1, filter.Limit)
	where, args := filter.where("o.")
	where += " AND o." + errorLevelCondition
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM observations o WHERE "+where, args...).Scan(&total); err != nil {
		return Page[SpanError]{}, fmt.Errorf("count span errors: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT o.id, o.trace_id, t.name, t.session_id, o.name, o.level, o.status_message, o.start_time, o.end_time
		FROM observations o LEFT JOIN traces t ON t.id = o.trace_id
		WHERE `+where+`
		ORDER BY o.start_time DESC, o.id DESC LIMIT ? OFFSET ?`, append(args, limit, max(filter.Offset, 0))...)
	if err != nil {
		return Page[SpanError]{}, fmt.Errorf("list span errors: %w", err)
	}
	defer func() { _ = rows.Close() }()
	data := make([]SpanError, 0, limit)
	for rows.Next() {
		var value SpanError
		var traceName, sessionID, name, level, message, start, end nullableString
		if err := rows.Scan(&value.ObservationID, &value.TraceID, &traceName, &sessionID, &name, &level, &message, &start, &end); err != nil {
			return Page[SpanError]{}, fmt.Errorf("scan span error: %w", err)
		}
		value.TraceName, value.SessionID, value.Name = traceName.String, sessionID.String, name.String
		value.Level, value.StatusMessage, value.StartTime = level.String, optionalValue(message.String), start.String
		value.DurationMs = durationMillis(start.String, end.String)
		data = append(data, value)
	}
	if err := rows.Err(); err != nil {
		return Page[SpanError]{}, fmt.Errorf("iterate span errors: %w", err)
	}
	return Page[SpanError]{Data: data, Total: total, Limit: limit}, nil
}

// SlowGroup summarizes the durations of spans sharing a role and label.
type SlowGroup struct {
	Role      string  `json:"role"`
	Label     string  `json:"label"`
	Count     int     `json:"count"`
	Errors    int     `json:"errors,omitempty"`
	P50Ms     float64 `json:"p50Ms"`
	P95Ms     float64 `json:"p95Ms"`
	MaxMs     float64 `json:"maxMs"`
	TotalMs   float64 `json:"totalMs"`
	SlowestID string  `json:"slowestObservationId"`
	SlowestIn string  `json:"slowestTraceId"`
	durations []float64
}

// SlowSpan is one individually slow span.
type SlowSpan struct {
	ObservationID string  `json:"observationId"`
	TraceID       string  `json:"traceId"`
	Role          string  `json:"role"`
	Label         string  `json:"label"`
	Name          string  `json:"name"`
	StartTime     string  `json:"startTime,omitempty"`
	DurationMs    float64 `json:"durationMs"`
	Level         string  `json:"level,omitempty"`
}

// SlowSummary is the result of SummarizeSlowSpans. ScanCapped means more spans
// matched than were read: the summary covers only the newest ScannedSpans.
type SlowSummary struct {
	ScannedSpans int         `json:"scannedSpans"`
	ScanCapped   bool        `json:"scanCapped,omitempty"`
	Groups       []SlowGroup `json:"groups"`
	Slowest      []SlowSpan  `json:"slowest"`
}

// SummarizeSlowSpans measures closed spans by role and label. Timing is the
// effective one and one provider call counts once. Wrappers such as turns and
// requests are measured too, so role narrows the view; "" keeps every role.
func (s *Store) SummarizeSlowSpans(ctx context.Context, filter SpanFilter, role string, topGroups, topSpans int) (SlowSummary, error) {
	where, args := filter.where("")
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, trace_id, parent_observation_id, type, name, model, usage, metadata, level, start_time, end_time,
		       input IS NOT NULL OR output IS NOT NULL
		FROM observations WHERE `+where+`
		ORDER BY start_time DESC LIMIT ?`, append(args, maxTimingRows+1)...)
	if err != nil {
		return SlowSummary{}, fmt.Errorf("list span timings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	byTrace := map[string][]Observation{}
	summary := SlowSummary{Groups: []SlowGroup{}, Slowest: []SlowSpan{}}
	for rows.Next() {
		if summary.ScannedSpans == maxTimingRows {
			summary.ScanCapped = true
			break
		}
		var observation Observation
		var parent, typ, name, model, usage, metadata, level, startTime, endTime nullableString
		if err := rows.Scan(&observation.ID, &observation.TraceID, &parent, &typ, &name, &model, &usage, &metadata, &level, &startTime, &endTime, &observation.hasPayload); err != nil {
			return SlowSummary{}, fmt.Errorf("scan span timing: %w", err)
		}
		observation.ParentObservationID, observation.Type, observation.Name = parent.String, typ.String, name.String
		observation.Model = model.String
		observation.Usage = NormalizeUsage(rawJSON(usage))
		observation.Metadata = rawJSON(metadata)
		observation.Level, observation.StartTime, observation.EndTime = level.String, startTime.String, endTime.String
		observation.Duration = durationMillis(observation.StartTime, observation.EndTime)
		byTrace[observation.TraceID] = append(byTrace[observation.TraceID], observation)
		summary.ScannedSpans++
	}
	if err := rows.Err(); err != nil {
		return SlowSummary{}, fmt.Errorf("iterate span timings: %w", err)
	}
	groups := map[string]*SlowGroup{}
	for traceID := range byTrace {
		observations := byTrace[traceID]
		applyEffectiveTiming(observations)
		annotateObservations(observations)
		for position := range observations {
			observation := &observations[position]
			if observation.MergedInto != "" || observation.EndTime == "" || (role != "" && observation.Role != role) {
				continue
			}
			summary.addSpan(groups, observation)
		}
	}
	summary.finish(groups, topGroups, topSpans)
	return summary, nil
}

func (summary *SlowSummary) addSpan(groups map[string]*SlowGroup, observation *Observation) {
	key := observation.Role + "\x00" + observation.Label
	group := groups[key]
	if group == nil {
		group = &SlowGroup{Role: observation.Role, Label: observation.Label}
		groups[key] = group
	}
	group.Count++
	group.TotalMs += observation.Duration
	group.durations = append(group.durations, observation.Duration)
	if isErrorLevel(observation.Level) {
		group.Errors++
	}
	if observation.Duration >= group.MaxMs {
		group.MaxMs, group.SlowestID, group.SlowestIn = observation.Duration, observation.ID, observation.TraceID
	}
	summary.Slowest = append(summary.Slowest, SlowSpan{ObservationID: observation.ID, TraceID: observation.TraceID, Role: observation.Role,
		Label: observation.Label, Name: observation.Name, StartTime: observation.StartTime, DurationMs: observation.Duration, Level: nonDefaultLevel(observation.Level)})
}

func (summary *SlowSummary) finish(groups map[string]*SlowGroup, topGroups, topSpans int) {
	for _, group := range groups {
		sort.Float64s(group.durations)
		group.P50Ms = percentile(group.durations, 0.50)
		group.P95Ms = percentile(group.durations, 0.95)
		summary.Groups = append(summary.Groups, *group)
	}
	sort.Slice(summary.Groups, func(left, right int) bool {
		if summary.Groups[left].P95Ms != summary.Groups[right].P95Ms {
			return summary.Groups[left].P95Ms > summary.Groups[right].P95Ms
		}
		return summary.Groups[left].Label < summary.Groups[right].Label
	})
	if len(summary.Groups) > topGroups {
		summary.Groups = summary.Groups[:topGroups]
	}
	sort.Slice(summary.Slowest, func(left, right int) bool {
		if summary.Slowest[left].DurationMs != summary.Slowest[right].DurationMs {
			return summary.Slowest[left].DurationMs > summary.Slowest[right].DurationMs
		}
		return summary.Slowest[left].ObservationID < summary.Slowest[right].ObservationID
	})
	if len(summary.Slowest) > topSpans {
		summary.Slowest = summary.Slowest[:topSpans]
	}
}

// percentile uses the nearest-rank method over sorted values.
func percentile(sorted []float64, rank float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	position := int(rank*float64(len(sorted))+0.999999) - 1
	if position < 0 {
		position = 0
	}
	if position >= len(sorted) {
		position = len(sorted) - 1
	}
	return sorted[position]
}
