FROM debian:12-slim

WORKDIR /app

# 接收构建参数（架构）
ARG TARGETARCH

# 复制对应架构的二进制
COPY iptv-linux-${TARGETARCH} /app/iptv

# 赋予执行权限
RUN chmod +x /app/iptv

# 启动命令
ENTRYPOINT ["/app/iptv"]
