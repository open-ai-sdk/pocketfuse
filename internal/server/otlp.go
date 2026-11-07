package server

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/open-ai-sdk/pocketfuse/internal/store"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	oteltrace "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func decodeOTLP(body []byte, contentType string) ([]store.IngestRecord, error) {
	request := new(collectortrace.ExportTraceServiceRequest)
	protobufBody := strings.Contains(strings.ToLower(contentType), "protobuf") || strings.Contains(strings.ToLower(contentType), "octet-stream")
	var err error
	if protobufBody {
		err = proto.Unmarshal(body, request)
	} else {
		// Protobuf JSON encodes bytes as base64 while the OTLP JSON mapping
		// convention commonly uses hexadecimal trace/span IDs. Normalize the
		// latter before handing the payload to the generated protobuf parser.
		if normalized, normalizeErr := normalizeOTLPJSONIDs(body); normalizeErr == nil {
			body = normalized
		}
		err = protojson.Unmarshal(body, request)
		if err != nil {
			err = proto.Unmarshal(body, request)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("decode OTLP trace payload: %w", err)
	}
	return recordsFromOTLP(request), nil
}

func normalizeOTLPJSONIDs(body []byte) ([]byte, error) {
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return nil, err
	}
	var visit func(any)
	visit = func(current any) {
		switch current := current.(type) {
		case []any:
			for _, item := range current {
				visit(item)
			}
		case map[string]any:
			for key, item := range current {
				if text, ok := item.(string); ok && isOTLPIDKey(key) {
					if decoded, err := hex.DecodeString(text); err == nil && (len(decoded) == 8 || len(decoded) == 16) {
						current[key] = base64.StdEncoding.EncodeToString(decoded)
					}
				}
				visit(item)
			}
		}
	}
	visit(value)
	return json.Marshal(value)
}

func isOTLPIDKey(key string) bool {
	switch strings.ToLower(key) {
	case "traceid", "spanid", "parentspanid":
		return true
	default:
		return false
	}
}

