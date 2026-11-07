package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// seedConversation stores one thread the way the sender exports it: an HTTP
// request holding a turn, a side call, a failed background job and a wake turn.
func seedConversation(t *testing.T, st *Store) {
	t.Helper()
	const system = `{"role":"system","content":"You are the agent"}`
	firstInput := `{"messages":[` + system + `,{"role":"user","content":[{"type":"text","text":"Draft a listing"}]}]}`
	secondInput := `{"messages":[` + system + `,{"role":"user","content":[{"type":"text","text":"Draft a listing"}]},
		{"role":"assistant","content":[]},
		{"role":"user","content":[{"type":"toolresult","call":"call-1","name":"spawn_jobs","content":[{"type":"json","value":{"jobs":[{"job_id":"job-7"}]}}]}]}]}`
	span := func(id, trace, parent, name, typ, start, end string) Observation {
		return Observation{ID: id, TraceID: trace, ParentObservationID: parent, Name: name, Type: typ, Level: "DEFAULT", StartTime: start, EndTime: end}
	}
	generation := func(id, trace, parent, input, output, start, end string) Observation {
		value := span(id, trace, parent, "openai.responses.stream", "GENERATION", start, end)
		value.Model, value.Input, value.Output = "gpt", json.RawMessage(input), json.RawMessage(output)
		value.Usage = json.RawMessage(`{"input":100,"cached_input":40,"output":10,"total":110}`)
		return value
	}
	nested := func(id, trace, parent, start, end string) Observation {
		value := span(id, trace, parent, "chat_streaming", "GENERATION", start, end)
		value.Model, value.Usage = "gpt", json.RawMessage(`{"input":100,"cached_input":40,"output":10}`)
		return value
	}
	at := func(clock string) string { return "2026-11-07T01:" + clock + "Z" }
	observations := []Observation{
		span("http", "t-turn", "", "POST /threads/{thread_id}/chat", "SPAN", at("00:00"), at("00:30")),
		span("turn", "t-turn", "http", "assistant.turn", "SPAN", at("00:01"), at("00:30")),
		generation("g1", "t-turn", "turn", firstInput, `{"text":"","tool_calls":[{"id":"call-1","name":"spawn_jobs","args":{"goal":"x"}}],"final":{"finish_reason":"tool_calls"}}`, at("00:02"), at("00:02.005")),
		nested("g1c", "t-turn", "g1", at("00:02.005"), at("00:10")),
		generation("g2", "t-turn", "turn", secondInput, `{"text":"Started the job.","tool_calls":[],"final":{"finish_reason":"stop"}}`, at("00:11"), at("00:11.005")),
		nested("g2c", "t-turn", "g2", at("00:11.005"), at("00:20")),
		generation("title", "t-turn", "turn", `{"messages":[{"role":"system","content":"Generate a title"},{"role":"user","content":"x"}]}`, `{"text":"A title","final":{"finish_reason":"stop"}}`, at("00:21"), at("00:22")),
		span("job", "t-job", "", "agent_job.run", "SPAN", at("00:15"), at("02:00")),
		generation("j1", "t-job", "job", firstInput, `{"text":"","tool_calls":[{"id":"call-9","name":"load_skill","args":{}}],"final":{"finish_reason":"tool_calls"}}`, at("00:16"), at("00:17")),
		{ID: "tool", TraceID: "t-job", ParentObservationID: "job", Name: "assistant.tool", Type: "SPAN", Level: "DEFAULT", StartTime: at("00:18"), EndTime: at("00:19"), Metadata: json.RawMessage(`{"tool":"load_skill"}`)},
		span("wake", "t-wake", "", "assistant.wake_turn", "SPAN", at("03:00"), at("03:30")),
		span("poll", "t-poll", "", "PUT /threads/{thread_id}/runner", "SPAN", at("00:01"), at("00:02")),
	}
	observations[0].Metadata = json.RawMessage(`{"http.route":"/threads/{thread_id}/chat"}`)
	observations[7].Level, observations[7].StatusMessage = "ERROR", "write failed"
	records := []IngestRecord{}
	for _, trace := range []Trace{
		{ID: "t-turn", Name: "POST /threads/{thread_id}/chat", SessionID: "thread:1", Timestamp: at("00:00")},
		{ID: "t-job", Name: "agent_job.run", SessionID: "thread:1", Timestamp: at("00:15"), Metadata: json.RawMessage(`{"job_id":"job-7","agent_id":"listing"}`)},
		{ID: "t-wake", Name: "assistant.wake_turn", SessionID: "thread:1", Timestamp: at("03:00")},
		{ID: "t-poll", Name: "PUT /threads/{thread_id}/runner", SessionID: "thread:1", Timestamp: at("00:01")},
	} {
		value := trace
		records = append(records, IngestRecord{Type: RecordTrace, Trace: &value})
	}
	for position := range observations {
		records = append(records, IngestRecord{Type: RecordObservation, Observation: &observations[position]})
	}
	if _, err := st.Ingest(context.Background(), records); err != nil {
		t.Fatal(err)
	}
}

