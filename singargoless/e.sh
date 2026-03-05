#!/bin/bash

# ── 1. 下载 sing-box ──
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)  SB_ARCH=amd64 ;;
  aarch64) SB_ARCH=arm64 ;;
  *) echo "[error] 不支持的架构: $ARCH" && exit 1 ;;
esac

if [ ! -x /usr/local/bin/sing-box ]; then
    echo "[download] 获取 sing-box ..."
    curl -fsSL -o /tmp/sing-box.tar.gz \
        "https://github.com/SagerNet/sing-box/releases/download/v1.12.22/sing-box-1.12.22-linux-${SB_ARCH}.tar.gz"
    tar -xzf /tmp/sing-box.tar.gz -C /tmp
    mv /tmp/sing-box-*/sing-box /usr/local/bin/sing-box
    chmod +x /usr/local/bin/sing-box
    rm -rf /tmp/sing-box*
    echo "[download] sing-box 安装完成"
else
    echo "[download] sing-box 已存在，跳过下载"
fi

# ── 2. 下载 cloudflared ──
if [ ! -x /usr/local/bin/cloudflared ]; then
    echo "[download] 下载 cloudflared (${SB_ARCH})..."
    curl -fsSL -o /usr/local/bin/cloudflared \
        "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-${SB_ARCH}"
    chmod +x /usr/local/bin/cloudflared
    echo "[download] cloudflared 安装完成"
else
    echo "[download] cloudflared 已存在，跳过下载"
fi

# ── 3. UUID：有环境变量用环境变量，否则随机生成 ──
if [ -z "$UUID" ]; then
    UUID=$(cat /proc/sys/kernel/random/uuid 2>/dev/null || \
           od -x /dev/urandom | head -1 | awk '{print $2$3"-"$4"-"$5"-"$6"-"$7$8$9}' | head -c 36)
    echo "[init] UUID 随机生成: ${UUID}"
else
    echo "[init] 使用环境变量 UUID: ${UUID}"
fi

# ── 4. 端口：有 PORT 用 PORT，否则默认 8080 ──
LISTEN_PORT=${PORT:-8080}
echo "[init] 监听端口: ${LISTEN_PORT}"

WS_PATH="/YDT4hf6q3ndbRzwvefijeiwnjwjen39"

# ── 5. 生成 sing-box 配置 ──
cat <<EOF > /etc/sing-box/config.json
{
  "log": { "level": "warn", "timestamp": true },
  "inbounds": [
    {
      "type": "vless",
      "tag": "vless-in",
      "listen": "::",
      "listen_port": ${LISTEN_PORT},
      "users": [{ "uuid": "${UUID}" }],
      "transport": {
        "type": "ws",
        "path": "${WS_PATH}"
      }
    }
  ],
  "outbounds": [{ "type": "direct", "tag": "direct" }]
}
EOF

# ── 6. 启动 sing-box ──
sing-box run -c /etc/sing-box/config.json > /var/log/sing-box.log 2>&1 &
echo "[init] sing-box 已启动"

# ── 7. 隧道函数 ──
USE_TOKEN_TUNNEL=false
[ -n "$DOMAIN" ] && [ -n "$TOKEN" ] && USE_TOKEN_TUNNEL=true

start_token_tunnel() {
    echo "[tunnel] 使用 TOKEN 启动 Named Tunnel..."
    cloudflared tunnel --no-autoupdate run --token "${TOKEN}" > /var/log/cloudflared.log 2>&1 &
    CF_PID=$!
}

start_temp_tunnel() {
    echo "[tunnel] 启动临时隧道 (Quick Tunnel)..."
    cloudflared tunnel --no-autoupdate --url "http://localhost:${LISTEN_PORT}" \
        > /var/log/cloudflared.log 2>&1 &
    CF_PID=$!

    echo "[tunnel] 等待临时域名分配..."
    TEMP_DOMAIN=""
    for i in $(seq 1 30); do
        TEMP_DOMAIN=$(grep -oE 'https://[a-zA-Z0-9-]+\.trycloudflare\.com' /var/log/cloudflared.log 2>/dev/null | head -1)
        [ -n "$TEMP_DOMAIN" ] && break
        sleep 1
    done

    if [ -n "$TEMP_DOMAIN" ]; then
        DOMAIN="${TEMP_DOMAIN#https://}"
        echo "[tunnel] 临时域名: ${DOMAIN}"
    else
        echo "[tunnel] ⚠️  未能获取临时域名，请查看 /var/log/cloudflared.log"
        DOMAIN="<获取失败>"
    fi
}

check_tunnel() {
    for i in $(seq 1 15); do
        STATUS=$(curl -s -L -o /dev/null -w "%{http_code}" "https://${1}" --max-time 3 2>/dev/null)
        [ "$STATUS" != "000" ] && [ -n "$STATUS" ] && return 0
        sleep 2
    done
    return 1
}

# ── 8. 启动隧道，必要时降级临时隧道 ──
if [ "$USE_TOKEN_TUNNEL" = true ]; then
    start_token_tunnel
    echo "[tunnel] 检测 Named Tunnel 连通性 (域名: ${DOMAIN})..."
    if check_tunnel "$DOMAIN"; then
        echo "[tunnel] Named Tunnel 连接成功"
    else
        echo "[tunnel] Named Tunnel 不通，切换临时隧道..."
        kill "$CF_PID" 2>/dev/null
        sleep 1
        start_temp_tunnel
    fi
else
    echo "[tunnel] DOMAIN 或 TOKEN 未完整设置，使用临时隧道"
    start_temp_tunnel
fi

# ── 9. 输出节点信息 ──
VLESS_LINK="vless://${UUID}@www.visa.com:443?encryption=none&security=tls&sni=${DOMAIN}&type=ws&host=${DOMAIN}&path=${WS_PATH}#Argo-VLESS"

echo ""
echo "==================================================="
echo "✅ 服务已就绪"
echo "==================================================="
echo "UUID        : ${UUID}"
echo "端口        : ${LISTEN_PORT}"
echo "域名        : ${DOMAIN}"
echo "---------------------------------------------------"
echo "VLESS 节点链接:"
echo "${VLESS_LINK}"
echo "==================================================="

wait
