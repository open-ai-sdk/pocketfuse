package mcpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/open-ai-sdk/pocketfuse/internal/store"
)

func TestToolErrorsBoundSerializedContent(t *testing.T) {
	session, _ := connect(t, "")
	for _, unit := range []string{"x", "<>&\"\\\n\t\x00", "界😀"} {
		for _, target := range []struct {
			tool string
			key  string
		}{
			{"get_trace", "trace_id"},
			{"get_observation", "observation_id"},
		} {
			t.Run(target.tool+"/"+unit, func(t *testing.T) {
				id := "Bearer abcdefghijklmno " + strings.Repeat(unit, max(20_000, 40_000/len(unit)))
				result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
					Name: target.tool, Arguments: map[string]any{target.key: id},
				})
				if err != nil {
					t.Fatalf("protocol error instead of tool error: %v", err)
				}
				if !result.IsError || len(result.Content) != 1 {
					t.Fatalf("expected one error content block, got %+v", result)
				}
				text := result.Content[0].(*mcp.TextContent).Text
				encoded, err := json.Marshal(result)
				if err != nil {
					t.Fatal(err)
				}
				native, err := json.Marshal(&mcp.CallToolResult{IsError: true, Content: result.Content})
				if err != nil {
					t.Fatal(err)
				}
				t.Logf("native handler shape=%d bytes; SDK result=%d bytes; injected envelope=%d bytes; meta=%v", len(native), len(encoded), len(encoded)-len(native), result.Meta)
				if len(encoded) > hardResponseBytes {
					t.Fatalf("serialized error = %d bytes, limit %d", len(encoded), hardResponseBytes)
				}
				if !utf8.ValidString(text) || !strings.Contains(text, "[Truncated:") {
					t.Fatalf("error lost UTF-8 validity or truncation notice: %q", text)
				}
				if strings.Contains(text, "abcdefghijklmno") || !strings.Contains(text, "[REDACTED:") {
					t.Fatal("error must redact credentials before truncating")
				}
			})
		}
	}
}

func TestToolErrorClippingPreservesRedactedSource(t *testing.T) {
	session, _ := connect(t, "")
	const notice = "\n[Truncated: response byte limit reached; shorten the request.]"
	sample, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "get_trace", Arguments: map[string]any{"trace_id": "missing"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Keep the SDK's actual protocol result type and server metadata in the
	// template so this deliberately places the cut inside a redaction marker.
	sample.Content = []mcp.Content{&mcp.TextContent{Text: `trace " Bearer [REDA` + notice}}
	template, err := json.Marshal(sample)
	if err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("x", hardResponseBytes-len(template)) + " Bearer abcdefghijklmno " + strings.Repeat("y", 1000)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "get_trace", Arguments: map[string]any{"trace_id": id},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected an explicit tool error")
	}
	text := result.Content[0].(*mcp.TextContent).Text
	prefix, found := strings.CutSuffix(text, notice)
	want := newRedactor().Text(notFound("trace", id, sql.ErrNoRows).Error())
	if !found || !strings.HasPrefix(want, prefix) || !strings.HasSuffix(prefix, "[REDA") {
		t.Fatal("clipping changed the already-redacted source")
	}
}

