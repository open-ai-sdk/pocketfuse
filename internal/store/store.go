// Package store owns Pocketfuse's local persistence layer.
//
// The database deliberately keeps the payload columns as JSON text. Ingested
// OTLP and JSON payloads evolve quickly; preserving the original JSON makes
// the MVP tolerant of fields that are not yet promoted to first-class columns.
package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	defaultPage  = 1
	defaultLimit = 25
	maxLimit     = 200
)

// Trace is a trace record as returned by the API.
type Trace struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId,omitempty"`
	// Project is a UI-friendly alias used by Pocketfuse's lightweight client.
	Project          string          `json:"project,omitempty"`
	Name             string          `json:"name,omitempty"`
	UserID           string          `json:"userId,omitempty"`
	SessionID        string          `json:"sessionId,omitempty"`
	Environment      string          `json:"environment,omitempty"`
	Release          string          `json:"release,omitempty"`
	Version          string          `json:"version,omitempty"`
	Input            json.RawMessage `json:"input,omitempty"`
	Output           json.RawMessage `json:"output,omitempty"`
	Metadata         json.RawMessage `json:"metadata,omitempty"`
	Tags             []string        `json:"tags,omitempty"`
	Public           bool            `json:"public,omitempty"`
	Timestamp        string          `json:"timestamp,omitempty"`
	StartTime        string          `json:"startTime,omitempty"`
	EndTime          string          `json:"endTime,omitempty"`
	CreatedAt        string          `json:"createdAt,omitempty"`
	UpdatedAt        string          `json:"updatedAt,omitempty"`
	Duration         float64         `json:"duration,omitempty"`
	Latency          float64         `json:"latency,omitempty"`
	Status           string          `json:"status,omitempty"`
	ObservationCount int             `json:"observationCount,omitempty"`

	// Observations is populated for the trace detail endpoint.
	Observations []Observation `json:"observations,omitempty"`
}

// Observation is a span, generation, event, or agent observation.
type Observation struct {
	ID                  string          `json:"id"`
	TraceID             string          `json:"traceId"`
	ProjectID           string          `json:"projectId,omitempty"`
	ParentObservationID string          `json:"parentObservationId,omitempty"`
	Type                string          `json:"type,omitempty"`
	Name                string          `json:"name,omitempty"`
	Input               json.RawMessage `json:"input,omitempty"`
	Output              json.RawMessage `json:"output,omitempty"`
	Metadata            json.RawMessage `json:"metadata,omitempty"`
	Model               string          `json:"model,omitempty"`
	ModelParameters     json.RawMessage `json:"modelParameters,omitempty"`
	Usage               json.RawMessage `json:"usage,omitempty"`
	Level               string          `json:"level,omitempty"`
	StatusMessage       string          `json:"statusMessage,omitempty"`
	Version             string          `json:"version,omitempty"`
	Environment         string          `json:"environment,omitempty"`
	StartTime           string          `json:"startTime,omitempty"`
	EndTime             string          `json:"endTime,omitempty"`
	CompletionStartTime string          `json:"completionStartTime,omitempty"`
	CreatedAt           string          `json:"createdAt,omitempty"`
	UpdatedAt           string          `json:"updatedAt,omitempty"`
	Duration            float64         `json:"duration,omitempty"`
}

// Session is a grouping of traces as returned by the sessions list.
type Session struct {
	ID            string          `json:"id"`
	ProjectID     string          `json:"projectId,omitempty"`
	Name          string          `json:"name,omitempty"`
	UserID        string          `json:"userId,omitempty"`
	Metadata      json.RawMessage `json:"metadata,omitempty"`
	CreatedAt     string          `json:"createdAt,omitempty"`
	LastUpdatedAt string          `json:"lastUpdatedAt,omitempty"`
}

// Score is a numeric or categorical evaluation attached to a trace or
// observation.
type Score struct {
	ID            string  `json:"id"`
	TraceID       string  `json:"traceId,omitempty"`
	ObservationID string  `json:"observationId,omitempty"`
	Name          string  `json:"name"`
	Value         float64 `json:"value,omitempty"`
	StringValue   string  `json:"stringValue,omitempty"`
	DataType      string  `json:"dataType,omitempty"`
	Comment       string  `json:"comment,omitempty"`
	Source        string  `json:"source,omitempty"`
	Timestamp     string  `json:"timestamp,omitempty"`
	CreatedAt     string  `json:"createdAt,omitempty"`
}

