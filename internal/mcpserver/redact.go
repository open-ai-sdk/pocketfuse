package mcpserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const sensitiveKeys = `(?:proxy[-_])?authorization|x[-_]api[-_]key|api[-_]?key|(?:access|refresh|id|auth|session|bearer)[-_]?token|[A-Za-z0-9_-]+[-_]token|token|(?:client[-_])?secret|secret[-_]?key|private[-_]?key|password|passwd|(?:set[-_])?cookie`

// Spans can hold request headers and credentials. Every tool response passes
// through these patterns, and the count of what they removed is reported, so a
// redaction is never silent. Token counts such as `input_tokens` do not match:
// keys match exactly and only string values are replaced.
var (
	plainKeyPattern   = regexp.MustCompile(`(?i)("(?:` + sensitiveKeys + `)"\s*:\s*")((?:[^"\\]|\\.)+)(")`)
	escapedKeyPattern = regexp.MustCompile(`(?i)(\\"(?:` + sensitiveKeys + `)\\"\s*:\s*\\")([^"]+?)(\\")`)
	headerPairPattern = regexp.MustCompile(`(?i)("(?:name|key)"\s*:\s*"(?:` + sensitiveKeys + `)"\s*,\s*"value"\s*:\s*")((?:[^"\\]|\\.)+)(")`)
	queryPattern      = regexp.MustCompile(`(?i)([?&](?:token|access_token|refresh_token|id_token|api_key|apikey|key|secret|signature|sig|password|x-amz-signature|x-amz-credential|x-amz-security-token)=)([^&"\s\\#]+)()`)
	headerTextPattern = regexp.MustCompile(`(?i)(\b(?:authorization|x-api-key|set-cookie|cookie)\s*:\s*)([^"\\\r\n]{8,})()`)
	schemeTextPattern = regexp.MustCompile(`(?i)(\b(?:bearer|basic)\s+)([A-Za-z0-9\-._~+/]+={0,2})()`)
)

// credentialPatterns match values that are secret by shape alone.
var credentialPatterns = []struct {
	kind    string
	pattern *regexp.Regexp
}{
	{"jwt", regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}(?:\.[A-Za-z0-9_-]+)?`)},
	{"api_key", regexp.MustCompile(`\b(?:sk|pk|rk)-[A-Za-z0-9_-]{16,}`)},
	{"api_key", regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|xox[abpr]-[A-Za-z0-9-]{10,}|AKIA[0-9A-Z]{16}|AIza[0-9A-Za-z_-]{30,})`)},
}

// redactor removes secrets from the text of one tool response and tallies them.
type redactor struct{ counts map[string]int }

func newRedactor() *redactor { return &redactor{counts: map[string]int{}} }

func marker(kind string) string { return "[REDACTED:" + kind + "]" }

// Text returns value without the secrets it recognizes.
func (r *redactor) Text(value string) string {
	value = r.keep(value, plainKeyPattern, "credential_field", nil)
	value = r.keep(value, escapedKeyPattern, "credential_field", nil)
	value = r.keep(value, headerPairPattern, "credential_field", nil)
	value = r.keep(value, queryPattern, "url_credential", nil)
	value = r.keep(value, headerTextPattern, "credential_header", looksLikeToken)
	// Scheme syntax is also valid in prose; prefer privacy over guessing which
	// alphabetic words are credentials.
	value = r.keep(value, schemeTextPattern, "auth_scheme_token", nil)
	for _, credential := range credentialPatterns {
		value = credential.pattern.ReplaceAllStringFunc(value, func(string) string {
			r.counts[credential.kind]++
			return marker(credential.kind)
		})
	}
	return value
}

// keep replaces the second group of a three-group pattern, leaving the rest. An
// accept function can veto a match that is ordinary prose.
func (r *redactor) keep(value string, pattern *regexp.Regexp, kind string, accept func(string) bool) string {
	return pattern.ReplaceAllStringFunc(value, func(match string) string {
		groups := pattern.FindStringSubmatch(match)
		secret := groups[2]
		if strings.Contains(secret, "[REDACTED:") || (accept != nil && !accept(secret)) {
			return match
		}
		r.counts[kind]++
		return groups[1] + marker(kind) + groups[3]
	})
}

// Header prose needs a stronger signal than a credential's scheme prefix.
func looksLikeToken(value string) bool {
	return strings.ContainsAny(value, "0123456789") || len(value) >= 32
}

// Redactions is how a response reports what was removed.
type Redactions struct {
	Count int            `json:"count"`
	Kinds map[string]int `json:"kinds"`
	Note  string         `json:"note"`
}

func (r *redactor) report() *Redactions {
	if len(r.counts) == 0 {
		return nil
	}
	total := 0
	for _, count := range r.counts {
		total += count
	}
	return &Redactions{Count: total, Kinds: r.counts, Note: "Credentials in span data were replaced by [REDACTED:kind] markers."}
}

// fieldResponse keeps already-redacted character windows out of the final
// credential pass. Redacting a partial marker would change its byte content
// without changing the continuation offsets.
type fieldPayload struct {
	name  string
	value *fieldSlice
}

type fieldResponse struct {
	body   any
	fields []fieldPayload
}

func newFieldResponse(body any, fields ...fieldPayload) fieldResponse {
	return fieldResponse{body: body, fields: fields}
}

// fits measures the exact final JSON, including redaction expansion and its
// report, without counting redactions again while trying a smaller window.
func (r *redactor) fits(value any) bool {
	probe := newRedactor()
	for kind, count := range r.counts {
		probe.counts[kind] = count
	}
	_, err := probe.render(value)
	return err == nil
}

// render serializes a tool result, redacts it and appends the redaction report.
// A result over the hard limit becomes an error: the per-tool budgets should
// prevent it, and overflowing a model's context silently would be worse.
func (r *redactor) render(value any) (string, error) {
	var fields []fieldPayload
	if response, ok := value.(fieldResponse); ok {
		value, fields = response.body, response.fields
	}
	text, err := encodeResult(value)
	if err != nil {
		return "", err
	}
	text = r.Text(text)
	if len(fields) > 0 {
		var encoded bytes.Buffer
		encoded.WriteString(strings.TrimSuffix(text, "}"))
		encoder := json.NewEncoder(&encoded)
		encoder.SetEscapeHTML(false)
		for _, field := range fields {
			if field.value == nil {
				continue
			}
			// Names are fixed tool schema properties, never span content.
			encoded.WriteString(`,"` + field.name + `":`)
			if err := encoder.Encode(field.value); err != nil {
				return "", fmt.Errorf("encode field window: %w", err)
			}
		}
		encoded.WriteByte('}')
		text = encoded.String()
	}
	if report := r.report(); report != nil {
		appended, err := json.Marshal(report)
		if err != nil {
			return "", fmt.Errorf("encode redaction report: %w", err)
		}
		text = strings.TrimSuffix(text, "}") + `,"redactions":` + string(appended) + "}"
	}
	if len(text) > hardResponseBytes {
		return "", fmt.Errorf("result of %d bytes exceeds the %d byte limit: narrow the request filters; for trace headers use get_trace field=header, for span payloads use get_observation with field", len(text), hardResponseBytes)
	}
	return text, nil
}

func encodeResult(value any) (string, error) {
	// HTML escaping would hide URL query separators from the patterns.
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return "", fmt.Errorf("encode result: %w", err)
	}
	return strings.TrimSpace(encoded.String()), nil
}
