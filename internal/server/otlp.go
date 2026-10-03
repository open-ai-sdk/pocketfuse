package server

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/open-ai-sdk/duckscope/internal/store"
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
		trace store.Trace
		obs   []store.Observation
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
					group = &traceGroup{trace: store.Trace{ID: traceID, ProjectID: project, Project: project, CreatedAt: now()}}
					groups[traceID] = group
					order = append(order, traceID)
				}
				spanAttrs := attrs(span.GetAttributes())
				start := unixNanoTime(span.GetStartTimeUnixNano())
				end := unixNanoTime(span.GetEndTimeUnixNano())
				isRoot := strings.EqualFold(spanAttrs["langfuse.internal.is_app_root"], "true") || len(span.GetParentSpanId()) == 0
				if group.trace.Name == "" || isRoot {
					group.trace.Name = first(spanAttrs["langfuse.trace.name"], span.GetName(), group.trace.Name)
				}
				group.trace.ProjectID = first(spanAttrs["langfuse.project.id"], project, group.trace.ProjectID)
				group.trace.Project = group.trace.ProjectID
				group.trace.UserID = first(spanAttrs["langfuse.trace.user_id"], spanAttrs["user.id"], group.trace.UserID)
				group.trace.SessionID = first(spanAttrs["langfuse.trace.session_id"], spanAttrs["session.id"], group.trace.SessionID)
				group.trace.Release = first(spanAttrs["langfuse.release"], resourceAttrs["service.version"], group.trace.Release)
				group.trace.Version = first(spanAttrs["langfuse.version"], group.trace.Version)
				group.trace.Environment = first(spanAttrs["langfuse.environment"], resourceAttrs["deployment.environment"], group.trace.Environment)
				group.trace.Timestamp = first(group.trace.Timestamp, start)
				if isRoot {
					group.trace.StartTime = first(start, group.trace.StartTime)
					group.trace.EndTime = first(end, group.trace.EndTime)
					group.trace.Input = valueJSON(firstAny(spanAttrs, "langfuse.trace.input", "langfuse.observation.input"))
					group.trace.Output = valueJSON(firstAny(spanAttrs, "langfuse.trace.output", "langfuse.observation.output"))
				}
				metadata := make(map[string]any)
				for key, value := range spanAttrs {
					const prefix = "langfuse.trace.metadata."
					if strings.HasPrefix(key, prefix) {
						metadata[strings.TrimPrefix(key, prefix)] = parseStringOrJSON(value)
					}
				}
				if len(metadata) > 0 {
					group.trace.Metadata = marshalRaw(metadata)
				}
				observationType := strings.ToUpper(first(spanAttrs["langfuse.observation.type"], ""))
				if observationType == "" {
					if spanAttrs["gen_ai.request.model"] != "" || spanAttrs["gen_ai.response.model"] != "" {
						observationType = "GENERATION"
					} else {
						observationType = "SPAN"
					}
				}
				if observationType == "AGENT" || observationType == "GENERATION" || observationType == "SPAN" || observationType == "EVENT" { /* keep */
				} else {
					observationType = "SPAN"
				}
				observation := store.Observation{ID: hex.EncodeToString(span.GetSpanId()), TraceID: traceID, ProjectID: group.trace.ProjectID, ParentObservationID: hex.EncodeToString(span.GetParentSpanId()), Type: observationType, Name: first(spanAttrs["langfuse.observation.name"], span.GetName()), Input: valueJSON(firstAny(spanAttrs, "langfuse.observation.input")), Output: valueJSON(firstAny(spanAttrs, "langfuse.observation.output")), Metadata: marshalRaw(attributeMetadata(spanAttrs)), Model: first(spanAttrs["langfuse.observation.model"], spanAttrs["gen_ai.request.model"], spanAttrs["gen_ai.response.model"]), Usage: usageJSON(spanAttrs), Level: first(spanAttrs["langfuse.observation.level"], statusLevel(span.GetStatus().GetCode())), StatusMessage: first(spanAttrs["langfuse.observation.status_message"], span.GetStatus().GetMessage()), Version: group.trace.Version, Environment: group.trace.Environment, StartTime: start, EndTime: end, CreatedAt: now()}
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
	case *commonv1.AnyValue_ArrayValue:
		return fmt.Sprintf("%v", kind.ArrayValue)
	case *commonv1.AnyValue_KvlistValue:
		return fmt.Sprintf("%v", kind.KvlistValue)
	}
	return ""
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
func attributeMetadata(values map[string]string) map[string]any {
	result := make(map[string]any)
	for key, value := range values {
		if strings.HasPrefix(key, "langfuse.") || strings.HasPrefix(key, "gen_ai.") {
			continue
		}
		result[key] = parseStringOrJSON(value)
	}
	return result
}
func usageJSON(values map[string]string) json.RawMessage {
	usage := map[string]any{}
	for key, value := range values {
		if strings.Contains(key, "usage") || strings.Contains(key, "token") {
			usage[key] = parseStringOrJSON(value)
		}
	}
	if len(usage) == 0 {
		return nil
	}
	return marshalRaw(usage)
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
