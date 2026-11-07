package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

const (
	maxSessionTraces = 500
	maxOverlapping   = 50
	// overlapLookback bounds how long before a window an overlapping trace may
	// have started. Longer traces are missed rather than scanning all history.
	overlapLookback = 15 * time.Minute
)

// noiseExclusion drops routine polling and maintenance traces: HTTP reads and
// queue housekeeping that consist only of plain spans. A trace that failed or
// recorded anything richer than a span is never noise.
const noiseExclusion = `NOT ((name LIKE 'queue.%' OR name LIKE 'GET %' OR name LIKE 'HEAD %' OR name LIKE 'OPTIONS %')
	AND id NOT IN (SELECT trace_id FROM observations WHERE ` + signalCondition + `))`

const signalCondition = `level IN ('ERROR', 'FATAL', 'error', 'fatal') OR type <> 'SPAN'`

// RelatedTraces lists the traces around one trace. SameSession shares its
// session id, a recorded link. Overlapping only ran at the same time, which
// suggests a link but does not establish one.
type RelatedTraces struct {
	SameSession []Trace `json:"sameSession"`
	Overlapping []Trace `json:"overlapping"`
}

// SessionDetail is one session's timeline. Unlinked holds traces without a
// session that started during one of its requests: a time match, not a link.
type SessionDetail struct {
	Session
	Traces      []Trace `json:"traces"`
	Unlinked    []Trace `json:"unlinked"`
	HiddenNoise int     `json:"hiddenNoise"`
}

func metadataValues(raw json.RawMessage) map[string]any {
	var values map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &values)
	}
	return values
}

// sessionTraces returns a session's traces in chronological order.
func (s *Store) sessionTraces(ctx context.Context, sessionID string, hideNoise bool) ([]Trace, error) {
	page, err := s.ListTraces(ctx, TraceFilter{SessionID: sessionID, HideNoise: hideNoise, Limit: maxLimit})
	if err != nil {
		return nil, err
	}
	traces := page.Data
	for pageNumber := 2; len(traces) < page.Total && len(traces) < maxSessionTraces; pageNumber++ {
		next, err := s.ListTraces(ctx, TraceFilter{SessionID: sessionID, HideNoise: hideNoise, Limit: maxLimit, Page: pageNumber})
		if err != nil {
			return nil, err
		}
		if len(next.Data) == 0 {
			break
		}
		traces = append(traces, next.Data...)
	}
	sortChronologically(traces)
	return traces, nil
}

