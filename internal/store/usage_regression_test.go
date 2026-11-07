package store

import (
	"context"
	"encoding/json"
	"math"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNormalizeUsageDeterministicAliasPrecedence(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "preferred OTel spellings",
			raw:  `{"gen_ai.usage.input_tokens":11,"gen_ai.usage.prompt_tokens":99,"gen_ai.usage.output_tokens":7,"gen_ai.usage.completion_tokens":88,"gen_ai.usage.reasoning.output_tokens":3,"gen_ai.usage.reasoning_tokens":77}`,
			want: `{"input":11,"output":7,"reasoning":3}`,
		},
		{
			name: "explicit canonical keys",
			raw:  `{"input":5,"output":6,"reasoning":2,"gen_ai.usage.input_tokens":11,"gen_ai.usage.prompt_tokens":99,"gen_ai.usage.output_tokens":7,"gen_ai.usage.reasoning_tokens":77}`,
			want: `{"input":5,"output":6,"reasoning":2}`,
		},
		{
			name: "Langfuse details override canonical and OTel",
			raw:  `{"input":5,"output":6,"gen_ai.usage.input_tokens":11,"gen_ai.usage.prompt_tokens":99,"gen_ai.usage.output_tokens":7,"langfuse.observation.usage_details":{"input":0,"output":2}}`,
			want: `{"input":0,"output":2}`,
		},
		{
			name: "string Langfuse details retain precedence",
			raw:  `{"input":5,"gen_ai.usage.input_tokens":11,"gen_ai.usage.prompt_tokens":99,"langfuse.observation.usage_details":"{\"input\":0}"}`,
			want: `{"input":0}`,
		},
		{
			name: "legacy only and unknown fields survive",
			raw:  `{"gen_ai.usage.prompt_tokens":4,"gen_ai.usage.completion_tokens":2,"custom":9}`,
			want: `{"custom":9,"input":4,"output":2}`,
		},
	}
	for _, current := range cases {
		t.Run(current.name, func(t *testing.T) {
			var want map[string]any
			if err := json.Unmarshal([]byte(current.want), &want); err != nil {
				t.Fatal(err)
			}
			for iteration := range 1024 {
				raw := NormalizeUsage(json.RawMessage(current.raw))
				var got map[string]any
				if err := json.Unmarshal(raw, &got); err != nil {
					t.Fatalf("normalized usage is invalid JSON: %v", err)
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("iteration %d: got %v, want %v", iteration, got, want)
				}
			}
		})
	}
}

func TestUsageNumbersRejectNonFiniteAndPreserveZero(t *testing.T) {
	for _, value := range []any{"NaN", "+Inf", "-Inf", "Infinity", math.NaN(), math.Inf(1), math.Inf(-1)} {
		if got, ok := numberIn(map[string]any{"input": value}, "input"); ok {
			t.Errorf("numberIn(%v) = %v, reported; want unreported", value, got)
		}
		if got, ok := numberIn(map[string]any{"input": value, "prompt_tokens": "0"}, "input", "prompt_tokens"); !ok || got != 0 {
			t.Errorf("finite alias fallback after %v = %v, %v; want 0, true", value, got, ok)
		}
	}
	for _, raw := range []string{
		`{"total":"NaN","input":"+Inf","output":"-Inf"}`,
		`{"input":"NaN","output":"+Inf"}`,
	} {
		usage := readUsage(json.RawMessage(raw))
		if usage.Input != nil || usage.Output != nil || usage.Total != nil {
			t.Errorf("readUsage(%s) = %+v, want unknown", raw, usage)
		}
		if _, err := json.Marshal(usage); err != nil {
			t.Errorf("usage is not JSON safe: %v", err)
		}
	}
	usage := readUsage(json.RawMessage(`{"total":"NaN","input":"0","output":"+Inf"}`))
	if usage.Input == nil || *usage.Input != 0 || usage.Total == nil || *usage.Total != 0 || usage.Output != nil {
		t.Errorf("finite zero/fallback usage = %+v", usage)
	}
	for _, raw := range []string{
		`{"cost_details":{"total":"NaN","input":"+Inf","output":"-Inf"}}`,
		`{"cost_details":{"input":"NaN"}}`,
	} {
		if cost := metadataCost(json.RawMessage(raw)); cost != nil {
			t.Errorf("metadataCost(%s) = %v, want unknown", raw, *cost)
		}
	}
	if cost := metadataCost(json.RawMessage(`{"cost_details":{"total":"NaN","input":"0","output":"+Inf"}}`)); cost == nil || *cost != 0 {
		t.Errorf("finite zero cost fallback = %v", cost)
	}
}

