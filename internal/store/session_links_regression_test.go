package store

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestSpawnedJobsRequireCompleteRecordedIdentifiers(t *testing.T) {
	for _, test := range []struct {
		name   string
		result string
		linked bool
	}{
		{"exact text", "job-1", true},
		{"text with punctuation", "Started job-1. Waiting for completion.", true},
		{"nested structured result", `{"jobs":[{"job_id":"job-1"}]}`, true},
		{"escaped JSON identifier", `{"jobs":[{"job_id":"job\u002d1"}]}`, true},
		{"JSON text block", `[{"type":"text","text":"Started job-1 successfully"}]`, true},
		{"numeric suffix", "Started job-10", false},
		{"JSON numeric suffix", `{"jobs":[{"job_id":"job-10"}]}`, false},
		{"identifier prefix", "Started other-job-1", false},
		{"identifier suffix", "Started job-1-extra", false},
		{"underscore suffix", "Started job-1_extra", false},
		{"unicode suffix", "Started job-1é", false},
		{"no recorded result", "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			parent := &Turn{TraceID: "parent", Steps: []TurnStep{{ObservationID: "generation", ToolCalls: []ToolCall{{Name: "spawn_jobs", Result: "job-1", rawResult: test.result}}}}}
			job := &Turn{TraceID: "job-trace", JobID: "job-1"}
			longer := &Turn{TraceID: "longer-job-trace", JobID: "job-10"}
			linkSpawnedJobs([]*Turn{parent, job, longer}, map[string]*Turn{"job-1": job, "job-10": longer})
			if (job.SpawnedBy != nil) != test.linked {
				t.Fatalf("spawn link = %+v, want linked %v for %q", job.SpawnedBy, test.linked, test.result)
			}
			if job.SpawnedBy != nil && (job.SpawnedBy.TraceID != "parent" || job.SpawnedBy.ObservationID != "generation" || job.SpawnedBy.Tool != "spawn_jobs") {
				t.Errorf("spawn evidence = %+v", job.SpawnedBy)
			}
			if (test.name == "numeric suffix" || test.name == "JSON numeric suffix") && longer.SpawnedBy == nil {
				t.Error("the fully named job-10 must still link")
			}
		})
	}
}

func TestSpawnedJobScalarResultsThroughStoryHelpers(t *testing.T) {
	for _, test := range []struct {
		name   string
		jobID  string
		result string
		linked bool
	}{
		{"numeric identifier", "123", "123", true},
		{"numeric prefix collision", "123", "1234", false},
		{"large numeric identifier", "9007199254740993", "9007199254740993", true},
		{"structured numeric identifier", "123", `{"job_id":123}`, true},
		{"structured numeric prefix collision", "123", `{"job_id":1234}`, false},
		{"boolean identifier", "true", "true", true},
		{"null text identifier", "null", "null", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			trace := &Trace{ID: "parent"}
			observations := []Observation{
				{ID: "spawn", Role: RoleModelCall, Output: json.RawMessage(`{"tool_calls":[{"id":"spawn-call","name":"spawn_jobs","args":{}}]}`)},
				{ID: "result", Role: RoleModelCall, Input: json.RawMessage(fmt.Sprintf(`{"messages":[{"role":"tool","tool_call_id":"spawn-call","content":%q}]}`, test.result))},
			}
			parent := buildTurn(trace, observations, TurnOptions{})
			if parent == nil {
				t.Fatal("model trace did not produce a turn")
			}
			if got := parent.Steps[0].ToolCalls[0].rawResult; got != test.result {
				t.Fatalf("story parsed result = %q, want %q", got, test.result)
			}
			job := &Turn{TraceID: "job", JobID: test.jobID}
			linkSpawnedJobs([]*Turn{parent, job}, map[string]*Turn{test.jobID: job})
			if (job.SpawnedBy != nil) != test.linked {
				t.Fatalf("job %q spawn link = %+v, want linked %v for result %q", test.jobID, job.SpawnedBy, test.linked, test.result)
			}
			if job.SpawnedBy != nil && (job.SpawnedBy.TraceID != "parent" || job.SpawnedBy.ObservationID != "spawn") {
				t.Errorf("spawn evidence = %+v", job.SpawnedBy)
			}
		})
	}
}

