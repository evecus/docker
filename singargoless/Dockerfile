# ---------- 第一阶段：下载 sing-box 和 cloudflared ----------
FROM alpine:latest AS builder

RUN apk add --no-cache curl tar jq ca-certificates

ARG TARGETARCH
ARG SB_VER_TAG
ARG CF_VER_TAG

# 下载 sing-box
RUN set -eux; \
    SB_VERSION=$(curl -s https://api.github.com/repos/SagerNet/sing-box/releases/latest | jq -r '.tag_name' | sed 's/^v//'); \
    case "$TARGETARCH" in \
      amd64) SB_ARCH=amd64 ;; \
      arm64) SB_ARCH=arm64 ;; \
      *) echo "unsupported arch: $TARGETARCH" && exit 1 ;; \
    esac; \
    curl -Lo /tmp/sing-box.tar.gz \
      https://github.com/SagerNet/sing-box/releases/download/v${SB_VERSION}/sing-box-${SB_VERSION}-linux-${SB_ARCH}.tar.gz; \
    tar -xzf /tmp/sing-box.tar.gz -C /tmp; \
    mv /tmp/sing-box-*/sing-box /usr/local/bin/sing-box; \
    chmod +x /usr/local/bin/sing-box

# 下载 cloudflared（纯静态 Go 二进制，无 libc 依赖）
RUN set -eux; \
    case "$TARGETARCH" in \
      amd64) CF_ARCH=amd64 ;; \
      arm64) CF_ARCH=arm64 ;; \
      *) echo "unsupported arch: $TARGETARCH" && exit 1 ;; \
    esac; \
    curl -Lo /usr/local/bin/cloudflared \
      https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-${CF_ARCH}; \
    chmod +x /usr/local/bin/cloudflared


# ---------- 第二阶段：运行 ----------
FROM alpine:latest

# gcompat 提供 glibc 兼容层，sing-box 动态链接了 glibc 符号需要它
# 符号链接让 ELF interpreter 路径能被 Alpine 的 musl 链接器接管
RUN apk add --no-cache bash ca-certificates curl gcompat && \
    case "$(uname -m)" in \
      x86_64)  ln -sf /lib/ld-musl-x86_64.so.1    /lib/ld-linux-x86-64.so.2 ;; \
      aarch64) ln -sf /lib/ld-musl-aarch64.so.1   /lib/ld-linux-aarch64.so.1 ;; \
    esac

RUN mkdir -p /etc/sing-box

COPY --from=builder /usr/local/bin/sing-box /usr/local/bin/
COPY --from=builder /usr/local/bin/cloudflared /usr/local/bin/
COPY entrypoint.sh /entrypoint.sh

RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/bin/bash", "/entrypoint.sh"]
