# ---------- 第一阶段：下载 sing-box ----------
FROM alpine:3.19 AS builder

RUN apk add --no-cache curl tar

ARG TARGETARCH
# 修复：补充 AS builder 命名，并使用 TARGETARCH 支持多架构，移除硬编码 amd64
RUN set -eux; \
    case "$TARGETARCH" in \
      amd64) SB_ARCH=amd64 ;; \
      arm64) SB_ARCH=arm64 ;; \
      *) echo "Unsupported arch: $TARGETARCH" && exit 1 ;; \
    esac; \
    SB_VERSION=$(curl -s https://api.github.com/repos/SagerNet/sing-box/releases/latest | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/' | sed 's/v//'); \
    curl -Lo /tmp/sing-box.tar.gz https://github.com/SagerNet/sing-box/releases/download/v1.12.22/sing-box-1.12.22-linux-${SB_ARCH}.tar.gz; \
    tar -xzf /tmp/sing-box.tar.gz -C /tmp; \
    mv /tmp/sing-box-*/sing-box /usr/local/bin/sing-box; \
    chmod +x /usr/local/bin/sing-box

# ---------- 第二阶段：运行镜像 ----------
FROM alpine:latest

# 安装 openssl 用于生成证书，curl 用于获取公网 IP
RUN apk add --no-cache bash curl openssl ca-certificates

COPY --from=builder /usr/local/bin/sing-box /usr/local/bin/
COPY singhy/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENV PASSWORD="jiend329ie2ek2okq"

ENTRYPOINT ["/bin/bash", "/entrypoint.sh"]
