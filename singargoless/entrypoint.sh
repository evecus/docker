#!/bin/bash

# 1. 检查环境变量 
if [ -z "$UUID" ] || [ -z "$DOMAIN" ] || [ -z "$TOKEN" ]; then
    echo "错误: 请确保设置了 UUID, DOMAIN 和 TOKEN 环境变量。" 
    exit 1 
fi

WS_PATH="/YDT4hf6q3ndbRzwvefijeiwnjwjen39" 
LISTEN_PORT=${PORT:-8001} 

# 2. 生成 sing-box 配置文件 (VLESS + WS) 
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

# 5. 生成 VLESS 节点链接 
VLESS_LINK="vless://${UUID}@www.visa.com:443?encryption=none&security=tls&sni=${DOMAIN}&type=ws&host=${DOMAIN}&path=${WS_PATH}#Argo-VLESS"

# 6. 启动服务 (后台运行) 
cloudflared tunnel --no-autoupdate run --token ${TOKEN} > /dev/null 2>&1 &
sing-box -D /etc/sing-box run > /dev/null 2>&1 &

# 7. 检测连接状态 
echo "正在启动并检测 Argo 隧道连接状态..." 

MAX_RETRIES=30 
COUNT=0 
while [ $COUNT -lt $MAX_RETRIES ]; do
    STATUS=$(curl -s -L -o /dev/null -w "%{http_code}" "https://${DOMAIN}" --max-time 2) 
    
    if [ "$STATUS" != "000" ]; then 
        echo "---------------------------------------------------" 
        echo "✅ Argo 隧道连接成功！" 
        echo "🚀 sing-box VLESS 服务已启动"
        echo "---------------------------------------------------" 
        echo "VLESS 节点链接:" 
        echo "${VLESS_LINK}" 
        echo "---------------------------------------------------" 
        wait 
        exit 0 
    fi
    sleep 2 
    COUNT=$((COUNT + 1)) 
done

echo "❌ 隧道连接超时，请检查 TOKEN 和域名配置。" 
exit 1
