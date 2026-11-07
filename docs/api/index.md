# HTTP API

Base URL: `http://127.0.0.1:7625` (default). All endpoints are unauthenticated.

## Endpoints

| Method | Path | Description |
|---|---|---|
| `GET`, `HEAD` | `/api/health` | Liveness and storage status. |
| `POST` | `/api/ingest` | JSON ingestion — trace, observations, batch, or single record. |
| `POST` | `/api/public/otel/v1/traces` | OTLP/HTTP trace export (protobuf or JSON). |
| `GET` | `/api/traces` | Paginated traces with status, tokens, cost and models when known. Filters: `project`, `name` (also matches an id prefix), `sessionId`, `userId`, `noise=hide` (leaves out polling and queue housekeeping), `from`, `to`, `page`, `limit`. |
| `GET` | `/api/traces/{id}` | One trace with its observations, each with a `role` and `label`, and its `turn` when it holds one. |
| `POST` | `/mcp` | [MCP endpoint](/guide/mcp) for AI agents. Read-only. |
| `GET` | `/api/traces/{id}/related` | `sameSession` (linked by id) and `overlapping` (ran at the same time, not linked). |
| `GET` | `/api/observations` | Paginated observations. Filters: `traceId`, `type`, `level`, `page`, `limit`. |
| `GET` | `/api/sessions` | Paginated sessions with trace counts. |
| `GET` | `/api/sessions/{id}/story` | One session read as a conversation: `totals`, and `items` holding each turn with its steps and the requests between turns. Accepts `noise=hide`. |
| `GET` | `/api/sessions/{id}` | One session's timeline: `traces`, plus `unlinked` traces that only match by time. Accepts `noise=hide`. |
| `GET` | `/api/scores` | Paginated scores. |

List responses are `{data: [...], total, page, limit}`. Writes return
`202 {accepted, traces, observations, sessions, scores}` for `/api/ingest` or
`200 {partialSuccess, accepted}` for OTLP.

## Examples

Health:

```sh
curl http://127.0.0.1:7625/api/health
# {"status":"ok","storage":"sqlite","version":"0.1.0"}
```

List traces of one project:

```sh
curl 'http://127.0.0.1:7625/api/traces?project=demo&limit=10'
```

Ingest a score on an existing trace:

```sh
curl -X POST http://127.0.0.1:7625/api/ingest \
  -H 'content-type: application/json' \
  -d '{"name":"accuracy","value":0.87,"traceId":"hello-1","dataType":"NUMERIC"}'
```
