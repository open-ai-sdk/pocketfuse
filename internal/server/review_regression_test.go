package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/open-ai-sdk/pocketfuse/internal/store"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	oteltrace "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

func TestOTLPBatchRootWithoutInputPreservesChildTraceInput(t *testing.T) {
	for _, rootFirst := range []bool{false, true} {
		name := "child-first"
		if rootFirst {
			name = "root-first"
		}
		t.Run(name, func(t *testing.T) {
			handler := testHandler(t)
			root := testSpan(chatTrace, "0000000000000001", "", "root", 0, 10)
			child := testSpan(chatTrace, "0000000000000002", "0000000000000001", "child", 1, 2,
				stringAttr("langfuse.trace.input", `{"message":"keep this input"}`))
			spans := []*oteltrace.Span{child, root}
			if rootFirst {
				spans = []*oteltrace.Span{root, child}
			}
			postSpans(t, handler, spans...)
			var trace store.Trace
			getJSON(t, handler, "/api/traces/"+chatTrace, &trace)
			if string(trace.Input) != `{"message":"keep this input"}` {
				t.Errorf("trace input = %s, want the child's trace input", trace.Input)
			}
		})
	}
}

func TestOTLPBatchRootSuppliedInputStillOverridesEarlierChild(t *testing.T) {
	for _, key := range []string{"langfuse.trace.input", "langfuse.observation.input"} {
		t.Run(key, func(t *testing.T) {
			handler := testHandler(t)
			postSpans(t, handler,
				testSpan(chatTrace, "0000000000000002", "0000000000000001", "child", 1, 2,
					stringAttr("langfuse.trace.input", `"child input"`)),
				testSpan(chatTrace, "0000000000000001", "", "root", 0, 10,
					stringAttr(key, `"root input"`)),
			)
			var trace store.Trace
			getJSON(t, handler, "/api/traces/"+chatTrace, &trace)
			if string(trace.Input) != `"root input"` {
				t.Errorf("trace input = %s, want the supplied root input", trace.Input)
			}
		})
	}
}

func TestOTLPHTTPPreservesBareUsageAliases(t *testing.T) {
	keys := []string{
		"input", "promptTokens", "prompt_tokens",
		"output", "completionTokens", "completion_tokens",
		"total", "totalTokens", "total_tokens",
		"cached_input", "cache_creation_input", "reasoning", "tool_use_prompt",
	}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			handler := testHandler(t)
			postSpans(t, handler, testSpan(genTrace, "0000000000000001", "", "generation", 0, 1,
				stringAttr("gen_ai.request.model", "test-model"), stringAttr(key, "17")))
			var page store.Page[store.Observation]
			getJSON(t, handler, "/api/observations?traceId="+genTrace, &page)
			if len(page.Data) != 1 {
				t.Fatalf("observations = %+v, want one generation", page.Data)
			}
			var usage map[string]float64
			if err := json.Unmarshal(page.Data[0].Usage, &usage); err != nil {
				t.Fatalf("usage = %s: %v", page.Data[0].Usage, err)
			}
			if usage[key] != 17 {
				t.Errorf("usage = %s, want bare %s = 17", page.Data[0].Usage, key)
			}
			switch key {
			case "input", "promptTokens", "prompt_tokens", "output", "completionTokens", "completion_tokens",
				"total", "totalTokens", "total_tokens":
				var trace store.Trace
				getJSON(t, handler, "/api/traces/"+genTrace, &trace)
				if trace.TotalTokens == nil || *trace.TotalTokens != 17 {
					t.Errorf("trace total tokens = %v, want 17 from bare %s", trace.TotalTokens, key)
				}
				if trace.Turn == nil || trace.Turn.Usage.Total == nil || *trace.Turn.Usage.Total != 17 {
					t.Errorf("generation turn = %+v, want total usage 17 from bare %s", trace.Turn, key)
				}
			}
		})
	}
}

func TestOTLPHTTPUsageDetailsKeepPrecedenceOverBareAndGenAI(t *testing.T) {
	handler := testHandler(t)
	postSpans(t, handler, testSpan(genTrace, "0000000000000001", "", "generation", 0, 1,
		stringAttr("gen_ai.request.model", "test-model"),
		stringAttr("langfuse.observation.usage_details", `{"input":11,"output":7,"total":18,"cached_input":3}`),
		stringAttr("input", "110"), stringAttr("output", "70"), stringAttr("total", "180"),
		stringAttr("cached_input", "30"), stringAttr("prompt_tokens", "111"),
		stringAttr("gen_ai.usage.input_tokens", "112"), stringAttr("gen_ai.usage.output_tokens", "72"),
		stringAttr("gen_ai.usage.total_tokens", "182"), stringAttr("gen_ai.usage.cache_read.input_tokens", "32")))
	var trace store.Trace
	getJSON(t, handler, "/api/traces/"+genTrace, &trace)
	if trace.TotalTokens == nil || *trace.TotalTokens != 18 {
		t.Errorf("total tokens = %v, want Langfuse details total 18", trace.TotalTokens)
	}
	if len(trace.Observations) != 1 {
		t.Fatalf("observations = %+v, want one generation", trace.Observations)
	}
	var usage map[string]float64
	if err := json.Unmarshal(trace.Observations[0].Usage, &usage); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]float64{"input": 11, "output": 7, "total": 18, "cached_input": 3} {
		if usage[key] != want {
			t.Errorf("usage[%q] = %v, want details value %v", key, usage[key], want)
		}
	}
}

