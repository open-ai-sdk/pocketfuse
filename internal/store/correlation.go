package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// threadSessionPrefix namespaces sessions derived from a thread id, so a
// thread never collides with an explicit session id (or an auth session
// number) sent by the application.
const threadSessionPrefix = "thread:"

// Correlation holds the identifiers an application attaches to its spans that
// tie otherwise separate traces into one conversation.
type Correlation struct {
	ThreadID string
	UserID   string
	JobID    string
	PlanID   string
	AgentID  string
}

// CorrelationFromAttributes reads correlation identifiers from raw span
// attributes. A thread id comes from a `thread_id` attribute or, on HTTP
// spans, from the `{thread_id}` parameter of the route matched against the
// concrete path. `session_id` is deliberately ignored: senders use it for the
// auth session, which spans every conversation of a login.
func CorrelationFromAttributes(attributes map[string]string) Correlation {
	return Correlation{
		ThreadID: firstNonEmpty(optionalValue(attributes["thread_id"]), routeParameter(attributes["http.route"], attributes["url.path"], "thread_id")),
		UserID:   optionalValue(attributes["user_id"]),
		JobID:    optionalValue(attributes["job_id"]),
		PlanID:   optionalValue(attributes["plan_id"]),
		AgentID:  optionalValue(attributes["agent_id"]),
	}
}

// SessionID is the session a correlated trace belongs to, or "" without a thread.
func (c Correlation) SessionID() string {
	if c.ThreadID == "" {
		return ""
	}
	return threadSessionPrefix + c.ThreadID
}

// Metadata lists the identifiers worth showing at trace level.
func (c Correlation) Metadata() map[string]any {
	result := make(map[string]any)
	for key, value := range map[string]string{"thread_id": c.ThreadID, "job_id": c.JobID, "plan_id": c.PlanID, "agent_id": c.AgentID} {
		if value != "" {
			result[key] = value
		}
	}
	return result
}

// OptionalValue unwraps a debug-formatted Rust Option, which is how `?field`
// tracing attributes arrive: `Some(x)` is x and `None` is absent.
func OptionalValue(value string) string { return optionalValue(value) }

func optionalValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "None" {
		return ""
	}
	if strings.HasPrefix(value, "Some(") && strings.HasSuffix(value, ")") {
		value = strings.TrimSuffix(strings.TrimPrefix(value, "Some("), ")")
		return strings.Trim(value, `"`)
	}
	return value
}

// routeParameter extracts `{name}` from path by aligning it with the route
// template segment by segment. A mismatch yields "".
func routeParameter(route, path, name string) string {
	if route == "" || path == "" {
		return ""
	}
	routeParts := strings.Split(strings.Trim(route, "/"), "/")
	pathParts := strings.Split(strings.Trim(path, "/"), "/")
	if len(routeParts) != len(pathParts) {
		return ""
	}
	for index, part := range routeParts {
		if part == "{"+name+"}" {
			return pathParts[index]
		}
	}
	return ""
}

// backfillVersion is the PRAGMA user_version recorded once existing rows carry
// correlation. Spans ingested before correlation existed still hold the
// attributes in their metadata, so the links can be recovered without resending.
const (
	backfillVersion  = 1
	timestampVersion = 2
)

func (s *Store) backfillCorrelation(ctx context.Context) error {
	var version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if version >= backfillVersion {
		return nil
	}
	found, err := s.correlationsFromStoredSpans(ctx)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin correlation backfill: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for traceID, correlation := range found {
		metadata, _ := json.Marshal(correlation.Metadata())
		// An explicit session, user or metadata value always wins over a derived one.
		if _, err := tx.ExecContext(ctx, `
			UPDATE traces SET
				session_id = CASE WHEN session_id = '' THEN ? ELSE session_id END,
				user_id = CASE WHEN user_id = '' THEN ? ELSE user_id END,
				metadata = COALESCE(metadata, ?)
			WHERE id = ?`, correlation.SessionID(), correlation.UserID, string(metadata), traceID); err != nil {
			return fmt.Errorf("backfill trace %q: %w", traceID, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO sessions (id, project_id, user_id, created_at, last_updated_at)
		SELECT session_id, MAX(project_id), MAX(user_id), MIN(timestamp), MAX(timestamp)
		FROM traces WHERE session_id LIKE '`+threadSessionPrefix+`%' GROUP BY session_id
		ON CONFLICT (id) DO NOTHING`); err != nil {
		return fmt.Errorf("backfill sessions: %w", err)
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", backfillVersion)); err != nil {
		return fmt.Errorf("record schema version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit correlation backfill: %w", err)
	}
	return nil
}

// repairTraceTimestamps corrects traces stamped with the first span of a batch.
// Spans export in end order, so the stamp could be minutes after the trace began,
// which misplaced it in lists and time filters. Spans hold the truth.
func (s *Store) repairTraceTimestamps(ctx context.Context) error {
	var version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if version >= timestampVersion {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin timestamp repair: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		UPDATE traces SET timestamp = (
			SELECT MIN(start_time) FROM observations WHERE trace_id = traces.id AND start_time <> ''
		)
		WHERE EXISTS (SELECT 1 FROM observations WHERE trace_id = traces.id AND start_time <> '')`); err != nil {
		return fmt.Errorf("repair trace timestamps: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE sessions SET created_at = (SELECT MIN(timestamp) FROM traces WHERE session_id = sessions.id)
		WHERE EXISTS (SELECT 1 FROM traces WHERE session_id = sessions.id)`); err != nil {
		return fmt.Errorf("repair session start: %w", err)
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", timestampVersion)); err != nil {
		return fmt.Errorf("record schema version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit timestamp repair: %w", err)
	}
	return nil
}

func (s *Store) correlationsFromStoredSpans(ctx context.Context) (map[string]Correlation, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT trace_id, metadata FROM observations
		WHERE metadata LIKE '%thread_id%' OR metadata LIKE '%job_id%'`)
	if err != nil {
		return nil, fmt.Errorf("scan spans for correlation: %w", err)
	}
	defer func() { _ = rows.Close() }()
	found := make(map[string]Correlation)
	for rows.Next() {
		var traceID string
		var metadata nullableString
		if err := rows.Scan(&traceID, &metadata); err != nil {
			return nil, fmt.Errorf("scan span correlation: %w", err)
		}
		var values map[string]any
		if json.Unmarshal([]byte(metadata.String), &values) != nil {
			continue
		}
		current := CorrelationFromAttributes(scalarAttributes(values))
		known := found[traceID]
		found[traceID] = Correlation{
			ThreadID: firstNonEmpty(known.ThreadID, current.ThreadID),
			UserID:   firstNonEmpty(known.UserID, current.UserID),
			JobID:    firstNonEmpty(known.JobID, current.JobID),
			PlanID:   firstNonEmpty(known.PlanID, current.PlanID),
			AgentID:  firstNonEmpty(known.AgentID, current.AgentID),
		}
	}
	return found, rows.Err()
}

func scalarAttributes(values map[string]any) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		switch value := value.(type) {
		case string:
			result[key] = value
		case float64:
			result[key] = strconv.FormatFloat(value, 'f', -1, 64)
		}
	}
	return result
}
