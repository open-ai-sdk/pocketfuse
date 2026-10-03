# syntax=docker/dockerfile:1.7

# This image packages prebuilt artifacts only — nothing is compiled here.
# The build context must contain the Linux binaries in GoReleaser's layout:
#   linux/amd64/pocketfuse, linux/arm64/pocketfuse
# Both release paths stage it for you:
#   - `goreleaser release` (dockers_v2) — CI, multi-arch push to GHCR
#   - `make docker-build` — local, stages build/docker/ for this host arch
# The SQLite driver (modernc.org/sqlite) is pure Go (CGO_ENABLED=0), so a
# static binary runs on alpine for both architectures.

FROM alpine:3.22

# TARGETPLATFORM is set by BuildKit/buildx (e.g. linux/arm64) and selects
# the matching prebuilt binary from the context.
ARG TARGETPLATFORM

RUN apk add --no-cache ca-certificates wget \
    && addgroup -S pocketfuse \
    && adduser -S -u 10001 -G pocketfuse pocketfuse \
    && mkdir -p /data \
    && chown pocketfuse:pocketfuse /data

COPY ${TARGETPLATFORM}/pocketfuse /usr/local/bin/pocketfuse

ENV POCKETFUSE_HOST=0.0.0.0 \
    POCKETFUSE_PORT=7625 \
    POCKETFUSE_DB_PATH=/data/pocketfuse.db

VOLUME ["/data"]
EXPOSE 7625

USER pocketfuse
ENTRYPOINT ["/usr/local/bin/pocketfuse"]
