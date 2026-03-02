#!/bin/bash

# --- 1. 变量准备 ---
# 默认 UUID (仅在没有环境变量 UUID 时使用)
DEFAULT_UUID="3d039e25-d253-4b05-8d9f-91badac7c3ff"
UUID=${UUID:-$DEFAULT_UUID}

# 其他配置
WS_PATH="/YDT4hf6qkamfijeiwnjwjen39" 
LISTEN_PORT=${PORT:-8001} 

# --- 2. 生成 sing-box 配置文件 ---
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

# --- 3. 启动 sing-box ---
echo "正在启动 Sing-box..."
sing-box -D /etc/sing-box run > /dev/null 2>&1 &

# --- 4. 隧道逻辑切换 ---
if [ -n "$DOMAIN" ] && [ -n "$TOKEN" ]; then
    # 情况 A: 变量齐全，使用固定隧道
    echo "检测到 DOMAIN 和 TOKEN，启动【固定隧道】模式..."
    cloudflared tunnel --no-autoupdate run --token ${TOKEN} > /dev/null 2>&1 &
    FINAL_DOMAIN=$DOMAIN
    MODE="Fixed"
else
    # 情况 B: 缺少变量，使用临时隧道
    echo "未检测到完整变量，启动【临时隧道】模式..."
    LOG_FILE="/tmp/cloudflared.log"
    # 映射本地 sing-box 端口到临时域名
    cloudflared tunnel --no-autoupdate --url http://localhost:${LISTEN_PORT} > ${LOG_FILE} 2>&1 &
    
    # 轮询日志以获取生成的临时域名 (最多等待 15 秒)
    echo "正在获取临时域名..."
    for i in {1..15}; do
        TEMP_DOMAIN=$(grep -oE "https://[a-zA-Z0-9-]+\.trycloudflare\.com" ${LOG_FILE} | head -n 1 | sed 's#https://##')
        if [ -n "$TEMP_DOMAIN" ]; then
            FINAL_DOMAIN=$TEMP_DOMAIN
            break
        fi
        sleep 1
    done

    if [ -z "$FINAL_DOMAIN" ]; then
        echo "❌ 错误: 无法获取临时域名。请检查网络或 cloudflared 是否安装。"
        exit 1
    fi
    MODE="Temp"
fi

# --- 5. 生成 VLESS 链接 ---
VLESS_LINK="vless://${UUID}@www.visa.com:443?encryption=none&security=tls&sni=${FINAL_DOMAIN}&type=ws&host=${FINAL_DOMAIN}&path=${WS_PATH}#Vless-Argo"

# --- 6. 最终输出 ---
echo "---------------------------------------------------" 
echo "🎉 服务已成功运行！" 
echo "模式: ${MODE}"
echo "UUID: ${UUID}"
echo "域名: ${FINAL_DOMAIN}"
echo "---------------------------------------------------" 
echo "VLESS 节点链接:" 
echo "${VLESS_LINK}" 
echo "---------------------------------------------------" 

# 保持脚本运行，防止容器/进程退出
wait
