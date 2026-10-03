# Pocketfuse

A small, local tracing UI for AI agents and LLM applications. One Go process
serves the embedded React SPA and stores everything in a single SQLite file —
no auth, no external database, built for local development and trusted
networks.

Repository: <https://github.com/open-ai-sdk/pocketfuse>
Docs: <https://open-ai-sdk.github.io/pocketfuse/>

## Quick start

Requires Go 1.26+ and Node 22+ with pnpm. No C toolchain — the SQLite driver
is pure Go.

```sh
make run          # builds web/dist, compiles, runs on http://127.0.0.1:7625
```

Or run the published Docker image:

```sh
mkdir -p data
docker compose up -d
```

Send your first trace:

```sh
curl -fsS -X POST http://127.0.0.1:7625/api/ingest \
  -H 'content-type: application/json' \
  -d '{"trace":{"id":"hello-1","projectId":"demo","name":"hello.trace"},"observations":[{"traceId":"hello-1","type":"GENERATION","name":"gen","model":"demo-model"}]}'
```

Open <http://127.0.0.1:7625> — the trace appears under project `demo`.

## Documentation

Everything else lives in the [docs site](https://open-ai-sdk.github.io/pocketfuse/)
(sources in [`docs/`](./docs), local preview with `make docs-dev`):

- [Getting started](https://open-ai-sdk.github.io/pocketfuse/guide/getting-started) —
  run options, first traces
- [Configuration](https://open-ai-sdk.github.io/pocketfuse/guide/configuration) —
  `POCKETFUSE_*` environment variables
- [Docker & Compose](https://open-ai-sdk.github.io/pocketfuse/guide/docker) —
  published images, version pinning, local builds
- [Ingesting traces](https://open-ai-sdk.github.io/pocketfuse/guide/ingestion) —
  OpenTelemetry/OTLP and the JSON API
- [HTTP API](https://open-ai-sdk.github.io/pocketfuse/api/) — endpoint reference
- [Development](https://open-ai-sdk.github.io/pocketfuse/guide/development) —
  live reload, checks, project layout
