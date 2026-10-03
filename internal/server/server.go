// Package server implements Duckscope's small JSON API and embedded SPA
// handler. It intentionally uses net/http so the binary has no web framework
// runtime to configure.
package server

import (
	"compress/gzip"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/open-ai-sdk/duckscope/internal/store"
	"github.com/open-ai-sdk/duckscope/web"
)

const (
	Version        = "0.1.0"
	maxRequestSize = 32 << 20
)

type Server struct {
	Store *store.Store
}

func New(st *store.Store) *Server { return &Server{Store: st} }

// Handler returns the complete HTTP handler, including the embedded frontend.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.health)
	mux.HandleFunc("/api/ingest", s.ingest)
	mux.HandleFunc("/api/public/otel/v1/traces", s.otlpTraces)
	mux.HandleFunc("/api/traces", s.traces)
	mux.HandleFunc("/api/traces/", s.traces)
	mux.HandleFunc("/api/observations", s.observations)
	mux.HandleFunc("/api/sessions", s.sessions)
	mux.HandleFunc("/api/scores", s.scores)
	assets, _ := fs.Sub(web.Dist, "dist")
	mux.Handle("/", spaHandler{assets: http.FS(assets)})
	return cors(mux)
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type spaHandler struct{ assets http.FileSystem }

func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	if file, err := h.assets.Open(path); err == nil {
		_ = file.Close()
		http.FileServer(h.assets).ServeHTTP(w, r)
		return
	}
	// Vite's client router needs index.html for deep links.
	r2 := r.Clone(r.Context())
	r2.URL.Path = "/index.html"
	http.FileServer(h.assets).ServeHTTP(w, r2)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if s.Store == nil || s.Store.DB() == nil || s.Store.DB().PingContext(r.Context()) != nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": Version, "storage": "duckdb"})
}

