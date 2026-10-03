package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

func (s *Store) ListTraces(ctx context.Context, filter TraceFilter) (Page[Trace], error) {
	page, limit := NormalizePage(filter.Page, filter.Limit)
	where, args := traceWhere(filter)
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM traces WHERE "+where, args...).Scan(&total); err != nil {
		return Page[Trace]{}, fmt.Errorf("count traces: %w", err)
	}
	args = append(args, limit, (page-1)*limit)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, name, user_id, session_id, environment, release, version,
		       input, output, metadata, tags, public, timestamp, start_time, end_time,
		       created_at, updated_at
		FROM traces WHERE `+where+`
		ORDER BY CASE WHEN timestamp = '' THEN 1 ELSE 0 END, timestamp DESC, id DESC
		LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return Page[Trace]{}, fmt.Errorf("list traces: %w", err)
	}
	defer func() { _ = rows.Close() }()
	data := make([]Trace, 0, min(limit, total))
	for rows.Next() {
		value, err := scanTrace(rows)
		if err != nil {
			return Page[Trace]{}, fmt.Errorf("scan trace: %w", err)
		}
		data = append(data, value)
	}
	if err := rows.Err(); err != nil {
		return Page[Trace]{}, fmt.Errorf("iterate traces: %w", err)
	}
	return Page[Trace]{Data: data, Total: total, Page: page, Limit: limit}, nil
}

