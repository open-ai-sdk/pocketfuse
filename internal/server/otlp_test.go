package server

import (
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/open-ai-sdk/pocketfuse/internal/store"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	oteltrace "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

const (
	spanStart = uint64(1_800_000_000_000_000_000)
	second    = uint64(1_000_000_000)
)

func stringAttr(key, value string) *commonv1.KeyValue {
	return &commonv1.KeyValue{Key: key, Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: value}}}
}

// testSpan builds a span from hex ids; offsets are seconds after spanStart.
func testSpan(traceID, spanID, parentID, name string, startOffset, endOffset uint64, attributes ...*commonv1.KeyValue) *oteltrace.Span {
	decode := func(value string) []byte { decoded, _ := hex.DecodeString(value); return decoded }
	return &oteltrace.Span{TraceId: decode(traceID), SpanId: decode(spanID), ParentSpanId: decode(parentID), Name: name,
		StartTimeUnixNano: spanStart + startOffset*second, EndTimeUnixNano: spanStart + endOffset*second, Attributes: attributes}
}

func otlpBody(t *testing.T, spans ...*oteltrace.Span) []byte {
	t.Helper()
	body, err := proto.Marshal(&collectortrace.ExportTraceServiceRequest{ResourceSpans: []*oteltrace.ResourceSpans{{
		Resource:   &resourcev1.Resource{Attributes: []*commonv1.KeyValue{stringAttr("service.name", "api-gateway")}},
		ScopeSpans: []*oteltrace.ScopeSpans{{Spans: spans}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func decodeSpans(t *testing.T, spans ...*oteltrace.Span) (*store.Trace, []store.Observation) {
	t.Helper()
	records, err := decodeOTLP(otlpBody(t, spans...), "application/x-protobuf")
	if err != nil {
		t.Fatal(err)
	}
	var trace *store.Trace
	var observations []store.Observation
	for _, record := range records {
		switch record.Type {
		case store.RecordTrace:
			trace = record.Trace
		case store.RecordObservation:
			observations = append(observations, *record.Observation)
		}
	}
	return trace, observations
}

// The attribute names below are the ones the Seller API gateway sends on its
// `llm.generation` spans (crates/llm/src/telemetry.rs).
func generationAttributes() []*commonv1.KeyValue {
	return []*commonv1.KeyValue{
		stringAttr("langfuse.observation.type", "generation"),
		stringAttr("langfuse.observation.name", "openai.responses.stream"),
		stringAttr("langfuse.observation.input", `{"messages":[{"role":"user","content":"hi"}]}`),
		stringAttr("langfuse.observation.output", `{"text":"hello"}`),
		stringAttr("langfuse.observation.model.name", "gpt-test"),
		stringAttr("langfuse.observation.model.parameters", `{"reasoning":{"effort":"low"}}`),
		stringAttr("langfuse.observation.usage_details", `{"input":121,"output":60,"total":181,"cached_input":100}`),
		stringAttr("gen_ai.operation.name", "chat"),
		stringAttr("gen_ai.provider.name", "openai"),
		stringAttr("gen_ai.request.model", "gpt-test"),
	}
}

func TestGenerationKeepsModelParametersProviderAndCanonicalUsage(t *testing.T) {
	_, observations := decodeSpans(t, testSpan("0102030405060708090a0b0c0d0e0f10", "0102030405060708", "", "llm.generation", 0, 1, generationAttributes()...))
	generation := observations[0]
	if generation.Type != "GENERATION" || generation.Name != "openai.responses.stream" || generation.Model != "gpt-test" {
		t.Fatalf("unexpected generation: %+v", generation)
	}
	if string(generation.ModelParameters) != `{"reasoning":{"effort":"low"}}` {
		t.Errorf("model parameters dropped: %s", generation.ModelParameters)
	}
	var usage map[string]float64
	if err := json.Unmarshal(generation.Usage, &usage); err != nil {
		t.Fatal(err)
	}
	if usage["input"] != 121 || usage["output"] != 60 || usage["total"] != 181 || usage["cached_input"] != 100 {
		t.Errorf("usage not canonical: %s", generation.Usage)
	}
	var metadata map[string]any
	if err := json.Unmarshal(generation.Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["gen_ai.provider.name"] != "openai" || metadata["gen_ai.operation.name"] != "chat" {
		t.Errorf("gen_ai attributes dropped: %s", generation.Metadata)
	}
	if _, leaked := metadata["langfuse.observation.input"]; leaked {
		t.Errorf("promoted attribute repeated in metadata: %s", generation.Metadata)
	}
}

func TestChatRouteLinksTraceToThreadAndUser(t *testing.T) {
	trace, _ := decodeSpans(t, testSpan("0102030405060708090a0b0c0d0e0f10", "0102030405060708", "", "POST /threads/{thread_id}/chat", 0, 5,
		stringAttr("http.route", "/threads/{thread_id}/chat"), stringAttr("url.path", "/threads/9/chat"),
		stringAttr("user_id", "1"), stringAttr("session_id", "77")))
	if trace.SessionID != "thread:9" {
		t.Errorf("session = %q, want the thread rather than the auth session", trace.SessionID)
	}
	if trace.UserID != "1" {
		t.Errorf("user = %q", trace.UserID)
	}
}

func TestJobSpanCorrelationReachesTraceMetadataAndUnwrapsOption(t *testing.T) {
	trace, _ := decodeSpans(t, testSpan("0102030405060708090a0b0c0d0e0f10", "0102030405060708", "", "agent_job.run", 0, 5,
		stringAttr("thread_id", "92"), stringAttr("job_id", "job-1"), stringAttr("plan_id", "Some(plan-1)"), stringAttr("agent_id", "listing")))
	if trace.SessionID != "thread:92" {
		t.Errorf("session = %q", trace.SessionID)
	}
	var metadata map[string]string
	if err := json.Unmarshal(trace.Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["job_id"] != "job-1" || metadata["plan_id"] != "plan-1" || metadata["agent_id"] != "listing" {
		t.Errorf("metadata = %v", metadata)
	}
}

func TestExplicitSessionWinsOverThread(t *testing.T) {
	trace, _ := decodeSpans(t, testSpan("0102030405060708090a0b0c0d0e0f10", "0102030405060708", "", "turn", 0, 1,
		stringAttr("thread_id", "92"), stringAttr("langfuse.session.id", "explicit")))
	if trace.SessionID != "explicit" {
		t.Errorf("session = %q", trace.SessionID)
	}
}

func TestToolSpansAreTyped(t *testing.T) {
	_, observations := decodeSpans(t,
		testSpan("0102030405060708090a0b0c0d0e0f10", "0102030405060701", "", "execute_tool read", 0, 1),
		testSpan("0102030405060708090a0b0c0d0e0f10", "0102030405060702", "", "tools.helium10_lookup", 0, 1),
		testSpan("0102030405060708090a0b0c0d0e0f10", "0102030405060703", "", "x", 0, 1, stringAttr("langfuse.observation.type", "tool")),
		testSpan("0102030405060708090a0b0c0d0e0f10", "0102030405060704", "", "plain", 0, 1))
	want := []string{"TOOL", "TOOL", "TOOL", "SPAN"}
	for index, observation := range observations {
		if observation.Type != want[index] {
			t.Errorf("%s: type = %s, want %s", observation.Name, observation.Type, want[index])
		}
	}
}

func TestServerErrorResponseIsAnError(t *testing.T) {
	_, observations := decodeSpans(t, testSpan("0102030405060708090a0b0c0d0e0f10", "0102030405060708", "", "POST /x", 0, 1, stringAttr("status_code", "502")))
	if observations[0].Level != "ERROR" {
		t.Errorf("level = %s", observations[0].Level)
	}
}

func TestArrayAttributesAreStoredAsJSON(t *testing.T) {
	array := &commonv1.KeyValue{Key: "tags", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_ArrayValue{ArrayValue: &commonv1.ArrayValue{
		Values: []*commonv1.AnyValue{{Value: &commonv1.AnyValue_StringValue{StringValue: "a"}}, {Value: &commonv1.AnyValue_IntValue{IntValue: 2}}},
	}}}}
	_, observations := decodeSpans(t, testSpan("0102030405060708090a0b0c0d0e0f10", "0102030405060708", "", "x", 0, 1, array))
	var metadata map[string]any
	if err := json.Unmarshal(observations[0].Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	values, ok := metadata["tags"].([]any)
	if !ok || values[0] != "a" || values[1] != float64(2) {
		t.Errorf("tags = %#v", metadata["tags"])
	}
}
