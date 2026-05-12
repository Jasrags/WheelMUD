# syntax=docker/dockerfile:1.7
#
# WheelMUD container image — Phase J slice J7 (#59).
#
# Build args (set via `docker build --build-arg`):
#   VERSION  — semver tag, e.g. v0.1.0
#   COMMIT   — git commit SHA
#   DATE     — RFC3339 build timestamp
#
# These are injected into the binary via ldflags so the runtime
# `/metrics` endpoint and `wheelmud-server -version` report
# meaningful values.

ARG GO_VERSION=1.25
ARG ALPINE_VERSION=3.20

# ─── Build stage ──────────────────────────────────────────────────────
FROM golang:${GO_VERSION}-alpine AS builder

ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown

WORKDIR /src

# Pre-fetch modules to leverage layer caching across source-only edits.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOFLAGS="-trimpath" go build \
      -ldflags "-s -w \
        -X main.buildVersion=${VERSION} \
        -X main.buildCommit=${COMMIT} \
        -X main.buildDate=${DATE}" \
      -o /out/wheelmud-server \
      ./cmd/server

# ─── Runtime stage ────────────────────────────────────────────────────
FROM alpine:${ALPINE_VERSION}

# wget is needed for the HEALTHCHECK probe; ca-certificates for any
# future outbound TLS (none today, but cheap insurance).
RUN apk add --no-cache wget ca-certificates && \
    addgroup -S wheelmud && \
    adduser -S -G wheelmud -h /var/lib/wheelmud -s /sbin/nologin wheelmud && \
    mkdir -p /var/lib/wheelmud /etc/wheelmud /var/backups/wheelmud && \
    chown -R wheelmud:wheelmud /var/lib/wheelmud /var/backups/wheelmud

WORKDIR /var/lib/wheelmud

COPY --from=builder /out/wheelmud-server /usr/local/bin/wheelmud-server
COPY --chown=wheelmud:wheelmud data/world /var/lib/wheelmud/data/world
COPY --chown=wheelmud:wheelmud config.example.yaml /etc/wheelmud/config.example.yaml

# Environment defaults. Operators override DB_DSN / BACKUP_DIR /
# WORLD_DIR at run time (or supply a config.yaml under /etc/wheelmud).
ENV DB_DSN=/var/lib/wheelmud/wheelmud.db \
    WORLD_DIR=/var/lib/wheelmud/data/world \
    BACKUP_DIR=/var/backups/wheelmud \
    LISTEN_ADDR=:2323 \
    METRICS_ADDR=127.0.0.1:9090 \
    LOG_LEVEL=info

EXPOSE 2323 9090

USER wheelmud

# /healthz comes from internal/metrics (Phase J slice J5). Returns
# 200 once the listener binds + DB ping succeeds; 503 during drain.
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
  CMD wget -q -O - http://127.0.0.1:9090/healthz || exit 1

ENTRYPOINT ["/usr/local/bin/wheelmud-server"]
