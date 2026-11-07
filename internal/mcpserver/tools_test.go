package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/open-ai-sdk/pocketfuse/internal/store"
)

const seededToken = "abcdef0123456789secret"

// seedStore holds one failed job and one long prompt with credentials in it.
func seedStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.New(filepath.Join(t.TempDir(), "mcp.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	now := time.Now().UTC()
	at := func(offset time.Duration) string { return now.Add(offset).Format(time.RFC3339Nano) }
	long := strings.Repeat("Xin chào ", 5000)
	input := fmt.Sprintf(`{"messages":[{"role":"system","content":"sys"},{"role":"user","content":"Fix the listing %s tail-marker"}],"headers":{"Authorization":"Bearer %s"}}`, long, seededToken)
	usage := json.RawMessage(`{"input":10,"output":2,"total":12}`)
	records := []store.IngestRecord{
		{Type: store.RecordTrace, Trace: &store.Trace{ID: "t1", Name: "assistant.turn", SessionID: "thread:9", Timestamp: at(-time.Minute)}},
		{Type: store.RecordObservation, Observation: &store.Observation{ID: "root", TraceID: "t1", Name: "assistant.turn", Type: "SPAN", Level: "DEFAULT", StartTime: at(-time.Minute), EndTime: at(-30 * time.Second)}},
		{Type: store.RecordObservation, Observation: &store.Observation{ID: "gen", TraceID: "t1", ParentObservationID: "root", Name: "openai.responses.stream", Type: "GENERATION", Model: "gpt", Usage: usage, Level: "DEFAULT",
			Input: json.RawMessage(input), Output: json.RawMessage(`{"text":"Done","final":{"finish_reason":"stop"}}`), StartTime: at(-50 * time.Second), EndTime: at(-49 * time.Second),
			Metadata: json.RawMessage(`{"url":"https://x.test/a?token=` + seededToken + `"}`)}},
		{Type: store.RecordObservation, Observation: &store.Observation{ID: "stream", TraceID: "t1", ParentObservationID: "gen", Name: "chat_streaming", Type: "GENERATION", Model: "gpt", Usage: usage, Level: "DEFAULT", StartTime: at(-49 * time.Second), EndTime: at(-40 * time.Second)}},
		{Type: store.RecordTrace, Trace: &store.Trace{ID: "t2", Name: "agent_job.run", SessionID: "thread:9", Timestamp: at(-20 * time.Second)}},
		{Type: store.RecordObservation, Observation: &store.Observation{ID: "job", TraceID: "t2", Name: "agent_job.run", Type: "SPAN", Level: "ERROR", StatusMessage: "write failed: Bearer " + seededToken, StartTime: at(-20 * time.Second), EndTime: at(-10 * time.Second)}},
		{Type: store.RecordTrace, Trace: &store.Trace{ID: "noise", Name: "GET /ping", Timestamp: at(-5 * time.Second)}},
		{Type: store.RecordObservation, Observation: &store.Observation{ID: "ping", TraceID: "noise", Name: "GET /ping", Type: "SPAN", Level: "DEFAULT", StartTime: at(-5 * time.Second), EndTime: at(-4 * time.Second)}},
	}
	for position := 0; position < 70; position++ {
		id := fmt.Sprintf("bulk-%02d", position)
		start := at(-time.Duration(position+2) * time.Hour)
		records = append(records,
			store.IngestRecord{Type: store.RecordTrace, Trace: &store.Trace{ID: id, Name: "bulk.work", Timestamp: start}},
			store.IngestRecord{Type: store.RecordObservation, Observation: &store.Observation{ID: id + "-span", TraceID: id, Name: "bulk.work", Type: "SPAN", Level: "DEFAULT", StartTime: start, EndTime: start}},
		)
	}
	if _, err := st.Ingest(context.Background(), records); err != nil {
		t.Fatal(err)
	}
	return st
}

func connect(t *testing.T, protocolVersion string) (*mcp.ClientSession, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(NewHandler(seedStore(t), "test"))
	t.Cleanup(server.Close)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: server.URL}, &mcp.ClientSessionOptions{ProtocolVersion: protocolVersion})
	if err != nil {
		t.Fatalf("connect with %q: %v", protocolVersion, err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session, server
}

// call runs a tool and returns its text and decoded result.
func call(t *testing.T, session *mcp.ClientSession, name string, arguments map[string]any) (string, map[string]any) {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if result.IsError {
		t.Fatalf("%s: tool error: %s", name, text)
	}
	if strings.Contains(text, seededToken) {
		t.Errorf("%s leaked a seeded credential", name)
	}
	if len(text) > hardResponseBytes {
		t.Errorf("%s returned %d bytes", name, len(text))
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(text), &decoded); err != nil {
		t.Fatalf("%s returned invalid JSON after redaction: %v", name, err)
	}
	return text, decoded
}

func TestHandshakeOnEveryProtocolEra(t *testing.T) {
	for _, version := range []string{"", "2025-11-25", "2025-06-18", "2025-03-26"} {
		t.Run("version "+version, func(t *testing.T) {
			session, _ := connect(t, version)
			listed, err := session.ListTools(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			names := map[string]bool{}
			for _, tool := range listed.Tools {
				names[tool.Name] = true
				if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
					t.Errorf("%s is not annotated read-only", tool.Name)
				}
				if tool.Description == "" {
					t.Errorf("%s has no description", tool.Name)
				}
			}
			for _, want := range []string{"list_traces", "list_sessions", "get_session_timeline", "get_trace", "get_observation", "recent_errors", "slow_calls"} {
				if !names[want] {
					t.Errorf("missing tool %s", want)
				}
			}
			if len(names) != 7 {
				t.Errorf("tools = %v", names)
			}
		})
	}
}

func TestListTracesFiltersAndPages(t *testing.T) {
	session, _ := connect(t, "")
	_, all := call(t, session, "list_traces", map[string]any{"limit": 10})
	if all["total"].(float64) != 72 { // noise hidden
		t.Errorf("total = %v", all["total"])
	}
	if all["nextOffset"].(float64) != 10 || !strings.Contains(all["truncated"].(string), "offset=10") {
		t.Errorf("truncation is not stated: %v %v", all["nextOffset"], all["truncated"])
	}
	_, capped := call(t, session, "list_traces", map[string]any{"limit": 1000})
	if capped["returned"].(float64) != 50 {
		t.Errorf("limit ceiling not applied: %v", capped["returned"])
	}
	_, last := call(t, session, "list_traces", map[string]any{"offset": 70, "limit": 10})
	if last["returned"].(float64) != 2 || last["nextOffset"] != nil {
		t.Errorf("last page = %v", last)
	}
	_, withNoise := call(t, session, "list_traces", map[string]any{"include_noise": true, "limit": 1})
	if withNoise["total"].(float64) != 73 {
		t.Errorf("include_noise total = %v", withNoise["total"])
	}
	_, errored := call(t, session, "list_traces", map[string]any{"errors_only": true})
	if errored["total"].(float64) != 1 || errored["traces"].([]any)[0].(map[string]any)["id"] != "t2" {
		t.Errorf("errors_only = %v", errored)
	}
	_, thread := call(t, session, "list_traces", map[string]any{"thread_id": "9"})
	if thread["total"].(float64) != 2 {
		t.Errorf("thread filter = %v", thread["total"])
	}
	_, recent := call(t, session, "list_traces", map[string]any{"from": "90m"})
	if recent["total"].(float64) != 2 {
		t.Errorf("relative from = %v", recent["total"])
	}
}

func TestInvalidArgumentsAreToolErrors(t *testing.T) {
	session, _ := connect(t, "")
	cases := []struct {
		tool      string
		arguments map[string]any
		want      string
	}{
		{"list_traces", map[string]any{"status": "weird"}, "not one of"},
		{"list_traces", map[string]any{"from": "yesterday"}, "RFC 3339"},
		{"list_traces", map[string]any{"session_id": "a", "thread_id": "b"}, "not both"},
		{"get_trace", map[string]any{"trace_id": "missing"}, "not found"},
		{"get_observation", map[string]any{"observation_id": "gen", "offset": 5}, "need field"},
		{"get_observation", map[string]any{"observation_id": "gen", "field": "nope"}, "not one of"},
		{"get_session_timeline", map[string]any{}, "required"},
		{"slow_calls", map[string]any{"kind": "nope"}, "not one of"},
	}
	for _, current := range cases {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: current.tool, Arguments: current.arguments})
		if err != nil {
			t.Errorf("%s: protocol error instead of tool error: %v", current.tool, err)
			continue
		}
		text := result.Content[0].(*mcp.TextContent).Text
		if !result.IsError || !strings.Contains(text, current.want) {
			t.Errorf("%s %v: isError %v text %q", current.tool, current.arguments, result.IsError, text)
		}
	}
}