func (s *Server) traces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/traces")
	if path != "" && path != "/" {
		id, err := url.PathUnescape(strings.TrimPrefix(path, "/"))
		if err != nil || id == "" {
			writeError(w, http.StatusBadRequest, "invalid trace id")
			return
		}
		trace, err := s.Store.GetTrace(r.Context(), id)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "trace not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, trace)
		return
	}
	q := r.URL.Query()
	result, err := s.Store.ListTraces(r.Context(), store.TraceFilter{Project: first(q.Get("project"), q.Get("projectId")), Name: q.Get("name"), From: q.Get("from"), To: q.Get("to"), Page: intQuery(q.Get("page")), Limit: intQuery(q.Get("limit"))})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) observations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	q := r.URL.Query()
	result, err := s.Store.ListObservations(r.Context(), store.ObservationFilter{TraceID: first(q.Get("traceId"), q.Get("trace_id")), Project: first(q.Get("project"), q.Get("projectId")), Name: q.Get("name"), Type: q.Get("type"), From: q.Get("from"), To: q.Get("to"), Page: intQuery(q.Get("page")), Limit: intQuery(q.Get("limit"))})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) sessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	q := r.URL.Query()
	result, err := s.Store.ListSessions(r.Context(), store.SessionFilter{Project: first(q.Get("project"), q.Get("projectId")), Name: q.Get("name"), UserID: first(q.Get("userId"), q.Get("user_id")), From: q.Get("from"), To: q.Get("to"), Page: intQuery(q.Get("page")), Limit: intQuery(q.Get("limit"))})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) scores(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	q := r.URL.Query()
	result, err := s.Store.ListScores(r.Context(), store.ScoreFilter{TraceID: first(q.Get("traceId"), q.Get("trace_id")), ObservationID: first(q.Get("observationId"), q.Get("observation_id")), Name: q.Get("name"), Source: q.Get("source"), Page: intQuery(q.Get("page")), Limit: intQuery(q.Get("limit"))})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) ingest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	body, err := readBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	records, err := decodeRecords(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := s.Store.Ingest(r.Context(), records)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (s *Server) otlpTraces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	body, err := readBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	records, err := decodeOTLP(body, r.Header.Get("Content-Type"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := s.Store.Ingest(r.Context(), records)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// OTLP exporters expect a 200 response with an empty success object.
	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, http.StatusOK, map[string]any{"partialSuccess": map[string]any{}, "accepted": result.Accepted})
}

func readBody(r *http.Request) ([]byte, error) {
	reader := io.Reader(io.LimitReader(r.Body, maxRequestSize+1))
	if strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gzip.NewReader(reader)
		if err != nil {
			return nil, fmt.Errorf("decode gzip body: %w", err)
		}
		defer func() { _ = gz.Close() }()
		reader = io.LimitReader(gz, maxRequestSize+1)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}
	if len(body) > maxRequestSize {
		return nil, errors.New("request body exceeds 32 MiB")
	}
	return body, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message, "message": message})
}
func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}
func intQuery(value string) int { result, _ := strconv.Atoi(value); return result }
func first(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

type object map[string]json.RawMessage

func decodeObject(raw []byte) (object, error) {
	var value object
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if value == nil {
		return nil, errors.New("request JSON must be an object")
	}
	return value, nil
}

func field(value object, names ...string) json.RawMessage {
	for _, name := range names {
		if raw, ok := value[name]; ok {
			return raw
		}
	}
	return nil
}

func textField(value object, names ...string) string {
	raw := field(value, names...)
	if len(raw) == 0 {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var number json.Number
	if json.Unmarshal(raw, &number) == nil {
		return number.String()
	}
	return ""
}

func boolField(value object, names ...string) bool {
	var result bool
	_ = json.Unmarshal(field(value, names...), &result)
	return result
}
func rawField(value object, names ...string) json.RawMessage {
	raw := field(value, names...)
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

func timestampField(value object, names ...string) string {
	raw := field(value, names...)
	if len(raw) == 0 {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return canonicalTime(text)
	}
	var number json.Number
	if json.Unmarshal(raw, &number) == nil {
		integer, _ := strconv.ParseInt(number.String(), 10, 64)
		if integer > 1_000_000_000_000_000 {
			return time.Unix(0, integer).UTC().Format(time.RFC3339Nano)
		}
		if integer > 1_000_000_000 {
			return time.UnixMilli(integer).UTC().Format(time.RFC3339Nano)
		}
		return time.Unix(integer, 0).UTC().Format(time.RFC3339Nano)
	}
	return ""
}

func canonicalTime(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.UTC().Format(time.RFC3339Nano)
	}
	if parsed, err := time.Parse("2006-01-02", value); err == nil {
		return parsed.UTC().Format(time.RFC3339Nano)
	}
	return value
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func newID(prefix string) string {
	var bytes [12]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return prefix + "-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return prefix + "-" + base64.RawURLEncoding.EncodeToString(bytes[:])
}

func traceFromObject(value object, fallbackID string) store.Trace {
	id := first(textField(value, "id", "traceId", "trace_id"), fallbackID, newID("trace"))
	project := textField(value, "projectId", "project_id", "project", "serviceName")
	return store.Trace{ID: id, ProjectID: project, Project: project, Name: textField(value, "name", "traceName"), UserID: textField(value, "userId", "user_id"), SessionID: textField(value, "sessionId", "session_id"), Environment: textField(value, "environment"), Release: textField(value, "release"), Version: textField(value, "version"), Input: rawField(value, "input"), Output: rawField(value, "output"), Metadata: rawField(value, "metadata"), Tags: stringSlice(rawField(value, "tags")), Public: boolField(value, "public"), Timestamp: first(timestampField(value, "timestamp", "startTime", "start_time"), now()), StartTime: timestampField(value, "startTime", "start_time", "timestamp"), EndTime: timestampField(value, "endTime", "end_time"), CreatedAt: first(timestampField(value, "createdAt", "created_at"), now()), UpdatedAt: timestampField(value, "updatedAt", "updated_at")}
}

func observationFromObject(value object, fallbackID, fallbackTrace string) store.Observation {
	typ := strings.ToUpper(textField(value, "type", "observationType", "observation_type"))
	if typ == "" {
		typ = "SPAN"
	}
	switch typ {
	case "GENERATION-CREATE", "GENERATION":
		typ = "GENERATION"
	case "SPAN-CREATE":
		typ = "SPAN"
	case "EVENT-CREATE", "EVENT":
		typ = "EVENT"
	case "AGENT":
		typ = "AGENT"
	}
	return store.Observation{ID: first(textField(value, "id", "observationId", "observation_id"), fallbackID, newID("obs")), TraceID: first(textField(value, "traceId", "trace_id"), fallbackTrace), ProjectID: textField(value, "projectId", "project_id", "project"), ParentObservationID: textField(value, "parentObservationId", "parent_observation_id", "parentId"), Type: typ, Name: textField(value, "name", "observationName"), Input: rawField(value, "input"), Output: rawField(value, "output"), Metadata: rawField(value, "metadata"), Model: textField(value, "model", "modelName"), ModelParameters: rawField(value, "modelParameters", "model_parameters"), Usage: rawField(value, "usage", "usageDetails", "usage_details"), Level: first(textField(value, "level", "status"), "DEFAULT"), StatusMessage: textField(value, "statusMessage", "status_message"), Version: textField(value, "version"), Environment: textField(value, "environment"), StartTime: timestampField(value, "startTime", "start_time", "timestamp"), EndTime: timestampField(value, "endTime", "end_time"), CompletionStartTime: timestampField(value, "completionStartTime", "completion_start_time"), CreatedAt: first(timestampField(value, "createdAt", "created_at"), now()), UpdatedAt: timestampField(value, "updatedAt", "updated_at")}
}

func scoreFromObject(value object, fallbackID string) store.Score {
	var numeric float64
	_ = json.Unmarshal(field(value, "value"), &numeric)
	return store.Score{ID: first(textField(value, "id", "scoreId", "score_id"), fallbackID, newID("score")), TraceID: textField(value, "traceId", "trace_id"), ObservationID: textField(value, "observationId", "observation_id"), Name: textField(value, "name"), Value: numeric, StringValue: textField(value, "stringValue", "string_value"), DataType: textField(value, "dataType", "data_type"), Comment: textField(value, "comment"), Source: textField(value, "source"), Timestamp: timestampField(value, "timestamp"), CreatedAt: first(timestampField(value, "createdAt", "created_at"), now())}
}

func sessionFromObject(value object, fallbackID string) store.Session {
	return store.Session{ID: first(textField(value, "id", "sessionId", "session_id"), fallbackID, newID("session")), ProjectID: textField(value, "projectId", "project_id", "project"), Name: textField(value, "name"), UserID: textField(value, "userId", "user_id"), Metadata: rawField(value, "metadata"), CreatedAt: first(timestampField(value, "createdAt", "created_at"), now()), LastUpdatedAt: timestampField(value, "lastUpdatedAt", "last_updated_at", "updatedAt", "updated_at")}
}

func stringSlice(raw json.RawMessage) []string {
	var result []string
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &result)
	}
	return result
}

func decodeRecords(body []byte) ([]store.IngestRecord, error) {
	value, err := decodeObject(body)
	if err != nil {
		return nil, err
	}
	var records []store.IngestRecord
	if batch := field(value, "batch"); len(batch) > 0 {
		var items []json.RawMessage
		if err := json.Unmarshal(batch, &items); err != nil {
			return nil, errors.New("batch must be an array")
		}
		for _, item := range items {
			record, err := decodeRecord(item, "")
			if err != nil {
				return nil, err
			}
			if record.Type != "" {
				records = append(records, record)
			}
		}
		return records, nil
	}
	if traceRaw := field(value, "trace"); len(traceRaw) > 0 {
		traceObj, err := decodeObject(traceRaw)
		if err != nil {
			return nil, err
		}
		trace := traceFromObject(traceObj, "")
		records = append(records, store.IngestRecord{Type: store.RecordTrace, Trace: &trace})
		var observations []json.RawMessage
		_ = json.Unmarshal(field(value, "observations"), &observations)
		for _, item := range observations {
			obj, err := decodeObject(item)
			if err != nil {
				return nil, err
			}
			observation := observationFromObject(obj, "", trace.ID)
			records = append(records, store.IngestRecord{Type: store.RecordObservation, Observation: &observation})
		}
		return records, nil
	}
	record, err := decodeRecord(body, "")
	if err != nil {
		return nil, err
	}
	if record.Type == "" {
		return nil, errors.New("request does not contain a trace, observation, or batch")
	}
	return []store.IngestRecord{record}, nil
}

func decodeRecord(raw []byte, fallbackID string) (store.IngestRecord, error) {
	value, err := decodeObject(raw)
	if err != nil {
		return store.IngestRecord{}, err
	}
	typ := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(textField(value, "type", "eventType"), "_", "-"), " ", "-"))
	body := value
	if nested := field(value, "body", "data"); len(nested) > 0 {
		if nestedValue, decodeErr := decodeObject(nested); decodeErr == nil {
			body = nestedValue
		}
	}
	switch {
	case strings.HasPrefix(typ, "trace"):
		trace := traceFromObject(body, first(textField(value, "id"), fallbackID))
		return store.IngestRecord{Type: store.RecordTrace, Trace: &trace}, nil
	case strings.HasPrefix(typ, "span") || strings.HasPrefix(typ, "generation") || strings.HasPrefix(typ, "event") || strings.HasPrefix(typ, "observation") || strings.HasPrefix(typ, "agent"):
		observation := observationFromObject(body, first(textField(value, "id"), fallbackID), "")
		return store.IngestRecord{Type: store.RecordObservation, Observation: &observation}, nil
	case strings.HasPrefix(typ, "score"):
		score := scoreFromObject(body, first(textField(value, "id"), fallbackID))
		return store.IngestRecord{Type: store.RecordScore, Score: &score}, nil
	case strings.HasPrefix(typ, "session"):
		session := sessionFromObject(body, first(textField(value, "id"), fallbackID))
		return store.IngestRecord{Type: store.RecordSession, Session: &session}, nil
	}
	if textField(body, "traceId", "trace_id") != "" {
		observation := observationFromObject(body, fallbackID, "")
		return store.IngestRecord{Type: store.RecordObservation, Observation: &observation}, nil
	}
	if textField(body, "scoreId", "score_id") != "" || (textField(body, "name") != "" && field(body, "value") != nil && field(body, "traceId") != nil) {
		score := scoreFromObject(body, fallbackID)
		return store.IngestRecord{Type: store.RecordScore, Score: &score}, nil
	}
	if textField(body, "id", "traceId", "trace_id") != "" {
		trace := traceFromObject(body, fallbackID)
		return store.IngestRecord{Type: store.RecordTrace, Trace: &trace}, nil
	}
	return store.IngestRecord{}, nil
}
