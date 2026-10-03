# Development

## Backend live reload

[air](https://github.com/air-verse/air) rebuilds and restarts the Go binary on
`.go` changes:

```sh
make dev     # air -c .air.toml
```

## Frontend dev server

```sh
pnpm --dir web run dev
```

Vite dev server on <http://localhost:4173> — proxies `/api` to the Go server on
port 7625, with hot reload. Frontend checks use
[vite-plus](https://viteplus.dev):

```sh
pnpm --dir web run check      # fmt + lint (oxlint)
pnpm --dir web run typecheck  # tsc --noEmit
pnpm --dir web run build      # tsc -b && vp build
```

## Full checks

```sh
make check   # frontend build, go test ./..., go vet ./...
```

## CI/CD

| Workflow | Trigger | Jobs |
|---|---|---|
| `ci.yml` | push/PR to `main` | golangci-lint · `go test -race` · frontend `vp check`+tsc+test · goreleaser snapshot |
| `release.yml` | tag `v*` | goreleaser binaries + GitHub release · docker → `ghcr.io/open-ai-sdk/pocketfuse` (amd64+arm64) |
| `docs.yml` | push to `main` touching `docs/**` | vitepress build → GitHub Pages |

## Project layout

```
cmd/pocketfuse        main entry
internal/server      HTTP API + OTLP decoding
internal/store       SQLite persistence
web/                 React SPA (TanStack Router/Query/Table, tailwind, vite-plus)
docs/                This site (VitePress)
```
