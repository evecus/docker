#!/bin/sh

DATA_DIR="/data/files"
ENV_CACHE="/data/.env_cache"
ENV_HASH_FILE="/data/.env_hash"
SYNCING_FILE="/data/.syncing"

# ─── 0. 应用时区 ─────────────────────────────────────────────────────────────

TZ="${TZ:-Asia/Shanghai}"
if [ -f "/usr/share/zoneinfo/$TZ" ]; then
    ln -sf "/usr/share/zoneinfo/$TZ" /etc/localtime
    echo "$TZ" > /etc/timezone
    echo "[init] 时区已设置为: $TZ"
else
    echo "[init] 警告: 时区 '$TZ' 不存在，回退到 Asia/Shanghai"
    TZ="Asia/Shanghai"
    ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime
    echo "Asia/Shanghai" > /etc/timezone
fi
export TZ

# ─── 1. 读取并校验环境变量 ───────────────────────────────────────────────────

REPOSITORIES="${REPOSITORIES:-}"
URLS="${URLS:-}"
PATH_NAME="${PATH_NAME:-}"
TIME="${TIME:-12:00}"
CRON_ENV="${CRON:-}"

is_valid_cron() {
    echo "$1" | grep -qE '^(\*|[0-9,-/]+) (\*|[0-9,-/]+) (\*|[0-9,-/]+) (\*|[0-9,-/]+) (\*|[0-9,-/]+)$'
}
is_valid_time() {
    echo "$1" | grep -qE '^([01][0-9]|2[0-3]):[0-5][0-9]$'
}

# 将小时数按 TZ 偏移转换为 UTC 小时
tz_hour_to_utc() {
    _h="$1"
    TZ_OFFSET=$(date +%z)
    TZ_SIGN=$(echo "$TZ_OFFSET" | cut -c1)
    TZ_H=$(echo "$TZ_OFFSET" | cut -c2-3 | sed 's/^0//')
    [ -z "$TZ_H" ] && TZ_H=0
    if [ "$TZ_SIGN" = "+" ]; then
        echo $(( (_h - TZ_H + 24) % 24 ))
    else
        echo $(( (_h + TZ_H) % 24 ))
    fi
}

if is_valid_cron "$CRON_ENV"; then
    CRON_MIN=$(echo "$CRON_ENV"  | awk '{print $1}')
    CRON_H=$(echo "$CRON_ENV"    | awk '{print $2}')
    CRON_REST=$(echo "$CRON_ENV" | awk '{print $3,$4,$5}')
    if echo "$CRON_H" | grep -qE '^[0-9]+$'; then
        #UTC_H=$(tz_hour_to_utc "$CRON_H")
        FINAL_CRON="$CRON_MIN $CRON_H $CRON_REST"
        echo "[init] 使用 CRON=$CRON_ENV ($TZ) -> cron UTC: $FINAL_CRON"
    else
        FINAL_CRON="$CRON_ENV"
        echo "[init] 警告: CRON 小时字段非固定值，无法转换时区，直接按 UTC 使用: $FINAL_CRON"
    fi
elif is_valid_time "$TIME"; then
    HOUR=$(echo "$TIME" | cut -d: -f1)
    MINUTE=$(echo "$TIME" | cut -d: -f2)
    #UTC_HOUR=$(tz_hour_to_utc "$HOUR")
    FINAL_CRON="$MINUTE $HOUR * * *"
    echo "[init] 使用 TIME=$TIME ($TZ) -> cron UTC: $FINAL_CRON"
else
    FINAL_CRON="0 12 * * *"
    echo "[init] TIME 格式无效，使用默认 UTC 12:00 (Asia/Shanghai 12:00) -> cron: $FINAL_CRON"
fi

# ─── 2. 检测环境变量是否变化 ─────────────────────────────────────────────────

mkdir -p "$DATA_DIR"

# 只对 REPOSITORIES / URLS / PATH_NAME 做哈希
CURRENT_HASH=$(printf '%s\n' "$REPOSITORIES" "$URLS" "$PATH_NAME" | md5sum | cut -d' ' -f1)
OLD_HASH=$(cat "$ENV_HASH_FILE" 2>/dev/null || echo "")

