# Configuration

| Variable | Default | Purpose |
|---|---|---|
| `DUCKSCOPE_DB_PATH` | `./duckscope.db` | DuckDB file path. In Compose use a path under `/data`. `:memory:` also works. |
| `DUCKSCOPE_HOST` | `127.0.0.1` | Bind address. Use `0.0.0.0` in containers or for remote access. |
| `DUCKSCOPE_PORT` | `3825` | HTTP port (`DUCK` on a phone keypad). |
| `DUCKSCOPE_DATA_DIR` | `./data` | Host directory bind-mounted to `/data` by Compose. |

The Go process does not read `.env` itself — export variables in the shell:

```sh
DUCKSCOPE_DB_PATH="$PWD/data/app.db" DUCKSCOPE_PORT=4000 ./bin/duckscope
```

## Storage

A single DuckDB file with upsert-by-ID semantics. Tables: `traces`,
`observations`, `sessions`, `scores`. Single-writer — sized for local
development, not high-throughput fleets.