func recordsFromOTLP(request *collectortrace.ExportTraceServiceRequest) []store.IngestRecord {
	type traceGroup struct {
		trace    store.Trace
		obs      []store.Observation
		metadata map[string]any
	}
	groups := make(map[string]*traceGroup)
	order := make([]string, 0)
	for _, resourceSpans := range request.GetResourceSpans() {
		resourceAttrs := attrs(resourceSpans.GetResource().GetAttributes())
		project := first(resourceAttrs["service.name"], resourceAttrs["project.name"], "default")
		for _, scopeSpans := range resourceSpans.GetScopeSpans() {
			for _, span := range scopeSpans.GetSpans() {
				traceID := hex.EncodeToString(span.GetTraceId())
				if traceID == "" {
					traceID = newID("trace")
				}
				group := groups[traceID]
				if group == nil {
					group = &traceGroup{trace: store.Trace{ID: traceID, ProjectID: project, Project: project, CreatedAt: now(), Partial: true, ProvisionalTimestamp: true}, metadata: map[string]any{}}
					groups[traceID] = group
					order = append(order, traceID)
				}
				spanAttrs := attrs(span.GetAttributes())
				start := unixNanoTime(span.GetStartTimeUnixNano())
				end := unixNanoTime(span.GetEndTimeUnixNano())
				isRoot := strings.EqualFold(spanAttrs["langfuse.internal.is_app_root"], "true") || len(span.GetParentSpanId()) == 0
				correlation := store.CorrelationFromAttributes(spanAttrs)
				if group.trace.Name == "" || isRoot {
					group.trace.Name = first(spanAttrs["langfuse.trace.name"], span.GetName(), group.trace.Name)
				}
				if isRoot {
					group.trace.Partial = false
					group.trace.ProvisionalTimestamp = false
				}
				if explicit := spanAttrs["langfuse.trace.name"]; explicit != "" {
					group.trace.Name = explicit
					group.trace.Partial = false
				}
				group.trace.ProjectID = first(spanAttrs["langfuse.project.id"], project, group.trace.ProjectID)
				group.trace.Project = group.trace.ProjectID
				group.trace.UserID = first(spanAttrs["langfuse.trace.user_id"], spanAttrs["langfuse.user.id"], spanAttrs["user.id"], correlation.UserID, group.trace.UserID)
				group.trace.SessionID = first(spanAttrs["langfuse.trace.session_id"], spanAttrs["langfuse.session.id"], spanAttrs["session.id"], correlation.SessionID(), group.trace.SessionID)
				group.trace.Release = first(spanAttrs["langfuse.release"], resourceAttrs["service.version"], group.trace.Release)
				group.trace.Version = first(spanAttrs["langfuse.version"], group.trace.Version)
				group.trace.Environment = first(spanAttrs["langfuse.environment"], resourceAttrs["deployment.environment"], group.trace.Environment)
				group.trace.Timestamp = first(group.trace.Timestamp, start)
				if isRoot {
					// Spans export in end order, so the first one seen is not the earliest.
					group.trace.Timestamp = first(start, group.trace.Timestamp)
					group.trace.StartTime = first(start, group.trace.StartTime)
					group.trace.EndTime = first(end, group.trace.EndTime)
					if value := valueJSON(firstAny(spanAttrs, "langfuse.trace.input", "langfuse.observation.input")); value != nil {
						group.trace.Input = value
					}
					group.trace.Output = valueJSON(firstAny(spanAttrs, "langfuse.trace.output", "langfuse.observation.output"))
				} else {
					// Trace-level attributes may be set on any span of the trace.
					if value := valueJSON(spanAttrs["langfuse.trace.input"]); value != nil {
						group.trace.Input = value
					}
					if value := valueJSON(spanAttrs["langfuse.trace.output"]); value != nil {
						group.trace.Output = value
					}
				}
				for key, value := range correlation.Metadata() {
					if _, exists := group.metadata[key]; !exists {
						group.metadata[key] = value
					}
				}
				for key, value := range spanAttrs {
					const prefix = "langfuse.trace.metadata."
					if strings.HasPrefix(key, prefix) && value != "" {
						group.metadata[strings.TrimPrefix(key, prefix)] = parseStringOrJSON(value)
					}
				}
				if len(group.metadata) > 0 {
					group.trace.Metadata = marshalRaw(group.metadata)
				}
				observationType := observationTypeFor(span.GetName(), spanAttrs)
				observation := store.Observation{ID: hex.EncodeToString(span.GetSpanId()), TraceID: traceID, ProjectID: group.trace.ProjectID, ParentObservationID: hex.EncodeToString(span.GetParentSpanId()), Type: observationType, Name: first(spanAttrs["langfuse.observation.name"], span.GetName()), Input: valueJSON(firstAny(spanAttrs, "langfuse.observation.input", "gen_ai.input.messages", "gen_ai.prompt")), Output: valueJSON(firstAny(spanAttrs, "langfuse.observation.output", "gen_ai.output.messages", "gen_ai.completion")), Metadata: marshalRaw(spanMetadata(spanAttrs, span.GetEvents())), Model: first(spanAttrs["langfuse.observation.model.name"], spanAttrs["langfuse.observation.model"], spanAttrs["gen_ai.response.model"], spanAttrs["gen_ai.request.model"]), ModelParameters: valueJSON(firstAny(spanAttrs, "langfuse.observation.model.parameters")), Usage: usageJSON(spanAttrs), Level: first(spanAttrs["langfuse.observation.level"], httpFailureLevel(spanAttrs), statusLevel(span.GetStatus().GetCode())), StatusMessage: first(spanAttrs["langfuse.observation.status_message"], store.OptionalValue(span.GetStatus().GetMessage()), errorEventMessage(span.GetEvents())), Version: group.trace.Version, Environment: group.trace.Environment, StartTime: start, EndTime: end, CreatedAt: now()}
				if observation.ID == "" {
					observation.ID = newID("obs")
				}
				group.obs = append(group.obs, observation)
			}
		}
	}
	records := make([]store.IngestRecord, 0)
	for _, traceID := range order {
		group := groups[traceID]
		records = append(records, store.IngestRecord{Type: store.RecordTrace, Trace: &group.trace})
		for index := range group.obs {
			observation := group.obs[index]
			observation.ProjectID = group.trace.ProjectID
			records = append(records, store.IngestRecord{Type: store.RecordObservation, Observation: &observation})
		}
	}
	return records
}

