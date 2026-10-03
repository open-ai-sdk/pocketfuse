# HTTP API

Base URL: `http://127.0.0.1:7625` (default). All endpoints are unauthenticated.

## Endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/health` | Liveness and storage status. |
| `POST` | `/api/ingest` | JSON ingestion — trace, observations, batch, or single record. |
| `POST` | `/api/public/otel/v1/traces` | OTLP/HTTP trace export (protobuf or JSON). |
| `GET` | `/api/traces` | Paginated traces. Filters: `project`, `name`, `from`, `to`, `page`, `limit`. |
| `GET` | `/api/traces/{id}` | One trace with its observations. |
| `GET` | `/api/observations` | Paginated observations. Filters: `traceId`, `type`, `level`, `page`, `limit`. |
| `GET` | `/api/sessions` | Paginated sessions. |
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
