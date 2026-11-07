# Ingesting traces

Pocketfuse accepts two ingestion paths.

## OTLP/HTTP

```
POST {base}/api/public/otel/v1/traces
Content-Type: application/x-protobuf   # or application/json (OTLP JSON)
```

Point any OpenTelemetry exporter at it:

```sh
export OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=http://127.0.0.1:7625/api/public/otel/v1/traces
export OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
export OTEL_SERVICE_NAME=my-agent
```

`service.name` becomes the **project** in the UI. Authentication headers are
accepted but ignored.

### Attribute mapping

Span attributes that follow common LLM-tracing conventions (`gen_ai.*` and
related OTLP exporter vocabularies) are promoted onto trace and observation
fields: name, input/output, model, usage, level. Everything else is preserved
as observation metadata. Spans with `gen_ai.request.model` /
`gen_ai.response.model` become `GENERATION` observations automatically.

## JSON ingestion

```
POST /api/ingest
```

Accepts three shapes:

- `{trace: {...}, observations: [...]}` — a trace plus children
- `{batch: [{type|eventType, ...}, ...]}` — mixed records; `trace*`, `span|generation|event|agent*`, `score*`, `session*` type prefixes route each record
- a single record — `traceId` present → observation; `name`+`value`+`traceId` → score; bare `id` → trace

IDs and timestamps are optional — the server generates them (`trace-…`,
`obs-…`, `score-…`, `session-…`). Existing IDs upsert. Traces carrying
`sessionId` auto-create the session. Bodies may be gzipped; the limit is
32 MiB.

## Linking traces into a session

An OTLP trace joins a session through the first of `langfuse.trace.session_id`,
`langfuse.session.id` or `session.id`. Without those, a `thread_id` attribute or
a `{thread_id}` parameter in `http.route` matched against `url.path` places it in
session `thread:<id>`. Traces with no such attribute stay unlinked; the UI can
only show them as time matches, labelled as such.

## What senders can attach

A span with a failed status and no message takes its message from its events: an
`exception.message`, or the text of a log event at `ERROR` level. Up to 20 events
are kept under `otel.events` in the span's metadata.

See [Reading traces](/guide/reading-traces) for how span names and payloads become
turns.
