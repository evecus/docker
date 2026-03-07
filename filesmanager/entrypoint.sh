#!/bin/sh

# ── 1. 时区 ───────────────────────────────────────────────────────────────────
TZ="${TZ:-Asia/Shanghai}"
if [ -f "/usr/share/zoneinfo/$TZ" ]; then
    ln -sf "/usr/share/zoneinfo/$TZ" /etc/localtime
    echo "$TZ" > /etc/timezone
    echo "[init] 时区: $TZ"
else
    echo "[init] 警告: 时区 '$TZ' 不存在，回退 Asia/Shanghai"
    ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime
    echo "Asia/Shanghai" > /etc/timezone
fi
export TZ

# ── 2. 数据目录 ───────────────────────────────────────────────────────────────
mkdir -p /data/files
export DATA_DIR="/data/files"

# ── 3. Cloudflare Argo 隧道（可选）──────────────────────────────────────────
CF="${CF:-}"
TOKEN="${TOKEN:-}"
if [ "$CF" = "true" ] && [ -n "$TOKEN" ]; then
    echo "[init] 启动 Cloudflare Argo 隧道..."
    cloudflared tunnel --no-autoupdate run --token "$TOKEN" > /data/cloudflared.log 2>&1 &
    echo "[init] Argo 隧道已在后台启动"
else
    echo "[init] 未启用 Argo 隧道"
fi

# ── 4. 启动 FilesManager ────────────────────────────────────────────────────────
echo "[init] 启动 FilesManager..."
exec /usr/local/bin/filesmanager 8080