func TestTraceTotalsUseModelCallIdentity(t *testing.T) {
	usage := json.RawMessage(`{"input":5,"output":6}`)
	cost := 0.25
	pair := func() []Observation {
		return []Observation{
			{ID: "wrapper", Type: "GENERATION", Model: "m", Usage: usage, Cost: &cost},
			{ID: "provider", ParentObservationID: "wrapper", Type: "GENERATION", Model: "m", Usage: usage, Cost: &cost},
		}
	}
	cases := []struct {
		name   string
		change func([]Observation) []Observation
		tokens float64
		cost   float64
	}{
		{"duplicate once", func(obs []Observation) []Observation { return obs }, 11, 0.25},
		{"parent missing cost", func(obs []Observation) []Observation { obs[0].Cost = nil; return obs }, 11, 0.25},
		{"child missing cost", func(obs []Observation) []Observation { obs[1].Cost = nil; return obs }, 11, 0.25},
		{"different model", func(obs []Observation) []Observation { obs[1].Model = "other"; return obs }, 22, 0.5},
		{"different usage", func(obs []Observation) []Observation {
			obs[1].Usage = json.RawMessage(`{"input":5,"output":7}`)
			return obs
		}, 23, 0.5},
		{"independent payload", func(obs []Observation) []Observation {
			obs[1].Input = json.RawMessage(`"separate request"`)
			return obs
		}, 22, 0.5},
		{"multiple children", func(obs []Observation) []Observation {
			return append(obs, Observation{ID: "tool", ParentObservationID: "wrapper", Type: "TOOL"})
		}, 22, 0.5},
	}
	for _, current := range cases {
		t.Run(current.name, func(t *testing.T) {
			var trace Trace
			summarizeTrace(&trace, current.change(pair()))
			if trace.TotalTokens == nil || *trace.TotalTokens != current.tokens {
				t.Errorf("tokens = %v, want %v", trace.TotalTokens, current.tokens)
			}
			if trace.TotalCost == nil || *trace.TotalCost != current.cost {
				t.Errorf("cost = %v, want %v", trace.TotalCost, current.cost)
			}
		})
	}
}

func TestStoredTraceUsageListAndDetailAgree(t *testing.T) {
	ctx := context.Background()
	st, err := New(filepath.Join(t.TempDir(), "usage-regressions.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cases := []struct {
		id           string
		child        Observation
		want         float64
		wantCost     float64
		noParentCost bool
		noChildCost  bool
	}{
		{id: "duplicate", want: 11, wantCost: 0.25},
		{id: "payload", child: Observation{Input: json.RawMessage(`"independent request"`)}, want: 22, wantCost: 0.5},
		{id: "model", child: Observation{Model: "other"}, want: 22, wantCost: 0.5},
		{id: "parent-no-cost", want: 11, wantCost: 0.25, noParentCost: true},
		{id: "child-no-cost", want: 11, wantCost: 0.25, noChildCost: true},
		{id: "nonfinite"},
	}
	for _, current := range cases {
		usage := json.RawMessage(`{"gen_ai.usage.input_tokens":5,"gen_ai.usage.prompt_tokens":99,"gen_ai.usage.output_tokens":6}`)
		metadata := json.RawMessage(`{"cost_details":{"total":0.25}}`)
		if current.id == "nonfinite" {
			usage = json.RawMessage(`{"input":"NaN","output":"+Inf"}`)
			metadata = json.RawMessage(`{"cost_details":{"total":"NaN"}}`)
		}
		parent := Observation{ID: current.id + "-parent", TraceID: current.id, Type: "GENERATION", Model: "m", Usage: usage, Metadata: metadata}
		child := current.child
		child.ID, child.TraceID, child.ParentObservationID, child.Type = current.id+"-child", current.id, parent.ID, "GENERATION"
		if child.Model == "" {
			child.Model = "m"
		}
		child.Usage, child.Metadata = usage, metadata
		if current.noParentCost {
			parent.Metadata = nil
		}
		if current.noChildCost {
			child.Metadata = nil
		}
		_, err := st.Ingest(ctx, []IngestRecord{
			{Type: RecordTrace, Trace: &Trace{ID: current.id, Timestamp: "2026-01-01T00:00:00Z"}},
			{Type: RecordObservation, Observation: &parent},
			{Type: RecordObservation, Observation: &child},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	page, err := st.ListTraces(ctx, TraceFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := json.Marshal(page); err != nil {
		t.Errorf("trace list is not JSON safe: %v", err)
	}
	for _, current := range cases {
		detail, err := st.GetTrace(ctx, current.id)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := json.Marshal(detail); err != nil {
			t.Errorf("trace %s is not JSON safe: %v", current.id, err)
		}
		var listed *Trace
		for index := range page.Data {
			if page.Data[index].ID == current.id {
				listed = &page.Data[index]
				break
			}
		}
		if listed == nil {
			t.Fatalf("trace %s missing from list", current.id)
		}
		for _, trace := range []*Trace{detail, listed} {
			if current.id == "nonfinite" {
				if trace.TotalTokens != nil || trace.TotalCost != nil {
					t.Errorf("nonfinite totals should be unknown: %+v", trace)
				}
				continue
			}
			if trace.TotalTokens == nil || *trace.TotalTokens != current.want || trace.TotalCost == nil || *trace.TotalCost != current.wantCost {
				t.Errorf("%s totals = tokens %v, cost %v; want %v, %v", current.id, trace.TotalTokens, trace.TotalCost, current.want, current.wantCost)
			}
		}
	}
}
