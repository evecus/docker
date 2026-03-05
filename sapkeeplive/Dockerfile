# ---------- 第一阶段：下载 CF CLI 二进制 ----------
FROM alpine:latest AS fetcher

RUN apk add --no-cache curl

ARG TARGETARCH
# 官方提供 linux64-binary 压缩包，直接解压取二进制，无需 dpkg
RUN set -eux; \
    case "$TARGETARCH" in \
      amd64) CF_RELEASE="linux64-binary" ;; \
      arm64) CF_RELEASE="linuxarm64-binary" ;; \
      *) echo "Unsupported arch: $TARGETARCH" && exit 1 ;; \
    esac; \
    curl -fsSL "https://packages.cloudfoundry.org/stable?release=${CF_RELEASE}&version=v8&source=github" \
      | tar -zx -C /tmp; \
    mv /tmp/cf8 /usr/local/bin/cf8; \
    mv /tmp/cf  /usr/local/bin/cf; \
    chmod +x /usr/local/bin/cf8 /usr/local/bin/cf

# ---------- 第二阶段：运行镜像 ----------
FROM alpine:latest

RUN apk add --no-cache bash curl ca-certificates tzdata cronie

# 设置上海时区
RUN ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && echo "Asia/Shanghai" > /etc/timezone

# 从第一阶段拷贝 CF CLI
COPY --from=fetcher /usr/local/bin/cf8 /usr/local/bin/cf8
COPY --from=fetcher /usr/local/bin/cf  /usr/local/bin/cf

COPY sapkeeplive/sap-keep.sh /usr/local/bin/sap-keep.sh
RUN chmod +x /usr/local/bin/sap-keep.sh

# 初始化日志
RUN touch /var/log/cron.log

CMD ["sh", "-c", "\
    printenv > /etc/environment && \
    MINUTE=$(echo ${TIME:-08:30} | cut -d':' -f2) && \
    HOUR=$(echo ${TIME:-08:30} | cut -d':' -f1) && \
    echo \"$MINUTE $HOUR * * * . /etc/environment; /bin/bash /usr/local/bin/sap-keep.sh >> /var/log/cron.log 2>&1\" > /etc/cron.d/sap-cron && \
    chmod 0644 /etc/cron.d/sap-cron && \
    crontab /etc/cron.d/sap-cron && \
    echo \"Cron task set for $HOUR:$MINUTE\" && \
    crond && \
    tail -f /var/log/cron.log"]