// Page is the stable list response envelope consumed by the frontend.
type Page[T any] struct {
	Data  []T `json:"data"`
	Total int `json:"total"`
	Page  int `json:"page"`
	Limit int `json:"limit"`
}

type TraceFilter struct {
	Project string
	Name    string
	From    string
	To      string
	Page    int
	Limit   int
}

type ObservationFilter struct {
	TraceID string
	Project string
	Name    string
	Type    string
	From    string
	To      string
	Page    int
	Limit   int
}

type SessionFilter struct {
	Project string
	Name    string
	UserID  string
	From    string
	To      string
	Page    int
	Limit   int
}

type ScoreFilter struct {
	TraceID       string
	ObservationID string
	Name          string
	Source        string
	Page          int
	Limit         int
}

// RecordType identifies one normalized ingestion record.
type RecordType string

const (
	RecordTrace       RecordType = "trace"
	RecordObservation RecordType = "observation"
	RecordSession     RecordType = "session"
	RecordScore       RecordType = "score"
)

// IngestRecord is the server/store boundary. Decoding compatibility formats
// belongs to the HTTP layer; persistence receives one of these normalized
// records.
type IngestRecord struct {
	Type        RecordType
	Trace       *Trace
	Observation *Observation
	Session     *Session
	Score       *Score
}

type IngestResult struct {
	Accepted     int `json:"accepted"`
	Traces       int `json:"traces"`
	Observations int `json:"observations"`
	Sessions     int `json:"sessions"`
	Scores       int `json:"scores"`
}

// Store wraps a single SQLite database. SQLite permits a single writer, so
// the pool is intentionally kept to one connection for predictable local
// ingestion.
type Store struct {
	db *sql.DB
}

// Open opens or creates a SQLite database and applies the additive MVP schema.
func Open(ctx context.Context, path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		path = ":memory:"
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	store := &Store{db: db}
	if err := store.init(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

// New is a convenience for callers that do not need a context during startup.
func New(path string) (*Store, error) {
	return Open(context.Background(), path)
}

// NewStore wraps an already-open SQL database. It is useful for tests and for
// applications that need to control the driver's lifecycle themselves.
func NewStore(ctx context.Context, db *sql.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("pocketfuse store: nil database")
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &Store{db: db}
	if err := store.init(ctx); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) init(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping sqlite: %w", err)
	}
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := s.db.ExecContext(ctx, pragma); err != nil {
			return fmt.Errorf("set pragma %q: %w", pragma, err)
		}
	}
	for _, statement := range schemaStatements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initialize schema: %w", err)
		}
	}
	return nil
}

