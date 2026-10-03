# Docker & Compose

## Run the published image

Tagged releases publish multi-arch images (`linux/amd64`, `linux/arm64`) to
`ghcr.io/open-ai-sdk/pocketfuse`. Both `docker run` and Compose pull the
matching architecture automatically.

**docker run** (single command, data in `./data`):

```sh
mkdir -p data
docker run -d --name pocketfuse \
  -p 7625:7625 \
  -v "$PWD/data:/data" \
  ghcr.io/open-ai-sdk/pocketfuse:latest
```

Open <http://127.0.0.1:7625>. Stop with `docker stop pocketfuse`; data stays
in `./data`.

**docker compose** (recommended — healthcheck, restart policy, env wiring):

```sh
docker compose up -d          # pulls ghcr.io/open-ai-sdk/pocketfuse:latest
docker compose ps             # wait for "healthy"
docker compose down           # container removed, ./data kept
```

Pin a version with `POCKETFUSE_IMAGE`:

```sh
POCKETFUSE_IMAGE=ghcr.io/open-ai-sdk/pocketfuse:1.2.3 docker compose up -d
```

Custom host port / data directory:

```sh
POCKETFUSE_PORT=4000 POCKETFUSE_DATA_DIR="$PWD/.pocketfuse-data" \
  docker compose up -d
```

`POCKETFUSE_PORT` is the host port. The process always listens on **7625**
inside the container, and `POCKETFUSE_DB_PATH` must resolve inside the
container — normally `/data/pocketfuse.db`.

## Build locally (development)

The Dockerfile **packages prebuilt artifacts only** — nothing is compiled
inside the image. `make docker-build` stages the context (`build/docker/`) with
a Linux binary for your host arch in GoReleaser's `linux/<arch>/pocketfuse`
layout, then builds `pocketfuse:local`. `make docker-up` runs that local image
through the same Compose file via `POCKETFUSE_IMAGE`.

```sh
make docker-build          # frontend + linux binary + docker build -t pocketfuse:local
make docker-up             # docker-build + docker compose up (local image)
make docker-down
```

## Unprivileged user

The image runs as uid `10001`. If a bind mount was created with restrictive
ownership, make it writable first:

```sh
sudo chown -R 10001:10001 data
```
