# 代理监控和自动恢复系统

## 概述

为了防止 ss-proxy 代理连接失败导致币安 API 调用出现 EOF 错误,我们实施了多层防护机制:

## 1. 智能重试策略

### 代码级别优化 ([trader/binance_futures.go](trader/binance_futures.go))

**增强的重试参数:**
- 最大重试次数: 3次 → **5次**
- 基础延迟: 1秒 → **2秒**
- 最大延迟: **30秒** (指数退避)
- 代理恢复等待时间: **10秒**

**智能EOF检测:**
```go
// 检测连续EOF错误
if isEOF {
    consecutiveEOFCount++
}

// 连续2次EOF后,给代理更多恢复时间
if isEOF && consecutiveEOFCount >= 2 {
    log.Printf("检测到代理问题,等待10秒让代理恢复...")
    time.Sleep(proxyRecoveryWaitTime)
}

// 连续3次EOF后,记录告警
if consecutiveEOFCount >= 3 {
    log.Printf("❌ 检测到连续%d次EOF错误,可能代理异常,建议检查ss-proxy容器", consecutiveEOFCount)
}
```

**效果:**
- 临时网络抖动: 2秒后自动重试
- 代理短暂异常: 10秒等待期让代理恢复
- 持续失败: 明确告警提示管理员介入

## 2. 自动监控和重启

### 监控脚本 ([monitor_proxy.sh](monitor_proxy.sh))

**功能:**
- 每5分钟自动检查代理健康状态
- 检测 ss-proxy 容器运行状态
- 分析日志中的 timeout/error 错误
- 分析后端日志中的 EOF/API失败
- 直接通过 SOCKS5 代理请求币安 `ping` 接口
- 连续失败3次后自动重启 ss-proxy (失败计数持久化在 `/root/nofx/logs/.proxy_monitor_state`)

**检查项目:**

1. **容器状态检查**
   ```bash
   docker ps | grep ss-proxy
   ```

2. **代理日志分析** (最近5分钟)
   ```bash
   # 超过10个 timeout/error 则判定异常
   docker logs ss-proxy --tail 50 --since 5m | grep -i "timeout\|error\|failed"
   ```

3. **后端API失败分析** (最近5分钟)
   ```bash
   # 超过5个 EOF/API失败 则判定异常
   docker logs nofx-trading --tail 100 --since 5m | grep -i "EOF\|获取持仓失败\|获取账户失败"
   ```

**自动恢复流程:**
```
检测异常 → 失败计数+1 → 连续3次失败 → 自动重启ss-proxy → 等待5秒稳定 → 重置计数
```

### 定时任务配置

```bash
# 查看当前 cron 任务
crontab -l

# 输出:
*/5 * * * * /root/nofx/monitor_proxy.sh       # 每5分钟检查一次代理
0 3 * * * /root/nofx/backup_db.sh             # 每天3点备份数据库
```

## 3. 日志和告警

### 监控日志位置
```bash
/root/nofx/logs/proxy_monitor.log
```

### 日志示例
```
[2025-11-11 10:59:48] =========================================
[2025-11-11 10:59:48] 🔍 开始代理健康检查
[2025-11-11 10:59:48] ✅ 代理运行正常
[2025-11-11 10:59:48] =========================================

# 异常情况:
[2025-11-11 11:05:00] ⚠️  检测到 ss-proxy 最近5分钟有 15 个错误
[2025-11-11 11:05:00] ⚠️  代理检查失败 (1/3)
[2025-11-11 11:10:00] ⚠️  代理检查失败 (2/3)
[2025-11-11 11:15:00] ❗ 连续失败 3 次,触发自动重启
[2025-11-11 11:15:01] 🔄 正在重启 ss-proxy...
[2025-11-11 11:15:06] ✅ ss-proxy 重启成功
[2025-11-11 11:15:06] 🚨 告警: 代理异常,已自动重启并恢复正常
```

### 告警扩展 (TODO)

当前告警仅记录日志,可扩展为:
- **钉钉群消息**
- **企业微信通知**
- **邮件告警**
- **短信通知**

修改 `monitor_proxy.sh` 中的 `send_alert()` 函数即可:

```bash
send_alert() {
    local message="$1"
    log "🚨 告警: $message"

    # 钉钉 webhook 示例
    curl -X POST "https://oapi.dingtalk.com/robot/send?access_token=YOUR_TOKEN" \
         -H 'Content-Type: application/json' \
         -d "{\"msgtype\": \"text\", \"text\": {\"content\": \"$message\"}}"
}
```