func openSeeded(t *testing.T) *Store {
	t.Helper()
	st, err := New(filepath.Join(t.TempDir(), "story.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	seedConversation(t, st)
	return st
}

func TestFoldRequiresEvidenceOfOneCall(t *testing.T) {
	usage := json.RawMessage(`{"input":5,"output":6}`)
	pair := func(child Observation) []Observation {
		child.ID, child.ParentObservationID, child.Type = "c", "p", "GENERATION"
		return []Observation{{ID: "p", Type: "GENERATION", Model: "m", Usage: usage}, child}
	}
	cases := []struct {
		name   string
		obs    []Observation
		merged bool
	}{
		{"same model and tokens", pair(Observation{Model: "m", Usage: usage}), true},
		{"other model", pair(Observation{Model: "n", Usage: usage}), false},
		{"other tokens", pair(Observation{Model: "m", Usage: json.RawMessage(`{"input":5,"output":7}`)}), false},
		{"unknown tokens", pair(Observation{Model: "m"}), false},
		{"child has its own payload", pair(Observation{Model: "m", Usage: usage, Input: json.RawMessage(`"x"`)}), false},
		{"second child means separate work", append(pair(Observation{Model: "m", Usage: usage}), Observation{ID: "d", ParentObservationID: "p", Type: "SPAN"}), false},
	}
	for _, current := range cases {
		annotateObservations(current.obs)
		if merged := current.obs[1].MergedInto == "p"; merged != current.merged {
			t.Errorf("%s: merged = %v, want %v", current.name, merged, current.merged)
		}
	}
}

func TestFoldedCallKeepsChildFailure(t *testing.T) {
	usage := json.RawMessage(`{"input":5,"output":6}`)
	observations := []Observation{
		{ID: "p", Type: "GENERATION", Model: "m", Usage: usage, Level: "DEFAULT"},
		{ID: "c", ParentObservationID: "p", Type: "GENERATION", Model: "m", Usage: usage, Level: "ERROR", StatusMessage: "boom"},
	}
	annotateObservations(observations)
	if observations[0].Level != "ERROR" || observations[0].StatusMessage != "boom" {
		t.Errorf("parent = %s %q", observations[0].Level, observations[0].StatusMessage)
	}
}

func TestSessionStoryReadsAsOneConversation(t *testing.T) {
	st := openSeeded(t)
	story, err := st.GetSessionStory(context.Background(), "thread:1", true, TurnOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var shape []string
	for _, item := range story.Items {
		if item.Turn != nil {
			shape = append(shape, item.Turn.Kind)
		} else {
			shape = append(shape, item.Type)
		}
	}
	if got := strings.Join(shape, " "); got != "turn activity job wake_turn" {
		t.Fatalf("items = %s", got)
	}
	turn := story.Items[0].Turn
	if turn.UserMessage == nil || turn.UserMessage.Text != "Draft a listing" {
		t.Errorf("user message = %+v", turn.UserMessage)
	}
	// The title generator ran last, and its text is not the assistant's reply.
	if turn.Reply == nil || turn.Reply.Text != "Started the job." {
		t.Errorf("reply = %+v", turn.Reply)
	}
	if turn.ModelCalls != 3 || len(turn.Steps) != 3 || !turn.Steps[2].Side {
		t.Errorf("steps = %+v", turn.Steps)
	}
	if first := turn.Steps[0]; first.DurationMs != 8000 || *first.Usage.Input != 100 {
		t.Errorf("folded call duration %v usage %v, want the stream's 8s counted once", first.DurationMs, *first.Usage.Input)
	}
	if call := turn.Steps[0].ToolCalls[0]; call.Name != "spawn_jobs" || !strings.Contains(call.Result, "job-7") {
		t.Errorf("tool call was not paired with the result in the next call's input: %+v", call)
	}
	job := story.Items[2].Turn
	if job.SpawnedBy == nil || job.SpawnedBy.TraceID != "t-turn" || job.SpawnedBy.Tool != "spawn_jobs" {
		t.Errorf("job not linked to the tool call that named it: %+v", job.SpawnedBy)
	}
	if !job.Overlaps || job.ErrorCount != 1 || job.StatusMessage != "write failed" {
		t.Errorf("job = overlaps %v errors %d message %q", job.Overlaps, job.ErrorCount, job.StatusMessage)
	}
	if len(job.Steps) != 1 || job.Steps[0].ToolCalls[0].SpanID != "tool" || job.Steps[0].ToolCalls[0].DurationMs != 1000 {
		t.Errorf("tool span should fold into the model's tool call: %+v", job.Steps)
	}
	if story.Totals.Turns != 3 || story.Totals.ModelCalls != 4 || *story.Totals.Usage.Input != 400 {
		t.Errorf("totals = %+v", story.Totals)
	}
	if story.Title != "Draft a listing" {
		t.Errorf("title = %q", story.Title)
	}
}

func TestToolResultFromALaterTurnIsPaired(t *testing.T) {
	turns := []*Turn{
		{Steps: []TurnStep{{ToolCalls: []ToolCall{{ID: "wait", Name: "wait_for_jobs"}, {ID: "none", Name: "report_result"}}}}},
		{results: map[string]toolResult{"wait": {Text: "ends now", Error: ""}}},
	}
	pairAcrossTurns(turns, TurnOptions{})
	calls := turns[0].Steps[0].ToolCalls
	if calls[0].Result != "ends now" || calls[1].Result != "" || turns[0].ToolCallCount != 2 {
		t.Errorf("calls = %+v, count %d", calls, turns[0].ToolCallCount)
	}
}

func TestStoryBoundsAndFiltersBeforeClipping(t *testing.T) {
	st := openSeeded(t)
	story, err := st.GetSessionStory(context.Background(), "thread:1", true, TurnOptions{
		MessageChars: 5,
		Filter:       func(text string) string { return strings.ReplaceAll(text, "Draft", "XX") },
	})
	if err != nil {
		t.Fatal(err)
	}
	message := story.Items[0].Turn.UserMessage
	if message.Text != "XX a " || !message.Truncated() || message.TotalChars != 12 {
		t.Errorf("message = %+v", message)
	}
	clipped, err := st.GetSessionStory(context.Background(), "thread:1", true, TurnOptions{DetailChars: 3})
	if err != nil {
		t.Fatal(err)
	}
	if got := clipped.Items[0].Turn.Steps[0].ToolCalls[0].Args; got != `{"g… [+9 chars]` {
		t.Errorf("clipped args must state the cut: %q", got)
	}
}

func TestTraceDetailCarriesTurnAndAnnotations(t *testing.T) {
	st := openSeeded(t)
	trace, err := st.GetTrace(context.Background(), "t-turn")
	if err != nil {
		t.Fatal(err)
	}
	if trace.Turn == nil || trace.Turn.Kind != RoleTurn {
		t.Fatalf("turn = %+v", trace.Turn)
	}
	roles := map[string]string{}
	for _, observation := range trace.Observations {
		roles[observation.ID] = observation.Role + "/" + observation.MergedInto
	}
	if roles["g1c"] != "model_call/g1" || roles["http"] != "request/" || roles["turn"] != "turn/" {
		t.Errorf("roles = %v", roles)
	}
	plain, err := st.GetTrace(context.Background(), "t-poll")
	if err != nil || plain.Turn != nil {
		t.Errorf("plain request should have no turn: %+v, %v", plain, err)
	}
}

func TestStatusFilter(t *testing.T) {
	st := openSeeded(t)
	cases := map[string]string{StatusError: "t-job", StatusFinished: "t-poll t-turn t-wake"}
	for status, want := range cases {
		page, err := st.ListTraces(context.Background(), TraceFilter{Status: status})
		if err != nil {
			t.Fatal(err)
		}
		ids := []string{}
		for _, trace := range page.Data {
			ids = append(ids, trace.ID)
		}
		got := strings.Join(sortedStrings(ids), " ")
		if got != want {
			t.Errorf("status %s = %s, want %s", status, got, want)
		}
	}
}

func sortedStrings(values []string) []string {
	keys := map[string]bool{}
	for _, value := range values {
		keys[value] = true
	}
	return sortedKeys(keys)
}

func TestSlowSummaryCountsOneCallOnce(t *testing.T) {
	st := openSeeded(t)
	summary, err := st.SummarizeSlowSpans(context.Background(), SpanFilter{}, RoleModelCall, 5, 2)
	if err != nil {
		t.Fatal(err)
	}
	if summary.ScannedSpans != 12 || len(summary.Slowest) != 2 || summary.Slowest[0].DurationMs != 9000 {
		t.Errorf("summary = %+v", summary)
	}
	for _, group := range summary.Groups {
		if group.Label == "gpt" && (group.Count != 4 || group.MaxMs != 9000 || group.P50Ms != 1000) {
			t.Errorf("group = %+v, want 4 calls: nested duplicates count once", group)
		}
	}
}

func TestListSpanErrors(t *testing.T) {
	st := openSeeded(t)
	page, err := st.ListSpanErrors(context.Background(), SpanFilter{SessionID: "thread:1", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || page.Data[0].ObservationID != "job" || page.Data[0].DurationMs != 105000 || page.Data[0].TraceName != "agent_job.run" {
		t.Errorf("errors = %+v", page)
	}
}

func TestTimestampRepairUsesEarliestSpan(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repair.db")
	st, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := st.Ingest(ctx, []IngestRecord{
		{Type: RecordTrace, Trace: &Trace{ID: "a", SessionID: "s", Timestamp: "2026-01-01T00:05:00Z"}},
		{Type: RecordObservation, Observation: &Observation{ID: "o", TraceID: "a", StartTime: "2026-01-01T00:01:00Z"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().ExecContext(ctx, "PRAGMA user_version = 1"); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	var timestamp string
	if err := reopened.DB().QueryRowContext(ctx, "SELECT timestamp FROM traces WHERE id = 'a'").Scan(&timestamp); err != nil {
		t.Fatal(err)
	}
	if timestamp != "2026-01-01T00:01:00Z" {
		t.Errorf("timestamp = %s", timestamp)
	}
}
