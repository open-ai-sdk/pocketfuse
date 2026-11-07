package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/open-ai-sdk/pocketfuse/internal/store"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	oteltrace "go.opentelemetry.io/proto/otlp/trace/v1"
)

const (
	chatTrace   = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	genTrace    = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	jobTrace    = "cccccccccccccccccccccccccccccccc"
	noiseTrace  = "dddddddddddddddddddddddddddddddd"
	getErrTrace = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
)

func testHandler(t *testing.T) http.Handler {
	t.Helper()
	st, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return New(st).Handler()
}

func postSpans(t *testing.T, handler http.Handler, spans ...*oteltrace.Span) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/public/otel/v1/traces", bytes.NewReader(otlpBody(t, spans...)))
	request.Header.Set("Content-Type", "application/x-protobuf")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("ingest: %d %s", response.Code, response.Body)
	}
}

func getJSON(t *testing.T, handler http.Handler, path string, target any) {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("GET %s: %d %s", path, response.Code, response.Body)
	}
	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatal(err)
	}
}

// seedTurn stores a chat request, its detached generation, a job in the same
// thread, and two polls: the shape the Seller gateway sends today.
func seedTurn(t *testing.T, handler http.Handler) {
	t.Helper()
	postSpans(t, handler,
		testSpan(chatTrace, "0000000000000001", "", "POST /threads/{thread_id}/chat", 0, 5,
			stringAttr("http.route", "/threads/{thread_id}/chat"), stringAttr("url.path", "/threads/9/chat"), stringAttr("status_code", "200")),
		testSpan(genTrace, "0000000000000002", "", "llm.generation", 1, 1, generationAttributes()...),
		testSpan(genTrace, "0000000000000003", "0000000000000002", "chat_streaming", 1, 4,
			stringAttr("gen_ai.request.model", "gpt-test"), stringAttr("gen_ai.usage.input_tokens", "121"), stringAttr("gen_ai.usage.output_tokens", "60")),
		testSpan(jobTrace, "0000000000000004", "", "agent_job.run", 10, 20, stringAttr("thread_id", "9"), stringAttr("job_id", "job-1")),
		testSpan(noiseTrace, "0000000000000005", "", "queue.attributes.recover_expired", 1, 1),
		testSpan(getErrTrace, "0000000000000006", "", "GET /threads/{thread_id}/messages", 2, 3,
			stringAttr("http.route", "/threads/{thread_id}/messages"), stringAttr("url.path", "/threads/9/messages"), stringAttr("status_code", "500")),
	)
}

func TestHealthAnswersHeadForProbes(t *testing.T) {
	handler := testHandler(t)
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(method, "/api/health", nil))
		if response.Code != http.StatusOK {
			t.Errorf("%s /api/health = %d", method, response.Code)
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/health", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/health = %d", response.Code)
	}
}

func TestTraceTotalsCountOneCallOnceAndCoverChildDuration(t *testing.T) {
	handler := testHandler(t)
	seedTurn(t, handler)
	var trace store.Trace
	getJSON(t, handler, "/api/traces/"+genTrace, &trace)
	if trace.TotalTokens == nil || *trace.TotalTokens != 181 {
		t.Errorf("tokens = %v, want 181 (generation and its SDK child are one call)", trace.TotalTokens)
	}
	if trace.TotalCost != nil {
		t.Errorf("cost = %v, want unknown: the sender reported none", *trace.TotalCost)
	}
	if trace.Duration != 3000 {
		t.Errorf("duration = %v, want the child's 3s", trace.Duration)
	}
	if parent := trace.Observations[0]; parent.Duration != 3000 || parent.ReportedEndTime == "" {
		t.Errorf("parent not extended: duration=%v reported=%q", parent.Duration, parent.ReportedEndTime)
	}
}

func TestListCarriesSummaryAndHonestStatus(t *testing.T) {
	handler := testHandler(t)
	seedTurn(t, handler)
	var page store.Page[store.Trace]
	getJSON(t, handler, "/api/traces?limit=50", &page)
	byID := map[string]store.Trace{}
	for _, trace := range page.Data {
		byID[trace.ID] = trace
	}
	if got := byID[chatTrace].Status; got != "success" {
		t.Errorf("chat status = %q, want success from its 200", got)
	}
	if got := byID[genTrace].Status; got != "completed" {
		t.Errorf("generation status = %q, want completed: no outcome was reported", got)
	}
	if got := byID[getErrTrace].Status; got != "error" {
		t.Errorf("failed GET status = %q", got)
	}
	if byID[genTrace].TotalTokens == nil || *byID[genTrace].TotalTokens != 181 || byID[genTrace].ObservationCount != 2 {
		t.Errorf("generation summary = %+v", byID[genTrace])
	}
	if len(byID[genTrace].Models) != 1 || byID[genTrace].Models[0] != "gpt-test" {
		t.Errorf("models = %v", byID[genTrace].Models)
	}
}

