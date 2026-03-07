# ── Stage 1: download cloudflared ─────────────────────────────────────────────
FROM alpine:latest AS fetcher

RUN apk add --no-cache curl

ARG TARGETARCH
RUN set -eux; \
    case "$TARGETARCH" in \
      amd64) CF_ARCH="amd64" ;; \
      arm64) CF_ARCH="arm64" ;; \
      *) echo "Unsupported arch: $TARGETARCH" && exit 1 ;; \
    esac; \
    curl -fsSL -o /usr/local/bin/cloudflared \
      "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-${CF_ARCH}"; \
    chmod +x /usr/local/bin/cloudflared

# ── Stage 2: build Go filesmanager ──────────────────────────────────────────────
FROM golang:alpine AS builder

WORKDIR /build
COPY go.mod .
COPY main.go .
COPY templates/ templates/

RUN go build -ldflags="-s -w" -o filesmanager .

# ── Stage 3: runtime ──────────────────────────────────────────────────────────
FROM alpine:latest

RUN apk add --no-cache tzdata ca-certificates

ENV TZ=Asia/Shanghai

COPY --from=fetcher /usr/local/bin/cloudflared /usr/local/bin/cloudflared
COPY --from=builder /build/filesmanager /usr/local/bin/filesmanager

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

VOLUME ["/data"]
EXPOSE 8080

ENTRYPOINT ["/entrypoint.sh"]
