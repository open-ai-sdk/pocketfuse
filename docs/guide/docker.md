# Docker & Compose

The Dockerfile **packages artifacts only** — build the frontend first so
`web/dist` exists (`make docker-build` / `make docker-up` do both; CI builds it
in a separate job before the image build).

```sh
make docker-build          # pnpm --dir web build + docker build -t duckscope:local
make docker-up             # frontend build + docker compose up --build
make docker-down
```

## Compose layout

```yaml
duckscope:
  build: .
  ports: ["${DUCKSCOPE_PORT:-3825}:3825"]
  volumes: ["${DUCKSCOPE_DATA_DIR:-./data}:/data"]
```

The process listens on **3825** inside the container. `DUCKSCOPE_PORT` is the
host port. `DUCKSCOPE_DB_PATH` must resolve inside the container — normally
`/data/duckscope.db`.

Custom port/data dir:

```sh
DUCKSCOPE_PORT=4000 DUCKSCOPE_DATA_DIR="$PWD/.duckscope-data" make docker-up
```

## Multi-arch images

Tagged releases (`v*`) build and push `ghcr.io/open-ai-sdk/duckscope` for
`linux/amd64` and `linux/arm64`. Pull the matching tag:

```sh
docker pull ghcr.io/open-ai-sdk/duckscope:latest
```

## Unprivileged user

The image runs as uid `10001`. If a bind mount was created with restrictive
ownership, make it writable first:

```sh
sudo chown -R 10001:10001 data
```