var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS traces (
		id VARCHAR PRIMARY KEY,
		project_id VARCHAR NOT NULL DEFAULT '',
		name VARCHAR NOT NULL DEFAULT '',
		user_id VARCHAR NOT NULL DEFAULT '',
		session_id VARCHAR NOT NULL DEFAULT '',
		environment VARCHAR NOT NULL DEFAULT '',
		release VARCHAR NOT NULL DEFAULT '',
		version VARCHAR NOT NULL DEFAULT '',
		input VARCHAR,
		output VARCHAR,
		metadata VARCHAR,
		tags VARCHAR,
		public BOOLEAN NOT NULL DEFAULT FALSE,
		timestamp VARCHAR NOT NULL DEFAULT '',
		start_time VARCHAR NOT NULL DEFAULT '',
		end_time VARCHAR NOT NULL DEFAULT '',
		created_at VARCHAR NOT NULL DEFAULT '',
		updated_at VARCHAR NOT NULL DEFAULT ''
	)`,
	`CREATE INDEX IF NOT EXISTS traces_project_timestamp_idx ON traces(project_id, timestamp, id)`,
	`CREATE INDEX IF NOT EXISTS traces_session_idx ON traces(session_id, project_id)`,
	`CREATE TABLE IF NOT EXISTS observations (
		id VARCHAR PRIMARY KEY,
		trace_id VARCHAR NOT NULL,
		project_id VARCHAR NOT NULL DEFAULT '',
		parent_observation_id VARCHAR NOT NULL DEFAULT '',
		type VARCHAR NOT NULL DEFAULT 'SPAN',
		name VARCHAR NOT NULL DEFAULT '',
		input VARCHAR,
		output VARCHAR,
		metadata VARCHAR,
		model VARCHAR NOT NULL DEFAULT '',
		model_parameters VARCHAR,
		usage VARCHAR,
		level VARCHAR NOT NULL DEFAULT 'DEFAULT',
		status_message VARCHAR NOT NULL DEFAULT '',
		version VARCHAR NOT NULL DEFAULT '',
		environment VARCHAR NOT NULL DEFAULT '',
		start_time VARCHAR NOT NULL DEFAULT '',
		end_time VARCHAR NOT NULL DEFAULT '',
		completion_start_time VARCHAR NOT NULL DEFAULT '',
		created_at VARCHAR NOT NULL DEFAULT '',
		updated_at VARCHAR NOT NULL DEFAULT ''
	)`,
	`CREATE INDEX IF NOT EXISTS observations_trace_start_idx ON observations(trace_id, start_time, id)`,
	`CREATE INDEX IF NOT EXISTS observations_project_start_idx ON observations(project_id, start_time, id)`,
	`CREATE TABLE IF NOT EXISTS sessions (
		id VARCHAR PRIMARY KEY,
		project_id VARCHAR NOT NULL DEFAULT '',
		name VARCHAR NOT NULL DEFAULT '',
		user_id VARCHAR NOT NULL DEFAULT '',
		metadata VARCHAR,
		created_at VARCHAR NOT NULL DEFAULT '',
		last_updated_at VARCHAR NOT NULL DEFAULT ''
	)`,
	`CREATE INDEX IF NOT EXISTS sessions_project_updated_idx ON sessions(project_id, last_updated_at, id)`,
	`CREATE TABLE IF NOT EXISTS scores (
		id VARCHAR PRIMARY KEY,
		trace_id VARCHAR NOT NULL DEFAULT '',
		observation_id VARCHAR NOT NULL DEFAULT '',
		name VARCHAR NOT NULL DEFAULT '',
		value DOUBLE,
		string_value VARCHAR NOT NULL DEFAULT '',
		data_type VARCHAR NOT NULL DEFAULT '',
		comment VARCHAR NOT NULL DEFAULT '',
		source VARCHAR NOT NULL DEFAULT '',
		timestamp VARCHAR NOT NULL DEFAULT '',
		created_at VARCHAR NOT NULL DEFAULT ''
	)`,
	`CREATE INDEX IF NOT EXISTS scores_trace_idx ON scores(trace_id, timestamp, id)`,
}

// NormalizePage applies the API's pagination bounds. It is exported so the
// handler and tests use exactly the same behavior.
func NormalizePage(page, limit int) (int, int) {
	if page < 1 {
		page = defaultPage
	}
	if limit < 1 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	return page, limit
}

func (s *Store) Ingest(ctx context.Context, records []IngestRecord) (IngestResult, error) {
	var result IngestResult
	if len(records) == 0 {
		return result, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, fmt.Errorf("begin ingestion: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	for _, record := range records {
		switch record.Type {
		case RecordTrace:
			if record.Trace == nil {
				return result, errors.New("ingestion trace record is nil")
			}
			if err = upsertTrace(ctx, tx, *record.Trace); err != nil {
				return result, err
			}
			result.Traces++
			result.Accepted++
			if record.Trace.SessionID != "" {
				session := Session{
					ID:            record.Trace.SessionID,
					ProjectID:     record.Trace.ProjectID,
					UserID:        record.Trace.UserID,
					CreatedAt:     record.Trace.Timestamp,
					LastUpdatedAt: firstNonEmpty(record.Trace.EndTime, record.Trace.Timestamp),
				}
				if err = upsertSession(ctx, tx, session); err != nil {
					return result, err
				}
			}
		case RecordObservation:
			if record.Observation == nil {
				return result, errors.New("ingestion observation record is nil")
			}
			if err = upsertObservation(ctx, tx, *record.Observation); err != nil {
				return result, err
			}
			result.Observations++
			result.Accepted++
		case RecordSession:
			if record.Session == nil {
				return result, errors.New("ingestion session record is nil")
			}
			if err = upsertSession(ctx, tx, *record.Session); err != nil {
				return result, err
			}
			result.Sessions++
			result.Accepted++
		case RecordScore:
			if record.Score == nil {
				return result, errors.New("ingestion score record is nil")
			}
			if err = upsertScore(ctx, tx, *record.Score); err != nil {
				return result, err
			}
			result.Scores++
			result.Accepted++
		default:
			return result, fmt.Errorf("unknown ingestion record type %q", record.Type)
		}
	}
	if err = tx.Commit(); err != nil {
		return result, fmt.Errorf("commit ingestion: %w", err)
	}
	return result, nil
}

func upsertTrace(ctx context.Context, tx *sql.Tx, value Trace) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO traces (
			id, project_id, name, user_id, session_id, environment, release, version,
			input, output, metadata, tags, public, timestamp, start_time, end_time,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			project_id = COALESCE(NULLIF(excluded.project_id, ''), traces.project_id),
			name = COALESCE(NULLIF(excluded.name, ''), traces.name),
			user_id = COALESCE(NULLIF(excluded.user_id, ''), traces.user_id),
			session_id = COALESCE(NULLIF(excluded.session_id, ''), traces.session_id),
			environment = COALESCE(NULLIF(excluded.environment, ''), traces.environment),
			release = COALESCE(NULLIF(excluded.release, ''), traces.release),
			version = COALESCE(NULLIF(excluded.version, ''), traces.version),
			input = COALESCE(excluded.input, traces.input),
			output = COALESCE(excluded.output, traces.output),
			metadata = COALESCE(excluded.metadata, traces.metadata),
			tags = COALESCE(excluded.tags, traces.tags),
			public = excluded.public,
			timestamp = COALESCE(NULLIF(excluded.timestamp, ''), traces.timestamp),
			start_time = COALESCE(NULLIF(excluded.start_time, ''), traces.start_time),
			end_time = COALESCE(NULLIF(excluded.end_time, ''), traces.end_time),
			created_at = COALESCE(NULLIF(traces.created_at, ''), excluded.created_at),
			updated_at = COALESCE(NULLIF(excluded.updated_at, ''), traces.updated_at)`,
		value.ID,
		value.ProjectID,
		value.Name,
		value.UserID,
		value.SessionID,
		value.Environment,
		value.Release,
		value.Version,
		nullableJSON(value.Input),
		nullableJSON(value.Output),
		nullableJSON(value.Metadata),
		nullableStrings(value.Tags),
		value.Public,
		value.Timestamp,
		value.StartTime,
		value.EndTime,
		value.CreatedAt,
		value.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert trace %q: %w", value.ID, err)
	}
	return nil
}