func (s *Store) GetTrace(ctx context.Context, id string) (*Trace, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, name, user_id, session_id, environment, release, version,
		       input, output, metadata, tags, public, timestamp, start_time, end_time,
		       created_at, updated_at
		FROM traces WHERE id = ?`, id)
	value, err := scanTrace(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("get trace %q: %w", id, err)
	}
	observations, err := s.listTraceObservations(ctx, id)
	if err != nil {
		return nil, err
	}
	value.Observations = observations
	value.ObservationCount = len(observations)
	for _, observation := range observations {
		if strings.EqualFold(observation.Level, "error") || strings.EqualFold(observation.Level, "fatal") {
			value.Status = "error"
			break
		}
	}
	if value.Status == "" && len(observations) > 0 {
		value.Status = "success"
	}
	return &value, nil
}

func (s *Store) ListObservations(ctx context.Context, filter ObservationFilter) (Page[Observation], error) {
	page, limit := NormalizePage(filter.Page, filter.Limit)
	where, args := observationWhere(filter)
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM observations WHERE "+where, args...).Scan(&total); err != nil {
		return Page[Observation]{}, fmt.Errorf("count observations: %w", err)
	}
	args = append(args, limit, (page-1)*limit)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, trace_id, project_id, parent_observation_id, type, name, input, output,
		       metadata, model, model_parameters, usage, level, status_message, version,
		       environment, start_time, end_time, completion_start_time, created_at, updated_at
		FROM observations WHERE `+where+`
		ORDER BY CASE WHEN start_time = '' THEN 1 ELSE 0 END, start_time DESC, id DESC
		LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return Page[Observation]{}, fmt.Errorf("list observations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	data := make([]Observation, 0, min(limit, total))
	for rows.Next() {
		value, err := scanObservation(rows)
		if err != nil {
			return Page[Observation]{}, fmt.Errorf("scan observation: %w", err)
		}
		data = append(data, value)
	}
	if err := rows.Err(); err != nil {
		return Page[Observation]{}, fmt.Errorf("iterate observations: %w", err)
	}
	return Page[Observation]{Data: data, Total: total, Page: page, Limit: limit}, nil
}

func (s *Store) ListSessions(ctx context.Context, filter SessionFilter) (Page[Session], error) {
	page, limit := NormalizePage(filter.Page, filter.Limit)
	where, args := sessionWhere(filter)
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sessions WHERE "+where, args...).Scan(&total); err != nil {
		return Page[Session]{}, fmt.Errorf("count sessions: %w", err)
	}
	args = append(args, limit, (page-1)*limit)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, name, user_id, metadata, created_at, last_updated_at
		FROM sessions WHERE `+where+`
		ORDER BY CASE WHEN last_updated_at = '' THEN 1 ELSE 0 END, last_updated_at DESC, id DESC
		LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return Page[Session]{}, fmt.Errorf("list sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	data := make([]Session, 0, min(limit, total))
	for rows.Next() {
		value, err := scanSession(rows)
		if err != nil {
			return Page[Session]{}, fmt.Errorf("scan session: %w", err)
		}
		data = append(data, value)
	}
	if err := rows.Err(); err != nil {
		return Page[Session]{}, fmt.Errorf("iterate sessions: %w", err)
	}
	return Page[Session]{Data: data, Total: total, Page: page, Limit: limit}, nil
}

func (s *Store) ListScores(ctx context.Context, filter ScoreFilter) (Page[Score], error) {
	page, limit := NormalizePage(filter.Page, filter.Limit)
	where, args := scoreWhere(filter)
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM scores WHERE "+where, args...).Scan(&total); err != nil {
		return Page[Score]{}, fmt.Errorf("count scores: %w", err)
	}
	args = append(args, limit, (page-1)*limit)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, trace_id, observation_id, name, value, string_value, data_type,
		       comment, source, timestamp, created_at
		FROM scores WHERE `+where+`
		ORDER BY CASE WHEN timestamp = '' THEN 1 ELSE 0 END, timestamp DESC, id DESC
		LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return Page[Score]{}, fmt.Errorf("list scores: %w", err)
	}
	defer func() { _ = rows.Close() }()
	data := make([]Score, 0, min(limit, total))
	for rows.Next() {
		value, err := scanScore(rows)
		if err != nil {
			return Page[Score]{}, fmt.Errorf("scan score: %w", err)
		}
		data = append(data, value)
	}
	if err := rows.Err(); err != nil {
		return Page[Score]{}, fmt.Errorf("iterate scores: %w", err)
	}
	return Page[Score]{Data: data, Total: total, Page: page, Limit: limit}, nil
}

func (s *Store) listTraceObservations(ctx context.Context, id string) ([]Observation, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, trace_id, project_id, parent_observation_id, type, name, input, output,
		       metadata, model, model_parameters, usage, level, status_message, version,
		       environment, start_time, end_time, completion_start_time, created_at, updated_at
		FROM observations WHERE trace_id = ?
		ORDER BY CASE WHEN start_time = '' THEN 1 ELSE 0 END, start_time ASC, id ASC`, id)
	if err != nil {
		return nil, fmt.Errorf("list trace observations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	data := make([]Observation, 0)
	for rows.Next() {
		value, err := scanObservation(rows)
		if err != nil {
			return nil, fmt.Errorf("scan trace observation: %w", err)
		}
		data = append(data, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate trace observations: %w", err)
	}
	return data, nil
}

func traceWhere(filter TraceFilter) (string, []any) {
	conditions := []string{"1 = 1"}
	var args []any
	if value := strings.TrimSpace(filter.Project); value != "" {
		conditions = append(conditions, "project_id = ?")
		args = append(args, value)
	}
	if value := strings.TrimSpace(filter.Name); value != "" {
		conditions = append(conditions, "name ILIKE ?")
		args = append(args, "%"+value+"%")
	}
	appendRange(&conditions, &args, "timestamp", filter.From, filter.To)
	return strings.Join(conditions, " AND "), args
}

func observationWhere(filter ObservationFilter) (string, []any) {
	conditions := []string{"1 = 1"}
	var args []any
	if value := strings.TrimSpace(filter.TraceID); value != "" {
		conditions = append(conditions, "trace_id = ?")
		args = append(args, value)
	}
	if value := strings.TrimSpace(filter.Project); value != "" {
		conditions = append(conditions, "project_id = ?")
		args = append(args, value)
	}
	if value := strings.TrimSpace(filter.Name); value != "" {
		conditions = append(conditions, "name ILIKE ?")
		args = append(args, "%"+value+"%")
	}
	if value := strings.TrimSpace(filter.Type); value != "" {
		conditions = append(conditions, "type = ?")
		args = append(args, strings.ToUpper(value))
	}
	appendRange(&conditions, &args, "start_time", filter.From, filter.To)
	return strings.Join(conditions, " AND "), args
}

func sessionWhere(filter SessionFilter) (string, []any) {
	conditions := []string{"1 = 1"}
	var args []any
	if value := strings.TrimSpace(filter.Project); value != "" {
		conditions = append(conditions, "project_id = ?")
		args = append(args, value)
	}
	if value := strings.TrimSpace(filter.Name); value != "" {
		conditions = append(conditions, "name ILIKE ?")
		args = append(args, "%"+value+"%")
	}
	if value := strings.TrimSpace(filter.UserID); value != "" {
		conditions = append(conditions, "user_id = ?")
		args = append(args, value)
	}
	appendRange(&conditions, &args, "last_updated_at", filter.From, filter.To)
	return strings.Join(conditions, " AND "), args
}

func scoreWhere(filter ScoreFilter) (string, []any) {
	conditions := []string{"1 = 1"}
	var args []any
	if value := strings.TrimSpace(filter.TraceID); value != "" {
		conditions = append(conditions, "trace_id = ?")
		args = append(args, value)
	}
	if value := strings.TrimSpace(filter.ObservationID); value != "" {
		conditions = append(conditions, "observation_id = ?")
		args = append(args, value)
	}
	if value := strings.TrimSpace(filter.Name); value != "" {
		conditions = append(conditions, "name ILIKE ?")
		args = append(args, "%"+value+"%")
	}
	if value := strings.TrimSpace(filter.Source); value != "" {
		conditions = append(conditions, "source = ?")
		args = append(args, value)
	}
	return strings.Join(conditions, " AND "), args
}

func appendRange(conditions *[]string, args *[]any, column, from, to string) {
	if value := strings.TrimSpace(from); value != "" {
		*conditions = append(*conditions, column+" >= ?")
		*args = append(*args, value)
	}
	if value := strings.TrimSpace(to); value != "" {
		*conditions = append(*conditions, column+" <= ?")
		*args = append(*args, value)
	}
}

type scanner interface{ Scan(dest ...any) error }

func scanTrace(row scanner) (Trace, error) {
	var (
		value                            Trace
		input, output, metadata, tags    nullableString
		project, name, userID, sessionID nullableString
		environment, release, version    nullableString
		timestamp, startTime, endTime    nullableString
		createdAt, updatedAt             nullableString
	)
	err := row.Scan(
		&value.ID, &project, &name, &userID, &sessionID, &environment, &release, &version,
		&input, &output, &metadata, &tags, &value.Public, &timestamp, &startTime, &endTime,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return Trace{}, err
	}
	value.ProjectID = stringValue(project)
	value.Project = value.ProjectID
	value.Name = stringValue(name)
	value.UserID = stringValue(userID)
	value.SessionID = stringValue(sessionID)
	value.Environment = stringValue(environment)
	value.Release = stringValue(release)
	value.Version = stringValue(version)
	value.Input = rawJSON(input)
	value.Output = rawJSON(output)
	value.Metadata = rawJSON(metadata)
	value.Tags = parseStrings(tags)
	value.Timestamp = scanTimestamp(timestamp)
	value.StartTime = scanTimestamp(startTime)
	value.EndTime = scanTimestamp(endTime)
	value.CreatedAt = scanTimestamp(createdAt)
	value.UpdatedAt = scanTimestamp(updatedAt)
	value.Duration = durationMillis(value.StartTime, value.EndTime)
	value.Latency = value.Duration
	return value, nil
}

func scanObservation(row scanner) (Observation, error) {
	var (
		value                                                         Observation
		traceID, project, parentID, typ, name                         nullableString
		input, output, metadata, model, modelParameters, usage        nullableString
		level, statusMessage, version, environment                    nullableString
		startTime, endTime, completionStartTime, createdAt, updatedAt nullableString
	)
	err := row.Scan(
		&value.ID, &traceID, &project, &parentID, &typ, &name, &input, &output,
		&metadata, &model, &modelParameters, &usage, &level, &statusMessage, &version,
		&environment, &startTime, &endTime, &completionStartTime, &createdAt, &updatedAt,
	)
	if err != nil {
		return Observation{}, err
	}
	value.TraceID = stringValue(traceID)
	value.ProjectID = stringValue(project)
	value.ParentObservationID = stringValue(parentID)
	value.Type = stringValue(typ)
	value.Name = stringValue(name)
	value.Input = rawJSON(input)
	value.Output = rawJSON(output)
	value.Metadata = rawJSON(metadata)
	value.Model = stringValue(model)
	value.ModelParameters = rawJSON(modelParameters)
	value.Usage = rawJSON(usage)
	value.Level = stringValue(level)
	value.StatusMessage = stringValue(statusMessage)
	value.Version = stringValue(version)
	value.Environment = stringValue(environment)
	value.StartTime = scanTimestamp(startTime)
	value.EndTime = scanTimestamp(endTime)
	value.CompletionStartTime = scanTimestamp(completionStartTime)
	value.CreatedAt = scanTimestamp(createdAt)
	value.UpdatedAt = scanTimestamp(updatedAt)
	value.Duration = durationMillis(value.StartTime, value.EndTime)
	return value, nil
}

func scanSession(row scanner) (Session, error) {
	var (
		value                           Session
		project, name, userID, metadata nullableString
		createdAt, lastUpdatedAt        nullableString
	)
	err := row.Scan(&value.ID, &project, &name, &userID, &metadata, &createdAt, &lastUpdatedAt)
	if err != nil {
		return Session{}, err
	}
	value.ProjectID = stringValue(project)
	value.Name = stringValue(name)
	value.UserID = stringValue(userID)
	value.Metadata = rawJSON(metadata)
	value.CreatedAt = stringValue(createdAt)
	value.LastUpdatedAt = stringValue(lastUpdatedAt)
	return value, nil
}

func scanScore(row scanner) (Score, error) {
	var (
		value                                 Score
		traceID, observationID, name          nullableString
		stringValueValue, dataType            nullableString
		comment, source, timestamp, createdAt nullableString
		numeric                               sql.NullFloat64
	)
	err := row.Scan(
		&value.ID, &traceID, &observationID, &name, &numeric, &stringValueValue, &dataType,
		&comment, &source, &timestamp, &createdAt,
	)
	if err != nil {
		return Score{}, err
	}
	value.TraceID = stringValue(traceID)
	value.ObservationID = stringValue(observationID)
	value.Name = stringValue(name)
	if numeric.Valid {
		value.Value = numeric.Float64
	}
	value.StringValue = stringValue(stringValueValue)
	value.DataType = stringValue(dataType)
	value.Comment = stringValue(comment)
	value.Source = stringValue(source)
	value.Timestamp = stringValue(timestamp)
	value.CreatedAt = stringValue(createdAt)
	return value, nil
}

func parseStrings(value nullableString) []string {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return nil
	}
	var result []string
	if err := json.Unmarshal([]byte(value.String), &result); err != nil {
		return nil
	}
	return result
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func durationMillis(start, end string) float64 {
	startTime, startOK := parseTimestamp(start)
	endTime, endOK := parseTimestamp(end)
	if !startOK || !endOK || endTime.Before(startTime) {
		return 0
	}
	return float64(endTime.Sub(startTime)) / float64(1e6)
}