func TestSpawnedJobLinksUseTheLaterExactResultInsteadOfAnEarlierPrefix(t *testing.T) {
	first := &Turn{TraceID: "first", Steps: []TurnStep{{ObservationID: "first-call", ToolCalls: []ToolCall{{Name: "spawn_jobs", rawResult: `{"job_id":"job-10"}`}}}}}
	second := &Turn{TraceID: "second", Steps: []TurnStep{{ObservationID: "second-call", ToolCalls: []ToolCall{{Name: "spawn_jobs", rawResult: `{"job_id":"job-1"}`}}}}}
	short := &Turn{TraceID: "short", JobID: "job-1"}
	long := &Turn{TraceID: "long", JobID: "job-10"}
	linkSpawnedJobs([]*Turn{first, second, short, long}, map[string]*Turn{"job-1": short, "job-10": long})
	if short.SpawnedBy == nil || short.SpawnedBy.TraceID != "second" || short.SpawnedBy.ObservationID != "second-call" {
		t.Errorf("job-1 link = %+v, want the later exact evidence", short.SpawnedBy)
	}
	if long.SpawnedBy == nil || long.SpawnedBy.TraceID != "first" {
		t.Errorf("job-10 link = %+v, want the first exact evidence", long.SpawnedBy)
	}
}

func TestSpawnedJobEvidenceDoesNotReplaceExistingLinksOrLinkSelf(t *testing.T) {
	existing := &TurnLink{TraceID: "original", ObservationID: "original-call", Tool: "spawn_jobs"}
	parent := &Turn{TraceID: "parent", JobID: "self", Steps: []TurnStep{{ToolCalls: []ToolCall{{Name: "spawn_jobs", rawResult: "self job-1"}}}}}
	job := &Turn{TraceID: "job-trace", JobID: "job-1", SpawnedBy: existing}
	linkSpawnedJobs([]*Turn{parent, job}, map[string]*Turn{"self": parent, "job-1": job})
	if parent.SpawnedBy != nil || job.SpawnedBy != existing {
		t.Fatalf("recorded evidence changed: self = %+v, existing = %+v", parent.SpawnedBy, job.SpawnedBy)
	}
}