func attrs(values []*commonv1.KeyValue) map[string]string {
	result := make(map[string]string, len(values))
	for _, item := range values {
		if item != nil {
			result[item.GetKey()] = anyString(item.GetValue())
		}
	}
	return result
}

func anyString(value *commonv1.AnyValue) string {
	if value == nil {
		return ""
	}
	switch kind := value.GetValue().(type) {
	case *commonv1.AnyValue_StringValue:
		return kind.StringValue
	case *commonv1.AnyValue_BoolValue:
		return fmt.Sprintf("%t", kind.BoolValue)
	case *commonv1.AnyValue_IntValue:
		return fmt.Sprintf("%d", kind.IntValue)
	case *commonv1.AnyValue_DoubleValue:
		return fmt.Sprintf("%g", kind.DoubleValue)
	case *commonv1.AnyValue_BytesValue:
		return hex.EncodeToString(kind.BytesValue)
	case *commonv1.AnyValue_ArrayValue, *commonv1.AnyValue_KvlistValue:
		return string(marshalRaw(anyNative(value)))
	}
	return ""
}

// anyNative converts an OTLP value to plain Go data so that array and map
// attributes are stored as JSON rather than protobuf debug text.
func anyNative(value *commonv1.AnyValue) any {
	switch kind := value.GetValue().(type) {
	case *commonv1.AnyValue_ArrayValue:
		items := make([]any, 0, len(kind.ArrayValue.GetValues()))
		for _, item := range kind.ArrayValue.GetValues() {
			items = append(items, anyNative(item))
		}
		return items
	case *commonv1.AnyValue_KvlistValue:
		fields := make(map[string]any, len(kind.KvlistValue.GetValues()))
		for _, item := range kind.KvlistValue.GetValues() {
			fields[item.GetKey()] = anyNative(item.GetValue())
		}
		return fields
	case *commonv1.AnyValue_BoolValue:
		return kind.BoolValue
	case *commonv1.AnyValue_IntValue:
		return kind.IntValue
	case *commonv1.AnyValue_DoubleValue:
		return kind.DoubleValue
	}
	return anyString(value)
}

func firstAny(values map[string]string, names ...string) string {
	for _, name := range names {
		if value := values[name]; value != "" {
			return value
		}
	}
	return ""
}
func parseStringOrJSON(value string) any {
	var parsed any
	if json.Unmarshal([]byte(value), &parsed) == nil {
		return parsed
	}
	return value
}
func valueJSON(value string) json.RawMessage {
	if value == "" {
		return nil
	}
	if json.Valid([]byte(value)) {
		return json.RawMessage(value)
	}
	return marshalRaw(value)
}
func marshalRaw(value any) json.RawMessage { raw, _ := json.Marshal(value); return raw }

// promotedAttributes are stored in first-class observation or trace columns.
// Every other attribute is kept in the observation's metadata.
var promotedAttributes = map[string]bool{
	"langfuse.internal.is_app_root":         true,
	"langfuse.observation.type":             true,
	"langfuse.observation.name":             true,
	"langfuse.observation.input":            true,
	"langfuse.observation.output":           true,
	"langfuse.observation.model":            true,
	"langfuse.observation.model.name":       true,
	"langfuse.observation.model.parameters": true,
	"langfuse.observation.usage_details":    true,
	"langfuse.observation.level":            true,
	"langfuse.observation.status_message":   true,
	"langfuse.trace.name":                   true,
	"langfuse.trace.input":                  true,
	"langfuse.trace.output":                 true,
	"gen_ai.input.messages":                 true,
	"gen_ai.output.messages":                true,
	"gen_ai.prompt":                         true,
	"gen_ai.completion":                     true,
}

func attributeMetadata(values map[string]string) map[string]any {
	result := make(map[string]any)
	for key, value := range values {
		switch {
		case promotedAttributes[key], strings.HasPrefix(key, "gen_ai.usage."), strings.HasPrefix(key, "langfuse.trace.metadata."):
			continue
		case key == "langfuse.observation.cost_details":
			result["cost_details"] = parseStringOrJSON(value)
		case strings.HasPrefix(key, "langfuse.observation.metadata."):
			result[strings.TrimPrefix(key, "langfuse.observation.metadata.")] = parseStringOrJSON(value)
		default:
			result[key] = parseStringOrJSON(value)
		}
	}
	return result
}