## 4. 手动操作

### 查看代理状态
```bash
# 查看容器状态
docker ps -a | grep ss-proxy

# 查看最近日志
docker logs ss-proxy --tail 50

# 查看后端API错误
docker logs nofx-trading --tail 100 | grep -i "EOF\|失败"
```

### 手动重启代理
```bash
docker restart ss-proxy

# 等待5秒后检查状态
sleep 5
docker logs nofx-trading --tail 20
```

### 手动运行监控脚本
```bash
/root/nofx/monitor_proxy.sh

# 查看监控日志
tail -f /root/nofx/logs/proxy_monitor.log
```

### 查看监控日志历史
```bash
# 查看最近50行
tail -50 /root/nofx/logs/proxy_monitor.log

# 实时监控
tail -f /root/nofx/logs/proxy_monitor.log

# 查找重启事件
grep "重启" /root/nofx/logs/proxy_monitor.log
```

### 连续失败计数状态文件

- 路径: `/root/nofx/logs/.proxy_monitor_state`
- 用途: 跨 cron 任务保存连续失败次数,确保 3 次失败后一定会被重启
- 重置: `echo 0 > /root/nofx/logs/.proxy_monitor_state` 或在脚本一次成功检查后自动重置

## 5. 缓存降级机制

当 API 调用失败时,系统会尝试使用缓存数据:

```go
// 缓存有效期配置
cacheDuration: 15秒          // 正常缓存时间
cacheStaleGracePeriod: 5分钟 // 失败降级缓存宽限期
```

**降级逻辑:**
1. API调用成功 → 更新缓存,使用最新数据
2. API调用失败 + 缓存未过期(15秒内) → 使用缓存数据
3. API调用失败 + 缓存过期但在宽限期内(5分钟) → 使用过期缓存,记录警告
4. API调用失败 + 缓存超过宽限期 → 返回错误,停止交易决策

这样即使代理短暂中断,系统也能继续运行一段时间。

## 6. 预防措施总结

| 防护层级 | 机制 | 响应时间 | 恢复能力 |
|---------|------|---------|----------|
| **L1: 代码重试** | 5次重试 + 智能延迟 | 2-30秒 | 临时抖动 |
| **L2: 缓存降级** | 15秒缓存 + 5分钟宽限期 | 立即 | 短暂中断 |
| **L3: 自动重启** | 每5分钟检查 + 3次确认 | 5-15分钟 | 持续故障 |
| **L4: 人工介入** | 日志告警 + 监控通知 | 人工响应 | 严重故障 |

## 7. 常见问题

**Q: 为什么EOF错误还会出现?**
A: EOF通常是网络层问题(代理超时/断连),短暂出现是正常的。只要不是持续大量出现,重试机制会自动处理。

**Q: 监控脚本会影响性能吗?**
A: 不会。脚本每5分钟运行一次,只读取最近的日志(不超过100行),执行时间不到1秒。

**Q: 如何调整监控敏感度?**
A: 修改 `monitor_proxy.sh` 中的参数:
```bash
MAX_FAILURES=3        # 触发重启前的失败次数 (1-5)
RECENT_ERRORS=10      # 代理日志错误阈值 (5-20)
BACKEND_ERRORS=5      # 后端API错误阈值 (3-10)
```

**Q: 可以禁用自动重启吗?**
A: 可以,删除 cron 任务:
```bash
crontab -e
# 删除或注释掉这行:
# */5 * * * * /root/nofx/monitor_proxy.sh
```

**Q: 如何查看历史重启记录?**
A:
```bash
grep "重启\|restart" /root/nofx/logs/proxy_monitor.log
```

## 8. 维护建议

1. **每周检查一次日志**
   ```bash
   tail -100 /root/nofx/logs/proxy_monitor.log
   ```

2. **每月清理旧日志** (保留最近30天)
   ```bash
   find /root/nofx/logs -name "*.log" -mtime +30 -delete
   ```

3. **关注告警频率**
   - 每天重启1-2次: 正常 (网络波动)
   - 每小时重启1次: 需要关注 (代理不稳定)
   - 每10分钟重启1次: 严重问题 (检查上游SS服务器)

4. **备用方案**
   如果代理持续不稳定,考虑:
   - 更换更稳定的SS服务器
   - 使用多个代理做负载均衡
   - 迁移到支持直连币安的服务器地区

---

**最后更新:** 2025-11-11
**维护者:** AI Trading System
