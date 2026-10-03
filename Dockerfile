# syntax=docker/dockerfile:1.7

# The Go server embeds web/dist. Build it before `docker build`
# (`pnpm --dir web build`, `make docker-build`, or CI's frontend job) —
# this Dockerfile only packages artifacts, it does not build the SPA.

# go-duckdb uses CGO; its prebuilt static libraries require glibc (fortify
# __*_chk symbols), so the builder uses Debian instead of Alpine.
FROM golang:1.25-bookworm AS backend

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . ./

RUN test -f web/dist/index.html || (echo "web/dist is missing — run 'pnpm --dir web build' first" && exit 1)

RUN CGO_ENABLED=1 go build \
    -trimpath \
    -ldflags='-s -w' \
    -o /out/duckscope \
    ./cmd/duckscope

# Keep the runtime image small. /data is the only persistent mount required.
FROM debian:bookworm-slim AS runtime

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates libstdc++6 wget \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd -r -g 10001 duckscope \
    && useradd -r -u 10001 -g duckscope duckscope \
    && mkdir -p /data \
    && chown duckscope:duckscope /data

COPY --from=backend /out/duckscope /usr/local/bin/duckscope

ENV DUCKSCOPE_HOST=0.0.0.0 \
    DUCKSCOPE_PORT=3825 \
    DUCKSCOPE_DB_PATH=/data/duckscope.db

VOLUME ["/data"]
EXPOSE 3825

USER duckscope
ENTRYPOINT ["/usr/local/bin/duckscope"]
