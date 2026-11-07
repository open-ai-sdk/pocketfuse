package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestCorrelationMigrationRepairsUserOnlyAfterVersionTwo(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy-user.db")
	// Build the actual old schema, not a fresh Store with new provenance columns.
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range schemaStatements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			_ = db.Close()
			t.Fatal(err)
		}
	}
	for _, statement := range []string{
		`INSERT INTO traces (id, timestamp) VALUES ('user-only', '2026-01-01T00:03:00Z')`,
		`INSERT INTO traces (id, timestamp, user_id, metadata) VALUES ('explicit', '2026-01-01T00:03:00Z', 'kept', '{"owner":"kept"}')`,
		`INSERT INTO observations (id, trace_id, parent_observation_id, start_time, metadata) VALUES ('user-span', 'user-only', 'missing-root', '2026-01-01T00:01:00Z', '{"user_id":42}')`,
		`INSERT INTO observations (id, trace_id, parent_observation_id, start_time, metadata) VALUES ('explicit-span', 'explicit', 'missing-root', '2026-01-01T00:03:00Z', '{"user_id":"derived"}')`,
		`PRAGMA user_version = 2`,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			_ = db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	// Reopen twice: the corrective migration must run despite v1/v2 already
	// being complete, and its second application must leave explicit values alone.
	for range 2 {
		st, err := Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		func() {
			defer func() { _ = st.Close() }()
			trace, err := st.GetTrace(ctx, "user-only")
			if err != nil {
				t.Fatal(err)
			}
			if trace.UserID != "42" || trace.SessionID != "" {
				t.Errorf("user-only correlation = user %q session %q", trace.UserID, trace.SessionID)
			}
			if trace.Timestamp != "2026-01-01T00:03:00Z" {
				t.Errorf("independent explicit timestamp changed to %q", trace.Timestamp)
			}
			explicit, err := st.GetTrace(ctx, "explicit")
			if err != nil {
				t.Fatal(err)
			}
			if explicit.UserID != "kept" || string(explicit.Metadata) != `{"owner":"kept"}` || explicit.Timestamp != "2026-01-01T00:03:00Z" {
				t.Errorf("explicit values changed: %+v", explicit)
			}
			// A legacy timestamp coincident with a child is still not proof that
			// the child supplied it. Subsequent rootless batches must not replace it.
			if _, err := st.Ingest(ctx, []IngestRecord{
				{Type: RecordTrace, Trace: &Trace{ID: "explicit", Partial: true, Timestamp: "2026-01-01T00:00:00Z"}},
				{Type: RecordObservation, Observation: &Observation{ID: "earlier", TraceID: "explicit", ParentObservationID: "missing-root", StartTime: "2026-01-01T00:00:00Z"}},
			}); err != nil {
				t.Fatal(err)
			}
			timestampRegressionAssertTrace(t, st, "explicit", "2026-01-01T00:03:00Z")
		}()
	}
}

func TestRootlessTimestampTracksEarlierBatchesThenKeepsRealRoot(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "rootless.db")
	st, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	ingest := func(trace Trace, observations ...Observation) {
		t.Helper()
		records := []IngestRecord{{Type: RecordTrace, Trace: &trace}}
		for i := range observations {
			records = append(records, IngestRecord{Type: RecordObservation, Observation: &observations[i]})
		}
		if _, err := st.Ingest(ctx, records); err != nil {
			t.Fatal(err)
		}
	}
	ingest(Trace{ID: "rootless", Partial: true, Timestamp: "2026-01-01T00:05:00Z"},
		Observation{ID: "late", TraceID: "rootless", ParentObservationID: "root", StartTime: "2026-01-01T00:05:00Z"})
	// The batch's first child is not necessarily its earliest child either.
	ingest(Trace{ID: "rootless", Partial: true, Timestamp: "2026-01-01T00:04:00Z"},
		Observation{ID: "middle", TraceID: "rootless", ParentObservationID: "root", StartTime: "2026-01-01T00:04:00Z"},
		Observation{ID: "early", TraceID: "rootless", ParentObservationID: "root", StartTime: "2026-01-01T00:01:00Z"})
	timestampRegressionAssertTrace(t, st, "rootless", "2026-01-01T00:01:00Z")
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	ingest(Trace{ID: "rootless", Partial: true, Timestamp: "2026-01-01T00:00:00Z"},
		Observation{ID: "earliest", TraceID: "rootless", ParentObservationID: "root", StartTime: "2026-01-01T00:00:00Z"})
	timestampRegressionAssertTrace(t, st, "rootless", "2026-01-01T00:00:00Z")
	// The root is authoritative even when clock skew puts a child before it.
	ingest(Trace{ID: "rootless", Name: "real root", Timestamp: "2026-01-01T00:02:00Z", StartTime: "2026-01-01T00:02:00Z"},
		Observation{ID: "root", TraceID: "rootless", StartTime: "2026-01-01T00:02:00Z"})
	ingest(Trace{ID: "rootless", Partial: true, Timestamp: "2025-12-31T23:59:00Z"},
		Observation{ID: "skewed", TraceID: "rootless", ParentObservationID: "root", StartTime: "2025-12-31T23:59:00Z", Metadata: json.RawMessage(`{"user_id":"u"}`)})
	timestampRegressionAssertTrace(t, st, "rootless", "2026-01-01T00:02:00Z")
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	timestampRegressionAssertTrace(t, st, "rootless", "2026-01-01T00:02:00Z")
}

func TestRootlessTimestampComparesFractionalSecondsChronologically(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "fractions.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	if _, err := st.Ingest(ctx, []IngestRecord{
		{Type: RecordTrace, Trace: &Trace{ID: "fractions", Partial: true, Timestamp: "2026-01-01T00:00:00.5Z"}},
		{Type: RecordObservation, Observation: &Observation{ID: "f1", TraceID: "fractions", ParentObservationID: "root", StartTime: "2026-01-01T00:00:00Z"}},
	}); err != nil {
		t.Fatal(err)
	}
	timestampRegressionAssertTrace(t, st, "fractions", "2026-01-01T00:00:00Z")
}

func timestampRegressionAssertTrace(t *testing.T, st *Store, id, want string) {
	t.Helper()
	ctx := context.Background()
	trace, err := st.GetTrace(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if trace.Timestamp != want {
		t.Errorf("detail timestamp = %q, want %q", trace.Timestamp, want)
	}
	page, err := st.ListTraces(ctx, TraceFilter{From: want, To: want})
	if err != nil {
		t.Fatal(err)
	}
	for _, listed := range page.Data {
		if listed.ID == id {
			if listed.Timestamp != want {
				t.Errorf("listed timestamp = %q, want %q", listed.Timestamp, want)
			}
			return
		}
	}
	t.Errorf("trace %q absent from exact timestamp filter %q: %+v", id, want, page)
}