func TestObservationRedactsAlphabeticSchemesAndTokenSuffixFields(t *testing.T) {
	st := seedStore(t)
	at := time.Now().UTC().Format(time.RFC3339Nano)
	metadata := map[string]any{
		"bearer":              "Bearer abcdefghijklmno",
		"short_bearer":        "bearer abc",
		"basic":               "Basic YWJjZA==",
		"ambiguous_scheme":    "Bearer authentication",
		"service_token":       "ordinarysecretvalue",
		"service-token":       "anothersecretvalue",
		"nested":              map[string]string{"custom_service_token": "nestedsecretvalue"},
		"encoded":             `{"custom-token":"escapedsecretvalue"}`,
		"header":              map[string]string{"name": "service_token", "value": "headersecretvalue"},
		"input_tokens":        15749,
		"output_tokens":       76,
		"cached_input_tokens": 12,
		"service_token_count": 3,
		"prose":               "authorization: required for this endpoint",
	}
	if _, err := st.Ingest(context.Background(), []store.IngestRecord{
		{Type: store.RecordObservation, Observation: &store.Observation{
			ID: "token-regression", TraceID: "t1", Name: "work", Type: "SPAN", StartTime: at, EndTime: at, Metadata: rawValue(t, metadata),
		}},
	}); err != nil {
		t.Fatal(err)
	}
	session, _ := connectStore(t, st, "")
	_, response := call(t, session, "get_observation", map[string]any{"observation_id": "token-regression", "field": "metadata"})
	text := response["metadata"].(map[string]any)["text"].(string)
	for _, secret := range []string{"abcdefghijklmno", "bearer abc", "YWJjZA==", "ordinarysecretvalue", "anothersecretvalue", "nestedsecretvalue", "escapedsecretvalue", "headersecretvalue"} {
		if strings.Contains(text, secret) {
			t.Errorf("metadata leaked %q", secret)
		}
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(text), &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"input_tokens", "output_tokens", "cached_input_tokens", "service_token_count", "prose"} {
		want := metadata[key]
		if number, ok := want.(int); ok {
			want = float64(number)
		}
		if decoded[key] != want {
			t.Errorf("noncredential %s changed: got %v, want %v", key, decoded[key], want)
		}
	}
	if decoded["ambiguous_scheme"] != "Bearer [REDACTED:auth_scheme_token]" {
		t.Errorf("scheme-prefixed alphabetic credential was not masked: %v", decoded["ambiguous_scheme"])
	}
	if response["redactions"] == nil {
		t.Fatal("redacted metadata omitted its redaction report")
	}
}

func TestMCPNodePagesClampMaximumIntegerOffsets(t *testing.T) {
	session, _ := connect(t, "")
	for _, target := range []struct {
		tool string
		key  string
		id   string
		rows string
	}{
		{"get_trace", "trace_id", "t1", "nodes"},
		{"get_session_timeline", "session_id", "thread:9", "items"},
	} {
		t.Run(target.tool, func(t *testing.T) {
			_, response := call(t, session, target.tool, map[string]any{
				target.key: target.id, "offset": 2_147_483_647, "limit": 30,
			})
			if response["returned"] != float64(0) || response["nextOffset"] != nil || len(response[target.rows].([]any)) != 0 {
				t.Fatalf("offset beyond the end should return an empty terminal page: %+v", response)
			}
		})
	}
}

func TestTypedNodePagesClampNativeMaximumIntegerOffsets(t *testing.T) {
	toolset := &tools{store: seedStore(t)}
	offset := int(^uint(0) >> 1)
	for _, target := range []struct {
		name string
		rows string
		call func(*redactor) (any, error)
	}{
		{"get_trace", "nodes", func(filter *redactor) (any, error) {
			return toolset.getTrace(context.Background(), getTraceInput{TraceID: "t1", Offset: offset, Limit: 30}, filter)
		}},
		{"get_session_timeline", "items", func(filter *redactor) (any, error) {
			return toolset.getSessionTimeline(context.Background(), sessionTimelineInput{SessionID: "thread:9", Offset: offset, Limit: 30}, filter)
		}},
	} {
		t.Run(target.name, func(t *testing.T) {
			filter := newRedactor()
			value, err := target.call(filter)
			if err != nil {
				t.Fatal(err)
			}
			text, err := filter.render(value)
			if err != nil {
				t.Fatal(err)
			}
			var response map[string]any
			if err := json.Unmarshal([]byte(text), &response); err != nil {
				t.Fatal(err)
			}
			if response["returned"] != float64(0) || response["nextOffset"] != nil || len(response[target.rows].([]any)) != 0 {
				t.Fatalf("native maximum offset should return an empty terminal page: %+v", response)
			}
		})
	}
}
