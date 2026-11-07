package mcpserver

import (
	"encoding/json"
	"strings"
	"testing"
)

// A character window can end inside a redaction marker. Serializing that
// window must not redact it a second time or change its continuation offsets.
func TestFieldWindowsPreservePartialRedactionMarkers(t *testing.T) {
	for _, name := range []string{"header", "input", "output", "metadata", "modelParameters"} {
		t.Run(name, func(t *testing.T) {
			filter := newRedactor()
			text := []rune(filter.Text(`{"url":"https://example.test/?token=credential-value"}`))
			var got strings.Builder
			for offset := 0; offset < len(text); {
				window := sliceField(text, offset, 9, false)
				value := newFieldResponse(struct {
					Name string `json:"name"`
				}{Name: "Bearer abcdef0123456789"}, fieldPayload{name: name, value: window})
				encoded, err := filter.render(value)
				if err != nil {
					t.Fatal(err)
				}
				var response map[string]json.RawMessage
				if err := json.Unmarshal([]byte(encoded), &response); err != nil {
					t.Fatal(err)
				}
				var field fieldSlice
				if err := json.Unmarshal(response[name], &field); err != nil {
					t.Fatal(err)
				}
				if field.Text != string(text[offset:min(offset+9, len(text))]) {
					t.Fatalf("window at %d changed: %q", offset, field.Text)
				}
				if strings.Contains(string(response["name"]), "abcdef0123456789") {
					t.Fatal("envelope credential was not redacted")
				}
				got.WriteString(field.Text)
				if field.NextOffset == nil {
					break
				}
				offset = *field.NextOffset
			}
			if got.String() != string(text) {
				t.Fatalf("continuation changed the redacted source: %q", got.String())
			}
		})
	}
}
