# ---------- 第一阶段：下载二进制 ----------
FROM alpine:3.19 AS builder

RUN apk add --no-cache curl tar

ARG TARGETARCH
# 修复：补充 AS builder 命名；用 TARGETARCH 替换 uname -m，支持交叉编译
RUN set -eux; \
    case "$TARGETARCH" in \
      amd64) SB_ARCH="linux-amd64"; CF_ARCH="amd64" ;; \
      arm64) SB_ARCH="linux-arm64"; CF_ARCH="arm64" ;; \
      *) echo "Unsupported arch: $TARGETARCH" && exit 1 ;; \
    esac; \
    SB_VERSION=$(curl -s https://api.github.com/repos/SagerNet/sing-box/releases/latest | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/' | sed 's/v//'); \
    curl -Lo /tmp/sing-box.tar.gz https://github.com/SagerNet/sing-box/releases/download/v1.12.22/sing-box-1.12.22-${SB_ARCH}.tar.gz; \
    tar -xzf /tmp/sing-box.tar.gz -C /tmp; \
    mv /tmp/sing-box-*/sing-box /usr/local/bin/; \
    chmod +x /usr/local/bin/sing-box; \
    curl -Lo /usr/local/bin/cloudflared https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-${CF_ARCH}; \
    chmod +x /usr/local/bin/cloudflared

# ---------- 第二阶段：运行镜像 ----------
FROM alpine:latest

RUN apk add --no-cache bash curl openssl ca-certificates jq

COPY --from=builder /usr/local/bin/sing-box /usr/local/bin/
COPY --from=builder /usr/local/bin/cloudflared /usr/local/bin/
COPY singall/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENV SELECTS="" UUID="" DOMAIN="" TOKEN="" PORT="" PASSWORD="" HPORT="" LPORT="" MPORT=""

ENTRYPOINT ["/bin/bash", "/entrypoint.sh"]