func TestSessionUnlinkedWindowsExposeAllEligibleTraces(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "session-window.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	base := time.Date(2026, 11, 7, 1, 0, 0, 0, time.UTC)
	stamp := func(at time.Time) string { return at.Format(time.RFC3339Nano) }
	session := Session{ID: "thread:window"}
	records := []IngestRecord{{Type: RecordSession, Session: &session}}
	add := func(id, sessionID, name, kind string, start, end time.Time) {
		trace := &Trace{ID: id, SessionID: sessionID, Name: name, Timestamp: stamp(start)}
		observation := &Observation{ID: id + "-span", TraceID: id, Name: name, Type: kind, Level: "DEFAULT", StartTime: stamp(start), EndTime: stamp(end)}
		records = append(records, IngestRecord{Type: RecordTrace, Trace: trace}, IngestRecord{Type: RecordObservation, Observation: observation})
	}
	add("request", session.ID, "POST /request", "SPAN", base, base.Add(30*time.Second))
	add("overlapping-request", session.ID, "POST /request", "SPAN", base.Add(5*time.Second), base.Add(20*time.Second))
	later := base.Add(24 * time.Hour)
	add("later-request", session.ID, "POST /request", "SPAN", later, later.Add(30*time.Second))
	modelWindow := later.Add(time.Hour)
	add("model-window", session.ID, "model", "GENERATION", modelWindow, modelWindow.Add(time.Minute))

	expected := make(map[string]bool)
	for index := range 505 {
		id := fmt.Sprintf("eligible-%03d", index)
		expected[id] = true
		// Identical timestamps exercise the stable ID ordering at page boundaries.
		add(id, "", "POST /background", "SPAN", base.Add(10*time.Second), base.Add(11*time.Second))
	}
	expected["later-eligible"] = true
	add("later-eligible", "", "POST /background", "SPAN", later.Add(10*time.Second), later.Add(11*time.Second))
	// Both near misses lie inside the widened SQL bounds, but outside the exact request.
	add("before-window", "", "POST /background", "SPAN", base.Add(-500*time.Millisecond), base)
	add("after-window", "", "POST /background", "SPAN", base.Add(30500*time.Millisecond), base.Add(31*time.Second))
	add("between-windows", "", "POST /background", "SPAN", base.Add(time.Hour), base.Add(time.Hour+time.Second))
	add("other-session", "thread:other", "POST /background", "SPAN", base.Add(10*time.Second), base.Add(11*time.Second))
	add("noise", "", "GET /health", "SPAN", base.Add(10*time.Second), base.Add(11*time.Second))
	add("inside-model-window", "", "POST /background", "SPAN", modelWindow.Add(10*time.Second), modelWindow.Add(11*time.Second))
	if _, err := st.Ingest(ctx, records); err != nil {
		t.Fatal(err)
	}

	detail, err := st.GetSessionDetail(ctx, session.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Unlinked) != len(expected) {
		t.Errorf("detail unlinked = %d, want %d", len(detail.Unlinked), len(expected))
	}
	seen := make(map[string]bool)
	for _, trace := range detail.Unlinked {
		if !expected[trace.ID] || seen[trace.ID] {
			t.Errorf("unexpected or duplicate detail trace %q", trace.ID)
		}
		seen[trace.ID] = true
	}
	for id := range expected {
		if !seen[id] {
			t.Errorf("detail omitted eligible trace %q", id)
		}
	}

	story, err := st.GetSessionStory(ctx, session.ID, true, TurnOptions{})
	if err != nil {
		t.Fatal(err)
	}
	seen = make(map[string]bool)
	for _, item := range story.Items {
		if item.Activity == nil {
			continue
		}
		for _, request := range item.Activity.Requests {
			if !request.Unlinked {
				continue
			}
			if !expected[request.TraceID] || seen[request.TraceID] {
				t.Errorf("unexpected or duplicate story request %q", request.TraceID)
			}
			seen[request.TraceID] = true
		}
	}
	if len(seen) != len(expected) {
		t.Errorf("story unlinked = %d, want %d", len(seen), len(expected))
	}
	for id := range expected {
		if !seen[id] {
			t.Errorf("story omitted eligible trace %q", id)
		}
	}
}

func TestSuccessfulToolSpanLevelsFoldIntoModelCalls(t *testing.T) {
	for _, level := range []string{"", "DEFAULT", "default", "OK", "ok", "ERROR", "FATAL", "WARN"} {
		t.Run(level, func(t *testing.T) {
			trace := &Trace{ID: "trace", StartTime: "2026-11-07T01:00:00Z", EndTime: "2026-11-07T01:00:04Z"}
			observations := []Observation{
				{ID: "model", Type: "GENERATION", Role: RoleModelCall, StartTime: "2026-11-07T01:00:01Z", EndTime: "2026-11-07T01:00:02Z", Output: json.RawMessage(`{"tool_calls":[{"id":"call","name":"load_skill","args":{}}]}`)},
				{ID: "tool", Type: "SPAN", Role: RoleTool, Label: "load_skill", Level: level, StatusMessage: "tool outcome", StartTime: "2026-11-07T01:00:02Z", EndTime: "2026-11-07T01:00:03Z", Duration: 1000},
			}
			turn := buildTurn(trace, observations, TurnOptions{})
			if turn == nil {
				t.Fatal("model trace did not produce a turn")
			}
			retained := level == "ERROR" || level == "FATAL" || level == "WARN"
			if retained {
				if len(turn.Steps) != 2 || turn.Steps[1].ObservationID != "tool" || turn.Steps[1].Level != level || turn.Steps[1].StatusMessage != "tool outcome" {
					t.Fatalf("diagnostic tool step was lost: %+v", turn.Steps)
				}
				if (level == "ERROR" || level == "FATAL") && turn.ErrorCount != 1 {
					t.Errorf("error count = %d, want 1", turn.ErrorCount)
				}
				return
			}
			if len(turn.Steps) != 1 || turn.ToolCallCount != 1 {
				t.Fatalf("successful execution counted twice: count %d, steps %+v", turn.ToolCallCount, turn.Steps)
			}
			call := turn.Steps[0].ToolCalls[0]
			if call.SpanID != "tool" || call.DurationMs != 1000 {
				t.Errorf("tool execution timing was not attached: %+v", call)
			}
		})
	}
}