func TestGetTraceTreeFoldsOneProviderCall(t *testing.T) {
	session, _ := connect(t, "")
	_, folded := call(t, session, "get_trace", map[string]any{"trace_id": "t1"})
	nodes := folded["nodes"].([]any)
	if folded["total"].(float64) != 2 || len(nodes) != 2 {
		t.Fatalf("nodes = %v", nodes)
	}
	model := nodes[1].(map[string]any)
	if model["depth"].(float64) != 1 || model["role"] != "model_call" || model["label"] != "gpt" {
		t.Errorf("model node = %v", model)
	}
	if model["durationMs"].(float64) < 9000 {
		t.Errorf("folded call lost the stream's duration: %v", model["durationMs"])
	}
	_, raw := call(t, session, "get_trace", map[string]any{"trace_id": "t1", "raw_spans": true})
	if raw["total"].(float64) != 3 {
		t.Errorf("raw total = %v", raw["total"])
	}
	if message := folded["trace"].(map[string]any)["userMessage"].(map[string]any); !strings.HasPrefix(message["text"].(string), "Fix the listing") {
		t.Errorf("trace header lacks the user message: %v", message)
	}
}

func TestGetObservationIsBoundedAndChunked(t *testing.T) {
	session, _ := connect(t, "")
	text, all := call(t, session, "get_observation", map[string]any{"observation_id": "gen"})
	input := all["input"].(map[string]any)
	if len([]rune(input["text"].(string))) != defaultFieldChars || input["truncated"] == nil {
		t.Errorf("input = %d chars, truncated %v", len([]rune(input["text"].(string))), input["truncated"])
	}
	if all["summary"] == nil || all["usage"] == nil || all["mergedSpans"] == nil {
		t.Errorf("header incomplete: %s", text[:200])
	}
	total := int(input["totalChars"].(float64))
	next := int(input["nextOffset"].(float64))
	_, second := call(t, session, "get_observation", map[string]any{"observation_id": "gen", "field": "input", "offset": next, "max_chars": 100})
	chunk := second["input"].(map[string]any)
	if int(chunk["offset"].(float64)) != next || len([]rune(chunk["text"].(string))) != 100 || second["output"] != nil {
		t.Errorf("second chunk = %v", chunk["offset"])
	}
	_, tail := call(t, session, "get_observation", map[string]any{"observation_id": "gen", "field": "input", "from_end": true, "max_chars": 200})
	end := tail["input"].(map[string]any)
	if !strings.Contains(end["text"].(string), "[REDACTED:") || end["nextOffset"] != nil || int(end["offset"].(float64)) != total-200 {
		t.Errorf("tail = %v", end)
	}
	_, huge := call(t, session, "get_observation", map[string]any{"observation_id": "gen", "field": "input", "max_chars": 10_000_000})
	if got := len([]rune(huge["input"].(map[string]any)["text"].(string))); got != maxFieldChars {
		t.Errorf("max_chars ceiling not applied: %d", got)
	}
	_, reported := call(t, session, "get_observation", map[string]any{"observation_id": "gen", "field": "metadata"})
	if reported["redactions"] == nil {
		t.Error("redaction was silent")
	}
}

