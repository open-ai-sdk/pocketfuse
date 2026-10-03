# Configuration

| Variable | Default | Purpose |
|---|---|---|
| `POCKETFUSE_DB_PATH` | `./pocketfuse.db` | SQLite file path. In Compose use a path under `/data`. `:memory:` also works. |
| `POCKETFUSE_HOST` | `127.0.0.1` | Bind address. Use `0.0.0.0` in containers or for remote access. |
| `POCKETFUSE_PORT` | `7625` | HTTP port (`POCK` on a phone keypad). |
| `POCKETFUSE_DATA_DIR` | `./data` | Host directory bind-mounted to `/data` by Compose. |

The Go process does not read `.env` itself — export variables in the shell:

```sh
POCKETFUSE_DB_PATH="$PWD/data/app.db" POCKETFUSE_PORT=4000 ./bin/pocketfuse
```

## Storage

A single SQLite file with upsert-by-ID semantics. Tables: `traces`,
`observations`, `sessions`, `scores`. Single-writer — sized for local
development, not high-throughput fleets.

## Migrating from duckscope.db

The pre-rename `duckscope` binary stored data in `duckscope.db` using the
DuckDB format, which the current SQLite engine cannot read — pointing
`POCKETFUSE_DB_PATH` at the old file does **not** convert it. To keep old
data: run the old binary once against `duckscope.db`, export via
`GET /api/traces` / `GET /api/observations` / `GET /api/sessions` /
`GET /api/scores`, then re-ingest each record through `POST /api/ingest` on
the new binary. The same applies to a container: export first, then mount a
fresh directory at `/data` — do not mount the old DuckDB file at
`/data/pocketfuse.db`.