// GetSessionDetail returns the session and its timeline, or sql.ErrNoRows.
func (s *Store) GetSessionDetail(ctx context.Context, id string, hideNoise bool) (*SessionDetail, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, name, user_id, metadata, created_at, last_updated_at
		FROM sessions WHERE id = ?`, id)
	session, err := scanSession(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("get session %q: %w", id, err)
	}
	traces, err := s.sessionTraces(ctx, id, hideNoise)
	if err != nil {
		return nil, err
	}
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM traces WHERE session_id = ?", id).Scan(&total); err != nil {
		return nil, fmt.Errorf("count session traces: %w", err)
	}
	session.TraceCount = total
	detail := &SessionDetail{Session: session, Traces: traces, Unlinked: []Trace{}}
	if hideNoise {
		detail.HiddenNoise = total - len(traces)
	}
	detail.Unlinked, err = s.unlinkedDuring(ctx, traces)
	if err != nil {
		return nil, err
	}
	return detail, nil
}

// unlinkedDuring finds session-less traces that started inside a request of the
// session. Traces that hold their own generations are skipped as windows: their
// model calls are already linked, and a long job would match unrelated work.
func (s *Store) unlinkedDuring(ctx context.Context, traces []Trace) ([]Trace, error) {
	unlinked := []Trace{}
	seen := make(map[string]bool)
	for position := range traces {
		start, startOK := parseTimestamp(traces[position].StartTime)
		end, endOK := parseTimestamp(traces[position].EndTime)
		if traces[position].hasGeneration || !startOK || !endOK {
			continue
		}
		// Keep each query inside one request window: a thread may span days.
		// Page through every candidate before moving to its later requests.
		for page := 0; ; page++ {
			candidates, err := s.tracesStartingBetweenPage(ctx, start, end, " AND session_id = ''", page)
			if err != nil {
				return nil, err
			}
			for _, candidate := range candidates {
				candidateStart, ok := parseTimestamp(candidate.StartTime)
				if !ok || seen[candidate.ID] || candidateStart.Before(start) || candidateStart.After(end) {
					continue
				}
				seen[candidate.ID] = true
				unlinked = append(unlinked, candidate)
			}
			if len(candidates) < maxSessionTraces {
				break
			}
		}
	}
	sortChronologically(unlinked)
	return unlinked, nil
}

// GetRelatedTraces returns the traces linked to id or overlapping it, or
// sql.ErrNoRows when it does not exist.
func (s *Store) GetRelatedTraces(ctx context.Context, id string) (*RelatedTraces, error) {
	trace, err := s.GetTrace(ctx, id)
	if err != nil {
		return nil, err
	}
	related := &RelatedTraces{SameSession: []Trace{}, Overlapping: []Trace{}}
	linked := map[string]bool{id: true}
	if trace.SessionID != "" {
		traces, err := s.sessionTraces(ctx, trace.SessionID, true)
		if err != nil {
			return nil, err
		}
		for _, candidate := range traces {
			linked[candidate.ID] = true
			if candidate.ID != id {
				related.SameSession = append(related.SameSession, candidate)
			}
		}
	}
	start, startOK := parseTimestamp(trace.StartTime)
	end, endOK := parseTimestamp(trace.EndTime)
	if !startOK || !endOK {
		return related, nil
	}
	candidates, err := s.tracesStartingBetween(ctx, start.Add(-overlapLookback), end, "")
	if err != nil {
		return nil, err
	}
	for _, candidate := range candidates {
		candidateStart, startOK := parseTimestamp(candidate.StartTime)
		candidateEnd, endOK := parseTimestamp(candidate.EndTime)
		if linked[candidate.ID] || !startOK || !endOK || candidateEnd.Before(start) || candidateStart.After(end) {
			continue
		}
		related.Overlapping = append(related.Overlapping, candidate)
		if len(related.Overlapping) >= maxOverlapping {
			break
		}
	}
	return related, nil
}

// tracesStartingBetween lists non-noise traces by start time, oldest first.
// Stored timestamps drop trailing zeros and so compare only roughly as text;
// the bounds are widened here and callers apply the exact check.
func (s *Store) tracesStartingBetween(ctx context.Context, from, to time.Time, condition string) ([]Trace, error) {
	return s.tracesStartingBetweenPage(ctx, from, to, condition, 0)
}

func (s *Store) tracesStartingBetweenPage(ctx context.Context, from, to time.Time, condition string, page int) ([]Trace, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, name, user_id, session_id, environment, release, version,
		       input, output, metadata, tags, public, timestamp, start_time, end_time,
		       created_at, updated_at
		FROM traces
		WHERE timestamp >= ? AND timestamp <= ? AND `+noiseExclusion+condition+`
		ORDER BY timestamp ASC, id ASC LIMIT ? OFFSET ?`,
		from.Add(-time.Second).UTC().Format(time.RFC3339Nano),
		to.Add(time.Second).UTC().Format(time.RFC3339Nano),
		maxSessionTraces, page*maxSessionTraces)
	if err != nil {
		return nil, fmt.Errorf("list traces in window: %w", err)
	}
	defer func() { _ = rows.Close() }()
	traces := []Trace{}
	for rows.Next() {
		value, err := scanTrace(rows)
		if err != nil {
			return nil, fmt.Errorf("scan trace in window: %w", err)
		}
		traces = append(traces, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate traces in window: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close traces in window: %w", err)
	}
	if err := s.attachSummaries(ctx, traces); err != nil {
		return nil, err
	}
	sortChronologically(traces)
	return traces, nil
}

func sortChronologically(traces []Trace) {
	sort.SliceStable(traces, func(left, right int) bool {
		leftStart, _ := parseTimestamp(firstNonEmpty(traces[left].StartTime, traces[left].Timestamp))
		rightStart, _ := parseTimestamp(firstNonEmpty(traces[right].StartTime, traces[right].Timestamp))
		return leftStart.Before(rightStart)
	})
}