func TestSessionTimelineIsBounded(t *testing.T) {
	session, _ := connect(t, "")
	_, timeline := call(t, session, "get_session_timeline", map[string]any{"thread_id": "9"})
	items := timeline["items"].([]any)
	if len(items) != 2 || timeline["title"] == nil {
		t.Fatalf("timeline = %v", timeline)
	}
	turn := items[0].(map[string]any)["turn"].(map[string]any)
	message := turn["userMessage"].(map[string]any)
	if len([]rune(message["text"].(string))) != 500 || message["totalChars"].(float64) <= 500 {
		t.Errorf("message not clipped with its length stated: %v", message["totalChars"])
	}
	job := items[1].(map[string]any)["turn"].(map[string]any)
	if job["kind"] != "job" || !strings.Contains(job["statusMessage"].(string), "[REDACTED:") {
		t.Errorf("job = %v", job)
	}
	_, paged := call(t, session, "get_session_timeline", map[string]any{"session_id": "thread:9", "limit": 1})
	if paged["returned"].(float64) != 1 || paged["nextOffset"].(float64) != 1 {
		t.Errorf("paged = %v", paged)
	}
}

func TestRecentErrorsAndSlowCalls(t *testing.T) {
	session, _ := connect(t, "")
	_, failures := call(t, session, "recent_errors", nil)
	rows := failures["errors"].([]any)
	if failures["total"].(float64) != 1 || rows[0].(map[string]any)["observationId"] != "job" {
		t.Errorf("errors = %v", failures)
	}
	if failures["redactions"] == nil {
		t.Error("status message redaction was silent")
	}
	_, none := call(t, session, "recent_errors", map[string]any{"to": "1h"})
	if none["total"].(float64) != 0 {
		t.Errorf("window ignored: %v", none["total"])
	}
	_, slow := call(t, session, "slow_calls", map[string]any{"kind": "model_call", "top_spans": 1})
	groups := slow["groups"].([]any)
	if len(groups) != 1 || groups[0].(map[string]any)["count"].(float64) != 1 {
		t.Errorf("one provider call must count once: %v", groups)
	}
	if len(slow["slowest"].([]any)) != 1 {
		t.Errorf("top_spans ignored: %v", slow["slowest"])
	}
}