func TestNoiseFilterKeepsFailuresAndIsOptIn(t *testing.T) {
	handler := testHandler(t)
	seedTurn(t, handler)
	var all, visible store.Page[store.Trace]
	getJSON(t, handler, "/api/traces?limit=50", &all)
	getJSON(t, handler, "/api/traces?limit=50&noise=hide", &visible)
	if all.Total != 5 {
		t.Errorf("default list hides traces: total = %d", all.Total)
	}
	if visible.Total != 4 {
		t.Errorf("filtered total = %d, want 4", visible.Total)
	}
	for _, trace := range visible.Data {
		if trace.ID == noiseTrace {
			t.Error("queue housekeeping was not hidden")
		}
	}
	found := false
	for _, trace := range visible.Data {
		found = found || trace.ID == getErrTrace
	}
	if !found {
		t.Error("a failed GET must stay visible")
	}
}

func TestSessionTimelineLinksThreadTracesAndFlagsTimeMatches(t *testing.T) {
	handler := testHandler(t)
	seedTurn(t, handler)
	var page store.Page[store.Session]
	getJSON(t, handler, "/api/sessions", &page)
	if page.Total != 1 || page.Data[0].ID != "thread:9" || page.Data[0].TraceCount != 3 {
		t.Fatalf("sessions = %+v", page)
	}
	var detail store.SessionDetail
	getJSON(t, handler, "/api/sessions/thread%3A9?noise=hide", &detail)
	if len(detail.Traces) != 3 || detail.Traces[0].ID != chatTrace || detail.Traces[2].ID != jobTrace {
		t.Errorf("timeline = %+v", detail.Traces)
	}
	if len(detail.Unlinked) != 1 || detail.Unlinked[0].ID != genTrace {
		t.Errorf("unlinked = %+v, want the generation that ran during the request", detail.Unlinked)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/sessions/missing", nil))
	if response.Code != http.StatusNotFound {
		t.Errorf("missing session = %d", response.Code)
	}
}

// A thread can span days: the generation of a later request must still match
// even when the time between its requests holds more traces than one query returns.
func TestTimeMatchesCoverEveryRequestOfALongThread(t *testing.T) {
	handler := testHandler(t)
	route := []*commonv1.KeyValue{stringAttr("http.route", "/threads/{thread_id}/chat"), stringAttr("url.path", "/threads/9/chat")}
	spans := []*oteltrace.Span{
		testSpan(chatTrace, "0000000000000001", "", "POST /threads/{thread_id}/chat", 0, 1, route...),
		testSpan(jobTrace, "0000000000000002", "", "POST /threads/{thread_id}/chat", 2000, 2005, route...),
		testSpan(genTrace, "0000000000000003", "", "llm.generation", 2001, 2002, generationAttributes()...),
	}
	for index := 0; index < 600; index++ {
		id := fmt.Sprintf("%032x", index+1)
		spans = append(spans, testSpan(id, fmt.Sprintf("%016x", index+100), "", "llm.generation", uint64(10+index), uint64(10+index), generationAttributes()...))
	}
	postSpans(t, handler, spans...)
	var detail store.SessionDetail
	getJSON(t, handler, "/api/sessions/thread%3A9", &detail)
	if len(detail.Unlinked) != 1 || detail.Unlinked[0].ID != genTrace {
		t.Errorf("unlinked = %d traces, want only the generation inside the second request", len(detail.Unlinked))
	}
}

func TestRelatedSeparatesLinksFromTimeOverlap(t *testing.T) {
	handler := testHandler(t)
	seedTurn(t, handler)
	var related store.RelatedTraces
	getJSON(t, handler, "/api/traces/"+genTrace+"/related", &related)
	if len(related.SameSession) != 0 {
		t.Errorf("a detached generation has no recorded link: %+v", related.SameSession)
	}
	found := false
	for _, trace := range related.Overlapping {
		found = found || trace.ID == chatTrace
		if trace.ID == noiseTrace {
			t.Error("noise listed as overlapping")
		}
	}
	if !found {
		t.Errorf("overlapping = %+v, want the chat request", related.Overlapping)
	}
	getJSON(t, handler, "/api/traces/"+chatTrace+"/related", &related)
	if len(related.SameSession) != 2 {
		t.Errorf("same session = %+v", related.SameSession)
	}
}

func TestLaterChildBatchKeepsRootNameAndTimestamp(t *testing.T) {
	handler := testHandler(t)
	postSpans(t, handler, testSpan(chatTrace, "0000000000000001", "", "POST /chat", 0, 5))
	postSpans(t, handler, testSpan(chatTrace, "0000000000000002", "0000000000000001", "child", 3, 8))
	var trace store.Trace
	getJSON(t, handler, "/api/traces/"+chatTrace, &trace)
	if trace.Name != "POST /chat" {
		t.Errorf("name = %q", trace.Name)
	}
	if trace.Duration != 8000 {
		t.Errorf("duration = %v, want it to cover the late child", trace.Duration)
	}
	var page store.Page[store.Trace]
	getJSON(t, handler, "/api/traces", &page)
	if page.Data[0].Timestamp != trace.Observations[0].StartTime {
		t.Errorf("timestamp moved to the child: %s", page.Data[0].Timestamp)
	}
}

func TestSearchMatchesTraceIDPrefix(t *testing.T) {
	handler := testHandler(t)
	seedTurn(t, handler)
	var page store.Page[store.Trace]
	getJSON(t, handler, "/api/traces?name=aaaaaaaa", &page)
	if page.Total != 1 || page.Data[0].ID != chatTrace {
		t.Errorf("search = %+v", page.Data)
	}
}

func TestSpanEventsExplainAFailureWithNoStatusMessage(t *testing.T) {
	span := testSpan(jobTrace, "1111111111111111", "", "agent_job.run", 0, 5)
	span.Status = &oteltrace.Status{Code: oteltrace.Status_STATUS_CODE_ERROR}
	span.Events = []*oteltrace.Span_Event{
		{Name: "still working", Attributes: []*commonv1.KeyValue{stringAttr("level", "INFO")}},
		{Name: "agent job write failed", TimeUnixNano: spanStart + second, Attributes: []*commonv1.KeyValue{stringAttr("level", "ERROR")}},
	}
	_, observations := decodeSpans(t, span)
	if observations[0].StatusMessage != "agent job write failed" {
		t.Errorf("status message = %q", observations[0].StatusMessage)
	}
	var metadata struct {
		Events []map[string]any `json:"otel.events"`
	}
	if err := json.Unmarshal(observations[0].Metadata, &metadata); err != nil || len(metadata.Events) != 2 {
		t.Errorf("events = %s (%v)", observations[0].Metadata, err)
	}
	span.Status.Message = "None"
	if _, empty := decodeSpans(t, span); empty[0].StatusMessage != "agent job write failed" {
		t.Errorf("a debug-formatted empty Option is no message: %q", empty[0].StatusMessage)
	}
	span.Status.Message = "explicit"
	if _, again := decodeSpans(t, span); again[0].StatusMessage != "explicit" {
		t.Errorf("status message should win over events: %q", again[0].StatusMessage)
	}
}

func TestRootSpanSetsTimestampWhateverTheExportOrder(t *testing.T) {
	trace, _ := decodeSpans(t,
		testSpan(chatTrace, "2222222222222222", "1111111111111111", "child", 30, 31),
		testSpan(chatTrace, "1111111111111111", "", "root", 0, 40),
	)
	if trace.Timestamp != unixNanoTime(spanStart) {
		t.Errorf("timestamp = %s, want the root's start", trace.Timestamp)
	}
}

func TestSessionStoryEndpoint(t *testing.T) {
	handler := testHandler(t)
	seedTurn(t, handler)
	var story struct {
		ID     string `json:"id"`
		Totals struct {
			Turns int `json:"turns"`
		} `json:"totals"`
		Items []struct {
			Type string `json:"type"`
		} `json:"items"`
	}
	getJSON(t, handler, "/api/sessions/thread:9/story?noise=hide", &story)
	if story.ID != "thread:9" || len(story.Items) == 0 {
		t.Errorf("story = %+v", story)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/sessions/missing/story", nil))
	if response.Code != http.StatusNotFound {
		t.Errorf("missing session: %d", response.Code)
	}
}

func TestMCPEndpointIsMountedWithoutWildcardCORS(t *testing.T) {
	handler := testHandler(t)
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"x","version":"1"}}}`
	request := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte("pocketfuse")) {
		t.Errorf("initialize: %d %s", response.Code, response.Body)
	}
	if value := response.Header().Get("Access-Control-Allow-Origin"); value != "" {
		t.Errorf("MCP must not allow every origin: %q", value)
	}
	api := httptest.NewRecorder()
	handler.ServeHTTP(api, httptest.NewRequest(http.MethodGet, "/api/traces", nil))
	if api.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("the JSON API keeps its CORS policy")
	}
	spa := httptest.NewRecorder()
	handler.ServeHTTP(spa, httptest.NewRequest(http.MethodGet, "/mcp", nil))
	if spa.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /mcp: %d, the transport has no GET stream in stateless mode", spa.Code)
	}
}
