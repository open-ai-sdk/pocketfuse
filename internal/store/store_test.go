package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestNormalizeUsageShapes(t *testing.T) {
	cases := map[string]string{
		`{"langfuse.observation.usage_details":{"input":1,"output":2,"total":3}}`:  `{"input":1,"output":2,"total":3}`,
		`{"langfuse.observation.usage_details":"{\"input\":1}"}`:                   `{"input":1}`,
		`{"gen_ai.usage.input_tokens":1,"gen_ai.usage.cache_read.input_tokens":4}`: `{"cached_input":4,"input":1}`,
		`{"input":5}`: `{"input":5}`,
	}
	for input, want := range cases {
		if got := string(NormalizeUsage(json.RawMessage(input))); got != want {
			t.Errorf("NormalizeUsage(%s) = %s, want %s", input, got, want)
		}
	}
}

func TestMetadataCostIsUnknownWithoutCostDetails(t *testing.T) {
	if cost := metadataCost(json.RawMessage(`{"a":1}`)); cost != nil {
		t.Errorf("cost = %v, want unknown", *cost)
	}
	if cost := metadataCost(json.RawMessage(`{"cost_details":{"input":0.5,"output":0.25}}`)); cost == nil || *cost != 0.75 {
		t.Errorf("summed cost = %v", cost)
	}
	if cost := metadataCost(json.RawMessage(`{"cost_details":{"input":0.5,"total":9}}`)); cost == nil || *cost != 9 {
		t.Errorf("explicit total = %v", cost)
	}
}

func TestTraceStatusReportsOnlyWhatSpansEstablish(t *testing.T) {
	cases := []struct {
		name string
		obs  []Observation
		want string
	}{
		{"no spans", nil, ""},
		{"error wins", []Observation{{Level: "ERROR", EndTime: "x"}, {Level: "OK", EndTime: "x"}}, "error"},
		{"open span", []Observation{{Level: "DEFAULT"}}, "pending"},
		{"open event is not pending", []Observation{{Type: "EVENT"}, {EndTime: "x"}}, "completed"},
		{"explicit ok", []Observation{{Level: "OK", EndTime: "x"}}, "success"},
		{"http 200", []Observation{{EndTime: "x", Metadata: json.RawMessage(`{"status_code":200}`)}}, "success"},
		{"no outcome", []Observation{{EndTime: "x"}}, "completed"},
	}
	for _, current := range cases {
		if got := traceStatus(current.obs); got != current.want {
			t.Errorf("%s: status = %q, want %q", current.name, got, current.want)
		}
	}
}

func TestParentCycleTerminates(t *testing.T) {
	observations := []Observation{
		{ID: "a", ParentObservationID: "b", StartTime: "2026-01-01T00:00:00Z", EndTime: "2026-01-01T00:00:01Z"},
		{ID: "b", ParentObservationID: "a", StartTime: "2026-01-01T00:00:00Z", EndTime: "2026-01-01T00:00:02Z"},
	}
	var trace Trace
	summarizeTrace(&trace, observations)
	if trace.Duration != 2000 {
		t.Errorf("duration = %v", trace.Duration)
	}
}

// Rows stored before correlation existed recover their links from span metadata
// once, and explicit values are kept.
func TestBackfillRecoversCorrelationFromStoredSpans(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")
	st, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	records := []IngestRecord{}
	addTrace := func(id, metadata string) {
		records = append(records,
			IngestRecord{Type: RecordTrace, Trace: &Trace{ID: id, Name: id, Timestamp: "2026-01-01T00:00:00Z"}},
			IngestRecord{Type: RecordObservation, Observation: &Observation{ID: id + "-span", TraceID: id, Type: "SPAN", Metadata: json.RawMessage(metadata)}})
	}
	addTrace("chat", `{"http.route":"/threads/{thread_id}/chat","url.path":"/threads/9/chat","user_id":1,"session_id":77}`)
	addTrace("job", `{"thread_id":92,"job_id":"j","plan_id":"Some(p)","agent_id":"listing"}`)
	addTrace("plain", `{"target":"x"}`)
	if _, err := st.Ingest(ctx, records); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Ingest(ctx, []IngestRecord{{Type: RecordTrace, Trace: &Trace{ID: "explicit", SessionID: "kept", Timestamp: "2026-01-01T00:00:00Z"}},
		{Type: RecordObservation, Observation: &Observation{ID: "e", TraceID: "explicit", Metadata: json.RawMessage(`{"thread_id":1}`)}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().ExecContext(ctx, "PRAGMA user_version = 0"); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()

	st, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	check := func(id, wantSession, wantUser string) *Trace {
		t.Helper()
		trace, err := st.GetTrace(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if trace.SessionID != wantSession || trace.UserID != wantUser {
			t.Errorf("%s: session=%q user=%q, want %q %q", id, trace.SessionID, trace.UserID, wantSession, wantUser)
		}
		return trace
	}
	check("chat", "thread:9", "1")
	job := check("job", "thread:92", "")
	check("plain", "", "")
	check("explicit", "kept", "")
	var metadata map[string]string
	if err := json.Unmarshal(job.Metadata, &metadata); err != nil || metadata["plan_id"] != "p" || metadata["job_id"] != "j" {
		t.Errorf("job metadata = %s (%v)", job.Metadata, err)
	}
	sessions, err := st.ListSessions(ctx, SessionFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if sessions.Total < 2 {
		t.Errorf("sessions = %+v", sessions.Data)
	}
	var version int
	if err := st.DB().QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil || version != timestampVersion {
		t.Errorf("version = %d (%v)", version, err)
	}
}