func upsertObservation(ctx context.Context, tx *sql.Tx, value Observation) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO observations (
			id, trace_id, project_id, parent_observation_id, type, name, input, output,
			metadata, model, model_parameters, usage, level, status_message, version,
			environment, start_time, end_time, completion_start_time, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			trace_id = COALESCE(NULLIF(excluded.trace_id, ''), observations.trace_id),
			project_id = COALESCE(NULLIF(excluded.project_id, ''), observations.project_id),
			parent_observation_id = COALESCE(NULLIF(excluded.parent_observation_id, ''), observations.parent_observation_id),
			type = COALESCE(NULLIF(excluded.type, ''), observations.type),
			name = COALESCE(NULLIF(excluded.name, ''), observations.name),
			input = COALESCE(excluded.input, observations.input),
			output = COALESCE(excluded.output, observations.output),
			metadata = COALESCE(excluded.metadata, observations.metadata),
			model = COALESCE(NULLIF(excluded.model, ''), observations.model),
			model_parameters = COALESCE(excluded.model_parameters, observations.model_parameters),
			usage = COALESCE(excluded.usage, observations.usage),
			level = COALESCE(NULLIF(excluded.level, ''), observations.level),
			status_message = COALESCE(NULLIF(excluded.status_message, ''), observations.status_message),
			version = COALESCE(NULLIF(excluded.version, ''), observations.version),
			environment = COALESCE(NULLIF(excluded.environment, ''), observations.environment),
			start_time = COALESCE(NULLIF(excluded.start_time, ''), observations.start_time),
			end_time = COALESCE(NULLIF(excluded.end_time, ''), observations.end_time),
			completion_start_time = COALESCE(NULLIF(excluded.completion_start_time, ''), observations.completion_start_time),
			created_at = COALESCE(NULLIF(observations.created_at, ''), excluded.created_at),
			updated_at = COALESCE(NULLIF(excluded.updated_at, ''), observations.updated_at)`,
		value.ID,
		value.TraceID,
		value.ProjectID,
		value.ParentObservationID,
		value.Type,
		value.Name,
		nullableJSON(value.Input),
		nullableJSON(value.Output),
		nullableJSON(value.Metadata),
		value.Model,
		nullableJSON(value.ModelParameters),
		nullableJSON(value.Usage),
		value.Level,
		value.StatusMessage,
		value.Version,
		value.Environment,
		value.StartTime,
		value.EndTime,
		value.CompletionStartTime,
		value.CreatedAt,
		value.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert observation %q: %w", value.ID, err)
	}
	return nil
}

func upsertSession(ctx context.Context, tx *sql.Tx, value Session) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO sessions (id, project_id, name, user_id, metadata, created_at, last_updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			project_id = COALESCE(NULLIF(excluded.project_id, ''), sessions.project_id),
			name = COALESCE(NULLIF(excluded.name, ''), sessions.name),
			user_id = COALESCE(NULLIF(excluded.user_id, ''), sessions.user_id),
			metadata = COALESCE(excluded.metadata, sessions.metadata),
			created_at = COALESCE(NULLIF(sessions.created_at, ''), excluded.created_at),
			last_updated_at = COALESCE(NULLIF(excluded.last_updated_at, ''), sessions.last_updated_at)`,
		value.ID,
		value.ProjectID,
		value.Name,
		value.UserID,
		nullableJSON(value.Metadata),
		value.CreatedAt,
		value.LastUpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert session %q: %w", value.ID, err)
	}
	return nil
}