func TestForeignOriginIsForbidden(t *testing.T) {
	server := httptest.NewServer(NewHandler(seedStore(t), "test"))
	defer server.Close()
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"x","version":"1"}}}`
	post := func(origin string) int {
		request, err := http.NewRequest(http.MethodPost, server.URL, bytes.NewBufferString(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json, text/event-stream")
		if origin != "" {
			request.Header.Set("Origin", origin)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		return response.StatusCode
	}
	for origin, want := range map[string]int{
		"":                              http.StatusOK,
		"http://localhost:5173":         http.StatusOK,
		"http://127.0.0.1:7625":         http.StatusOK,
		"http://[::1]:7625":             http.StatusOK,
		"http://evil.example":           http.StatusForbidden,
		"http://evil.example:80":        http.StatusForbidden,
		"null":                          http.StatusForbidden,
		"http://localhost.evil.example": http.StatusForbidden,
	} {
		if got := post(origin); got != want {
			t.Errorf("origin %q: status %d, want %d", origin, got, want)
		}
	}
}

func TestUnknownProtocolVersionNamesTheSupportedOnes(t *testing.T) {
	server := httptest.NewServer(NewHandler(seedStore(t), "test"))
	defer server.Close()
	request, err := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(`{"jsonrpc":"2.0","id":7,"method":"tools/list"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("MCP-Protocol-Version", "1900-01-01")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	var body struct {
		ID    int `json:"id"`
		Error struct {
			Code int `json:"code"`
			Data struct {
				Supported []string `json:"supported"`
				Requested string   `json:"requested"`
			} `json:"data"`
		} `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusBadRequest || body.ID != 7 || body.Error.Code != -32022 ||
		body.Error.Data.Requested != "1900-01-01" || body.Error.Data.Supported[0] != "2026-07-28" {
		t.Errorf("status %d, body %+v", response.StatusCode, body)
	}
}

func TestRedactorRemovesCredentialsAndCountsThem(t *testing.T) {
	token := "abc123def456ghi789"
	cases := []string{
		`{"authorization":"Bearer ` + token + `"}`,
		`{"Authorization":"` + token + `"}`,
		`{"x-api-key":"` + token + `"}`,
		`{"access_token":"` + token + `"}`,
		`{"name":"Authorization","value":"` + token + `"}`,
		`{"headers":"{\"cookie\":\"` + token + `\"}"}`,
		`curl -H "Authorization: Basic ` + token + `"`,
		`token is Bearer ` + token + ` here`,
		`https://x.test/a?b=1&api_key=` + token + `&c=2`,
		`eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0In0.abc123def`,
		`key sk-` + token + `abc`,
		`ghp_` + token + `abcdefgh`,
	}
	for _, input := range cases {
		filter := newRedactor()
		output := filter.Text(input)
		if strings.Contains(output, token) || strings.Contains(output, "eyJzdWIi") {
			t.Errorf("not redacted: %s -> %s", input, output)
		}
		if filter.report() == nil {
			t.Errorf("no report for %s", input)
		}
	}
}

func TestRedactorLeavesOrdinaryDataAlone(t *testing.T) {
	cases := []string{
		`{"input_tokens":15749,"output_tokens":76,"total_tokens":15825,"cached_input_tokens":12}`,
		`{"token":{"type":"string","description":"x"}}`,
		`Use bearer authentication and authorization: required for this endpoint`,
		`{"input":"a","url":"https://x.test/dp/B07?ref=abc123&keywords=echo"}`,
	}
	for _, input := range cases {
		filter := newRedactor()
		if output := filter.Text(input); output != input || filter.report() != nil {
			t.Errorf("changed: %s -> %s", input, output)
		}
	}
	filter := newRedactor()
	once := filter.Text(`{"token":"` + seededToken + `"}`)
	if twice := filter.Text(once); twice != once || filter.counts["credential_field"] != 1 {
		t.Errorf("not idempotent: %s, %v", twice, filter.counts)
	}
}

func TestRenderRedactsQueryParametersAfterEncoding(t *testing.T) {
	filter := newRedactor()
	text, err := filter.render(map[string]string{"url": "https://x.test/a?b=1&api_key=" + seededToken, "html": "<b>&"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, seededToken) || !strings.Contains(text, "<b>&") || !json.Valid([]byte(text)) {
		t.Errorf("text = %s", text)
	}
}

func TestRenderRefusesOversizedResults(t *testing.T) {
	filter := newRedactor()
	if _, err := filter.render(map[string]string{"text": strings.Repeat("x", hardResponseBytes)}); err == nil || !strings.Contains(err.Error(), "narrow") {
		t.Errorf("err = %v", err)
	}
}
