#!/bin/bash
# 数据库自动备份脚本
# 定期备份 config.db 以防数据丢失

BACKUP_DIR="/root/nofx/backups"
DB_FILE="/root/nofx/config.db"
DATE=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="$BACKUP_DIR/config_${DATE}.db"

# 创建备份目录
mkdir -p "$BACKUP_DIR"

# 备份数据库
cp "$DB_FILE" "$BACKUP_FILE"

# 只保留最近7天的备份
find "$BACKUP_DIR" -name "config_*.db" -mtime +7 -delete

echo "✅ 数据库备份成功: $BACKUP_FILE"

# 显示备份列表
echo "📋 现有备份:"
ls -lh "$BACKUP_DIR"