func upsertScore(ctx context.Context, tx *sql.Tx, value Score) error {
	var numeric any
	if value.DataType != "STRING" && value.StringValue == "" {
		numeric = value.Value
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO scores (
			id, trace_id, observation_id, name, value, string_value, data_type, comment, source, timestamp, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			trace_id = COALESCE(NULLIF(excluded.trace_id, ''), scores.trace_id),
			observation_id = COALESCE(NULLIF(excluded.observation_id, ''), scores.observation_id),
			name = COALESCE(NULLIF(excluded.name, ''), scores.name),
			value = COALESCE(excluded.value, scores.value),
			string_value = COALESCE(NULLIF(excluded.string_value, ''), scores.string_value),
			data_type = COALESCE(NULLIF(excluded.data_type, ''), scores.data_type),
			comment = COALESCE(NULLIF(excluded.comment, ''), scores.comment),
			source = COALESCE(NULLIF(excluded.source, ''), scores.source),
			timestamp = COALESCE(NULLIF(excluded.timestamp, ''), scores.timestamp),
			created_at = COALESCE(NULLIF(scores.created_at, ''), excluded.created_at)`,
		value.ID,
		value.TraceID,
		value.ObservationID,
		value.Name,
		numeric,
		value.StringValue,
		value.DataType,
		value.Comment,
		value.Source,
		value.Timestamp,
		value.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert score %q: %w", value.ID, err)
	}
	return nil
}

func nullableJSON(raw json.RawMessage) any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return string(raw)
}

func nullableStrings(values []string) any {
	if len(values) == 0 {
		return nil
	}
	b, err := json.Marshal(values)
	if err != nil {
		return nil
	}
	return string(b)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// A tiny scanner helper keeps nullable JSON and strings consistent across the
// list methods and makes the behavior of SQLite NULL values explicit.
type nullableString struct {
	String string
	Valid  bool
}

func (n *nullableString) Scan(value any) error {
	if value == nil {
		n.String, n.Valid = "", false
		return nil
	}
	switch value := value.(type) {
	case string:
		n.String, n.Valid = value, true
		return nil
	case []byte:
		n.String, n.Valid = string(value), true
		return nil
	default:
		n.String, n.Valid = fmt.Sprint(value), true
		return nil
	}
}

func (n nullableString) Value() (driver.Value, error) {
	if !n.Valid {
		return nil, nil
	}
	return n.String, nil
}

func rawJSON(value nullableString) json.RawMessage {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return nil
	}
	if json.Valid([]byte(value.String)) {
		return json.RawMessage(value.String)
	}
	b, _ := json.Marshal(value.String)
	return b
}

func stringValue(value nullableString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func scanTimestamp(value nullableString) string { return stringValue(value) }

func parseTimestamp(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return parsed, true
	}
	parsed, err = time.Parse("2006-01-02", value)
	return parsed, err == nil
}
