# syntax=docker/dockerfile:1.7

# The Go server embeds web/dist. Build it before `docker build`
# (`pnpm --dir web build`, `make docker-build`, or CI's frontend job) —
# this Dockerfile only packages artifacts, it does not build the SPA.

# The SQLite driver (modernc.org/sqlite) is pure Go — no CGO, no libc, so
# alpine works for both stages and cross-compiles cleanly.
FROM golang:1.26-alpine AS backend

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . ./

RUN test -f web/dist/index.html || (echo "web/dist is missing — run 'pnpm --dir web build' first" && exit 1)

RUN CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags='-s -w' \
    -o /out/pocketfuse \
    ./cmd/pocketfuse

# Keep the runtime image small. /data is the only persistent mount required.
FROM alpine:3.22 AS runtime

RUN apk add --no-cache ca-certificates wget \
    && addgroup -S pocketfuse \
    && adduser -S -u 10001 -G pocketfuse pocketfuse \
    && mkdir -p /data \
    && chown pocketfuse:pocketfuse /data

COPY --from=backend /out/pocketfuse /usr/local/bin/pocketfuse

ENV POCKETFUSE_HOST=0.0.0.0 \
    POCKETFUSE_PORT=7625 \
    POCKETFUSE_DB_PATH=/data/pocketfuse.db

VOLUME ["/data"]
EXPOSE 7625

USER pocketfuse
ENTRYPOINT ["/usr/local/bin/pocketfuse"]