func TestOTLPBatchObservationProjectsMatchFinalTraceProject(t *testing.T) {
	for _, rootFirst := range []bool{false, true} {
		name := "child-first"
		if rootFirst {
			name = "root-first"
		}
		t.Run(name, func(t *testing.T) {
			handler := testHandler(t)
			resource := func(project string, span *oteltrace.Span) *oteltrace.ResourceSpans {
				return &oteltrace.ResourceSpans{
					Resource:   &resourcev1.Resource{Attributes: []*commonv1.KeyValue{stringAttr("service.name", project)}},
					ScopeSpans: []*oteltrace.ScopeSpans{{Spans: []*oteltrace.Span{span}}},
				}
			}
			root := resource("root-service", testSpan(chatTrace, "0000000000000001", "", "root", 0, 10))
			child := resource("child-service", testSpan(chatTrace, "0000000000000002", "0000000000000001", "child", 1, 2))
			resources := []*oteltrace.ResourceSpans{child, root}
			wantProject, staleProject := "root-service", "child-service"
			if rootFirst {
				resources = []*oteltrace.ResourceSpans{root, child}
				wantProject, staleProject = staleProject, wantProject
			}
			resources = append(resources, resource("other-service", testSpan(jobTrace, "0000000000000003", "", "unrelated", 0, 1)))
			body, err := proto.Marshal(&collectortrace.ExportTraceServiceRequest{ResourceSpans: resources})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/api/public/otel/v1/traces", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/x-protobuf")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("ingest: %d %s", response.Code, response.Body)
			}
			var trace store.Trace
			getJSON(t, handler, "/api/traces/"+chatTrace, &trace)
			if trace.ProjectID != wantProject || trace.Project != wantProject {
				t.Errorf("trace projects = %q, %q; want %q", trace.ProjectID, trace.Project, wantProject)
			}
			for _, check := range []struct {
				project string
				count   int
				traceID string
			}{{wantProject, 2, chatTrace}, {staleProject, 0, chatTrace}, {"other-service", 1, jobTrace}} {
				var page store.Page[store.Observation]
				getJSON(t, handler, "/api/observations?project="+check.project, &page)
				if page.Total != check.count || len(page.Data) != check.count {
					t.Errorf("project %q observations = %d/%d, want %d", check.project, page.Total, len(page.Data), check.count)
				}
				for _, observation := range page.Data {
					if observation.ProjectID != check.project || observation.TraceID != check.traceID {
						t.Errorf("project %q leaked observation %+v", check.project, observation)
					}
				}
			}
		})
	}
}

func seedReviewRouteIDs(t *testing.T, handler http.Handler) {
	t.Helper()
	for _, value := range []struct{ trace, session string }{
		{"foo", "team"}, {"foo/related", "team/story"}, {"foo%2Frelated", "team%2Fstory"},
	} {
		body, err := json.Marshal(map[string]any{"trace": map[string]string{"id": value.trace, "sessionId": value.session}})
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/ingest", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusAccepted {
			t.Fatalf("seed trace %q: %d %s", value.trace, response.Code, response.Body)
		}
	}
}

func TestHTTPTraceRoutesSeparateEscapedIDsFromRelatedSuffix(t *testing.T) {
	handler := testHandler(t)
	seedReviewRouteIDs(t, handler)
	for _, check := range []struct{ path, id string }{
		{"/api/traces/foo", "foo"},
		{"/api/traces/foo%2Frelated", "foo/related"},
		{"/api/traces/foo%2frelated", "foo/related"},
		{"/api/traces/foo%252Frelated", "foo%2Frelated"},
		{"/api/traces/foo/%72elated", "foo/related"},
	} {
		t.Run(check.path, func(t *testing.T) {
			var trace store.Trace
			getJSON(t, handler, check.path, &trace)
			if trace.ID != check.id {
				t.Errorf("trace id = %q, want %q", trace.ID, check.id)
			}
		})
	}
	for _, path := range []string{"/api/traces/foo/related", "/api/traces/foo%2Frelated/related", "/api/traces/foo%252Frelated/related"} {
		var related map[string]json.RawMessage
		getJSON(t, handler, path, &related)
		if related["sameSession"] == nil || related["overlapping"] == nil || related["id"] != nil {
			t.Errorf("GET %s = %v, want the related-traces response", path, related)
		}
	}
}

func TestHTTPSessionRoutesSeparateEscapedIDsFromStorySuffix(t *testing.T) {
	handler := testHandler(t)
	seedReviewRouteIDs(t, handler)
	for _, check := range []struct{ path, id string }{
		{"/api/sessions/team", "team"},
		{"/api/sessions/team%2Fstory", "team/story"},
		{"/api/sessions/team%2fstory", "team/story"},
		{"/api/sessions/team%252Fstory", "team%2Fstory"},
		{"/api/sessions/team/%73tory", "team/story"},
	} {
		t.Run(check.path, func(t *testing.T) {
			var detail store.SessionDetail
			getJSON(t, handler, check.path, &detail)
			if detail.ID != check.id || len(detail.Traces) != 1 {
				t.Errorf("detail = %+v, want session %q with its one trace", detail, check.id)
			}
		})
	}
	for _, check := range []struct{ path, id string }{
		{"/api/sessions/team/story", "team"},
		{"/api/sessions/team%2Fstory/story", "team/story"},
		{"/api/sessions/team%252Fstory/story", "team%2Fstory"},
	} {
		var story store.SessionStory
		getJSON(t, handler, check.path, &story)
		if story.ID != check.id || len(story.Items) != 1 {
			t.Errorf("GET %s = %+v, want story for %q", check.path, story, check.id)
		}
	}
}
