# Getting started

Duckscope is a local tracing UI for AI agents and LLM applications. It runs as
one Go process, serves an embedded React SPA, and stores everything in a single
DuckDB file. There is **no authentication** — use it on localhost or trusted
networks only.

## Requirements

- Go 1.25+ with CGO (a C compiler — required by the DuckDB driver)
- Node.js 22+ and pnpm (frontend build only)

## Run locally

```sh
cp .env.example .env
mkdir -p data
set -a; . ./.env; set +a
make run
```

Open <http://127.0.0.1:3825>.

`make run` builds `web/dist` first so the Go `go:embed` step always ships the
current frontend. For a standalone binary:

```sh
make build
./bin/duckscope
```

## Run with Docker

```sh
mkdir -p data
make docker-up     # builds web/dist, then docker compose up --build
```

Data persists in `./data` (bind-mounted to `/data` in the container).

## Send your first trace

```sh
curl -fsS -X POST http://127.0.0.1:3825/api/ingest \
  -H 'content-type: application/json' \
  -d '{
    "trace": {
      "id": "hello-1",
      "projectId": "demo",
      "name": "hello.trace",
      "input": {"hello": "world"}
    },
    "observations": [
      {"traceId": "hello-1", "type": "GENERATION", "name": "gen", "model": "demo-model"}
    ]
  }'
```

Then open **Traces** in the UI — `hello.trace` appears under project `demo`.