# 将运行时参数写入缓存（cron 从这里读）
cat > "$ENV_CACHE" << EOF
REPOSITORIES='$REPOSITORIES'
URLS='$URLS'
PATH_NAME='$PATH_NAME'
FINAL_CRON='$FINAL_CRON'
DATA_DIR='$DATA_DIR'
SYNCING_FILE='$SYNCING_FILE'
EOF

# ─── 3. 同步函数 ─────────────────────────────────────────────────────────────

do_sync() {
    echo "[sync] ===== 开始同步 $(date '+%Y-%m-%d %H:%M:%S') ====="

    # 写同步中标志（nginx 通过 /__syncing__ 暴露给前端轮询）
    echo "$(date '+%Y-%m-%d %H:%M:%S')" > "$SYNCING_FILE"

    rm -rf "${DATA_DIR:?}"/*
    mkdir -p "$DATA_DIR"

    # ── 处理 REPOSITORIES ──────────────────────────────────────────────────
    if [ -n "$REPOSITORIES" ]; then
        VALID_COUNT=0
        for entry in $REPOSITORIES; do
            user=$(echo "$entry" | cut -d'/' -f1)
            repo=$(echo "$entry" | cut -d'/' -f2)
            [ -n "$user" ] && [ -n "$repo" ] && VALID_COUNT=$(( VALID_COUNT + 1 ))
        done

        MULTI=0
        [ "$VALID_COUNT" -gt 1 ] && MULTI=1

        for entry in $REPOSITORIES; do
            user=$(echo "$entry" | cut -d'/' -f1)
            repo=$(echo "$entry" | cut -d'/' -f2)
            branch=$(echo "$entry" | cut -d'/' -f3)
            [ -z "$branch" ] && branch="main"

            if [ -z "$user" ] || [ -z "$repo" ]; then
                echo "[sync] 跳过无效条目: $entry"
                continue
            fi

            REPO_URL="https://github.com/$user/$repo"
            echo "[sync] 检测: $REPO_URL @ $branch"

            if git ls-remote --exit-code --heads "$REPO_URL" "$branch" > /dev/null 2>&1; then
                TMP=$(mktemp -d)
                if git clone --depth=1 --branch "$branch" "$REPO_URL" "$TMP" 2>&1; then
                    if [ "$MULTI" -eq 1 ]; then
                        DEST="$DATA_DIR/$repo/$branch"
                        mkdir -p "$DEST"
                        find "$TMP" -mindepth 1 -maxdepth 1 \
                            ! -name ".github" ! -name "README.md" ! -name ".git" \
                            -exec cp -r {} "$DEST/" \;
                        echo "[sync] ✓ $user/$repo/$branch -> $repo/$branch/"
                    else
                        find "$TMP" -mindepth 1 -maxdepth 1 \
                            ! -name ".github" ! -name "README.md" ! -name ".git" \
                            -exec cp -r {} "$DATA_DIR/" \;
                        echo "[sync] ✓ $user/$repo/$branch -> /"
                    fi
                else
                    echo "[sync] ✗ 克隆失败: $user/$repo/$branch"
                fi
                rm -rf "$TMP"
            else
                echo "[sync] ✗ 分支不存在或无法访问: $user/$repo/$branch"
            fi
        done
    fi

    # ── 处理 URLS ──────────────────────────────────────────────────────────
    if [ -n "$URLS" ]; then
        if [ -n "$PATH_NAME" ]; then DEST="$DATA_DIR/$PATH_NAME"; else DEST="$DATA_DIR"; fi
        mkdir -p "$DEST"
        for url in $URLS; do
            filename=$(basename "$url" | cut -d'?' -f1)
            if wget -q --timeout=15 --tries=2 -O "$DEST/$filename" "$url"; then
                echo "[sync] ✓ $filename"
            else
                rm -f "$DEST/$filename"
                echo "[sync] ✗ 跳过: $url"
            fi
        done
    fi

    date '+%Y-%m-%d %H:%M:%S' > /data/.last_update

    # 删除同步中标志，前端轮询到文件消失后自动刷新
    rm -f "$SYNCING_FILE"

    echo "[sync] ===== 同步完成 ====="
}

if [ "$CURRENT_HASH" != "$OLD_HASH" ]; then
    echo "[init] 环境变量已变化，触发同步..."
    do_sync
    echo "$CURRENT_HASH" > "$ENV_HASH_FILE"
else
    echo "[init] 环境变量未变化，跳过同步"
fi

# ─── 4. 注册 cron 定时任务 ───────────────────────────────────────────────────

cat > /sync.sh << 'SYNCEOF'
#!/bin/sh
ENV_CACHE="/data/.env_cache"
. "$ENV_CACHE"

do_sync() {
    echo "[sync] ===== 开始同步 $(date '+%Y-%m-%d %H:%M:%S') ====="
    echo "$(date '+%Y-%m-%d %H:%M:%S')" > "$SYNCING_FILE"

    rm -rf "${DATA_DIR:?}"/*
    mkdir -p "$DATA_DIR"

    if [ -n "$REPOSITORIES" ]; then
        VALID_COUNT=0
        for entry in $REPOSITORIES; do
            user=$(echo "$entry" | cut -d'/' -f1)
            repo=$(echo "$entry" | cut -d'/' -f2)
            [ -n "$user" ] && [ -n "$repo" ] && VALID_COUNT=$(( VALID_COUNT + 1 ))
        done

        MULTI=0
        [ "$VALID_COUNT" -gt 1 ] && MULTI=1

        for entry in $REPOSITORIES; do
            user=$(echo "$entry" | cut -d'/' -f1)
            repo=$(echo "$entry" | cut -d'/' -f2)
            branch=$(echo "$entry" | cut -d'/' -f3)
            [ -z "$branch" ] && branch="main"

            if [ -z "$user" ] || [ -z "$repo" ]; then
                echo "[sync] 跳过无效条目: $entry"
                continue
            fi

            REPO_URL="https://github.com/$user/$repo"
            echo "[sync] 检测: $REPO_URL @ $branch"

            if git ls-remote --exit-code --heads "$REPO_URL" "$branch" > /dev/null 2>&1; then
                TMP=$(mktemp -d)
                if git clone --depth=1 --branch "$branch" "$REPO_URL" "$TMP" 2>&1; then
                    if [ "$MULTI" -eq 1 ]; then
                        DEST="$DATA_DIR/$repo/$branch"
                        mkdir -p "$DEST"
                        find "$TMP" -mindepth 1 -maxdepth 1 \
                            ! -name ".github" ! -name "README.md" ! -name ".git" \
                            -exec cp -r {} "$DEST/" \;
                        echo "[sync] ✓ $user/$repo/$branch -> $repo/$branch/"
                    else
                        find "$TMP" -mindepth 1 -maxdepth 1 \
                            ! -name ".github" ! -name "README.md" ! -name ".git" \
                            -exec cp -r {} "$DATA_DIR/" \;
                        echo "[sync] ✓ $user/$repo/$branch -> /"
                    fi
                else
                    echo "[sync] ✗ 克隆失败: $user/$repo/$branch"
                fi
                rm -rf "$TMP"
            else
                echo "[sync] ✗ 分支不存在或无法访问: $user/$repo/$branch"
            fi
        done
    fi

    if [ -n "$URLS" ]; then
        if [ -n "$PATH_NAME" ]; then DEST="$DATA_DIR/$PATH_NAME"; else DEST="$DATA_DIR"; fi
        mkdir -p "$DEST"
        for url in $URLS; do
            filename=$(basename "$url" | cut -d'?' -f1)
            if wget -q --timeout=15 --tries=2 -O "$DEST/$filename" "$url"; then
                echo "[sync] ✓ $filename"
            else
                rm -f "$DEST/$filename"
                echo "[sync] ✗ 跳过: $url"
            fi
        done
    fi

    date '+%Y-%m-%d %H:%M:%S' > /data/.last_update
    rm -f "$SYNCING_FILE"
    echo "[sync] ===== 同步完成 ====="
}

do_sync
SYNCEOF
chmod +x /sync.sh

echo "$FINAL_CRON /bin/sh /sync.sh >> /data/sync.log 2>&1" > /etc/crontabs/root
echo "[init] cron 已注册: $FINAL_CRON"

crond

# ─── 5. 启动 Nginx（前台）───────────────────────────────────────────────────

echo "[init] 启动 Nginx..."
exec nginx -g "daemon off;"