const (
	maxSpanEvents     = 20
	maxEventValueSize = 2000
)

// spanMetadata is a span's attributes plus its events. Events are where a tracing
// library puts a logged error, whose span otherwise reads as failed for no reason.
func spanMetadata(values map[string]string, events []*oteltrace.Span_Event) map[string]any {
	metadata := attributeMetadata(values)
	if len(events) == 0 {
		return metadata
	}
	stored := make([]map[string]any, 0, min(len(events), maxSpanEvents))
	for _, event := range events[:min(len(events), maxSpanEvents)] {
		fields := map[string]any{"name": event.GetName(), "time": unixNanoTime(event.GetTimeUnixNano())}
		for key, value := range attrs(event.GetAttributes()) {
			fields[key] = clipEventValue(value)
		}
		stored = append(stored, fields)
	}
	metadata["otel.events"] = stored
	if len(events) > maxSpanEvents {
		metadata["otel.events_omitted"] = len(events) - maxSpanEvents
	}
	return metadata
}

func clipEventValue(value string) string {
	if len(value) <= maxEventValueSize {
		return value
	}
	return strings.ToValidUTF8(value[:maxEventValueSize], "") + "..."
}

// errorEventMessage finds why a span failed when its status gives no message: an
// exception event, or a log event at error level, whose name is the logged text.
func errorEventMessage(events []*oteltrace.Span_Event) string {
	for _, event := range events {
		values := attrs(event.GetAttributes())
		if message := store.OptionalValue(values["exception.message"]); message != "" {
			return clipEventValue(message)
		}
		if strings.EqualFold(values["level"], "error") {
			return clipEventValue(first(store.OptionalValue(values["error"]), event.GetName()))
		}
	}
	return ""
}

// usageJSON collects the token usage attributes of a span into canonical keys.
func usageJSON(values map[string]string) json.RawMessage {
	usage := map[string]any{}
	for key, value := range values {
		switch key {
		case "input", "promptTokens", "prompt_tokens",
			"output", "completionTokens", "completion_tokens",
			"total", "totalTokens", "total_tokens",
			"cached_input", "cache_creation_input", "reasoning", "tool_use_prompt":
			usage[key] = parseStringOrJSON(value)
		default:
			if key == "langfuse.observation.usage_details" || strings.HasPrefix(key, "gen_ai.usage.") {
				usage[key] = parseStringOrJSON(value)
			}
		}
	}
	if len(usage) == 0 {
		return nil
	}
	return store.NormalizeUsage(marshalRaw(usage))
}

// observationTypeFor resolves a span's kind: an explicit Langfuse type wins,
// then the GenAI attributes that identify tool calls and model calls.
func observationTypeFor(name string, values map[string]string) string {
	switch explicit := strings.ToUpper(values["langfuse.observation.type"]); explicit {
	case "AGENT", "GENERATION", "SPAN", "EVENT", "TOOL", "CHAIN", "RETRIEVER", "EMBEDDING":
		return explicit
	}
	switch {
	case values["gen_ai.operation.name"] == "execute_tool", values["gen_ai.tool.name"] != "", strings.HasPrefix(name, "execute_tool "), strings.HasPrefix(name, "tools."):
		return "TOOL"
	case values["gen_ai.request.model"] != "", values["gen_ai.response.model"] != "":
		return "GENERATION"
	}
	return "SPAN"
}

// httpFailureLevel marks a server error response. HTTP server spans often leave
// the OTLP status unset, which would otherwise read as a healthy request.
func httpFailureLevel(values map[string]string) string {
	status, err := strconv.Atoi(first(values["http.response.status_code"], values["status_code"]))
	if err == nil && status >= 500 {
		return "ERROR"
	}
	return ""
}

func statusLevel(code oteltrace.Status_StatusCode) string {
	switch code {
	case oteltrace.Status_STATUS_CODE_ERROR:
		return "ERROR"
	case oteltrace.Status_STATUS_CODE_OK:
		return "OK"
	default:
		return "DEFAULT"
	}
}
func unixNanoTime(value uint64) string {
	if value == 0 {
		return ""
	}
	return time.Unix(0, int64(value)).UTC().Format(time.RFC3339Nano)
}
