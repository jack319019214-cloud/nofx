#!/bin/bash
# 代理健康监控和自动恢复脚本
# 每5分钟检查一次,如果检测到连接失败则自动重启ss-proxy

PROXY_CONTAINER="ss-proxy"
BACKEND_CONTAINER="nofx-trading"
LOG_FILE="/root/nofx/logs/proxy_monitor.log"
STATE_FILE="/root/nofx/logs/.proxy_monitor_state"
BINANCE_TEST_URL="https://fapi.binance.com/fapi/v1/ping"
PROXY_PORT="${PROXY_PORT:-1080}"  # ss-local监听端口
PROXY_HOST="${PROXY_HOST:-auto}"  # auto=自动读取容器IP
MAX_FAILURES=3  # 连续失败3次才重启
FAILURE_COUNT=0

# 创建日志目录 & 状态文件目录
mkdir -p "$(dirname "$LOG_FILE")"
mkdir -p "$(dirname "$STATE_FILE")"

load_failure_count() {
    if [ -f "$STATE_FILE" ]; then
        local value
        value=$(cat "$STATE_FILE" 2>/dev/null)
        if [[ "$value" =~ ^[0-9]+$ ]]; then
            FAILURE_COUNT=$value
            return
        fi
    fi
    FAILURE_COUNT=0
}

save_failure_count() {
    echo "$FAILURE_COUNT" > "$STATE_FILE"
}

load_failure_count

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" | tee -a "$LOG_FILE"
}

resolve_proxy_host() {
    if [ "$PROXY_HOST" = "auto" ]; then
        local ip
        ip=$(docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$PROXY_CONTAINER" 2>/dev/null)
        if [ -z "$ip" ]; then
            log "❌ 无法获取 $PROXY_CONTAINER 的容器IP"
            return 1
        fi
        echo "$ip"
    else
        echo "$PROXY_HOST"
    fi
}

# 检查代理是否正常工作
check_proxy() {
    # 方法1: 检查ss-proxy容器状态
    if ! docker ps | grep -q "$PROXY_CONTAINER"; then
        log "❌ $PROXY_CONTAINER 容器未运行"
        return 1
    fi

    # 方法2: 检查ss-proxy日志中是否有timeout错误
    RECENT_ERRORS=$(docker logs "$PROXY_CONTAINER" --tail 50 --since 5m 2>&1 | grep -i "timeout\|error\|failed" | wc -l)
    if [ "$RECENT_ERRORS" -gt 10 ]; then
        log "⚠️  检测到 $PROXY_CONTAINER 最近5分钟有 $RECENT_ERRORS 个错误"
        return 1
    fi

    # 方法3: 检查后端日志中是否有EOF错误
    BACKEND_ERRORS=$(docker logs "$BACKEND_CONTAINER" --tail 100 --since 5m 2>&1 | grep -i "EOF\|获取持仓失败\|获取账户失败" | wc -l)
    if [ "$BACKEND_ERRORS" -gt 5 ]; then
        log "⚠️  检测到后端最近5分钟有 $BACKEND_ERRORS 个API失败"
        return 1
    fi

    local proxy_host
    proxy_host=$(resolve_proxy_host) || return 1

    # 方法4: 直接通过代理请求币安
    if ! curl -sS --max-time 8 --retry 2 --retry-delay 2 \
        --socks5-hostname "${proxy_host}:${PROXY_PORT}" "$BINANCE_TEST_URL" >/dev/null; then
        log "⚠️  通过代理访问币安失败,可能是代理掉线"
        return 1
    fi

    log "✅ 代理运行正常"
    return 0
}

# 重启代理容器
restart_proxy() {
    log "🔄 正在重启 $PROXY_CONTAINER..."

    # 记录重启前的状态
    docker logs "$PROXY_CONTAINER" --tail 20 >> "$LOG_FILE" 2>&1

    # 重启容器
    docker restart "$PROXY_CONTAINER"

    if [ $? -eq 0 ]; then
        log "✅ $PROXY_CONTAINER 重启成功"
        # 等待5秒让代理稳定
        sleep 5
        return 0
    else
        log "❌ $PROXY_CONTAINER 重启失败"
        return 1
    fi
}

# 发送告警通知(可扩展为钉钉/企业微信/邮件等)
send_alert() {
    local message="$1"
    log "🚨 告警: $message"

    # TODO: 这里可以添加钉钉/企业微信/邮件通知
    # 示例: curl -X POST "钉钉webhook" -d "{\"text\": \"$message\"}"
}

# 主监控循环
log "========================================="
log "🔍 开始代理健康检查"

if ! check_proxy; then
    FAILURE_COUNT=$((FAILURE_COUNT + 1))
    save_failure_count
    log "⚠️  代理检查失败 ($FAILURE_COUNT/$MAX_FAILURES)"

    if [ $FAILURE_COUNT -ge $MAX_FAILURES ]; then
        log "❗ 连续失败 $MAX_FAILURES 次,触发自动重启"

        if restart_proxy; then
            send_alert "代理异常,已自动重启并恢复正常"
            FAILURE_COUNT=0
            save_failure_count
        else
            send_alert "代理异常,自动重启失败,请人工介入"
        fi
    fi
else
    # 重置失败计数
    if [ $FAILURE_COUNT -gt 0 ]; then
        log "✅ 代理已恢复,重置失败计数"
        FAILURE_COUNT=0
        save_failure_count
    fi
fi

log "========================================="
