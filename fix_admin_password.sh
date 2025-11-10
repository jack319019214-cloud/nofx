#!/bin/bash
# 修复 admin 用户密码的脚本
# 每次容器重启后运行此脚本来恢复密码

DB_PATH="/root/nofx/config.db"
ADMIN_EMAIL="admin@admin.com"
ADMIN_PASS_HASH='$2a$10$Vll7AlSknhdUxLsFDlzkVOpCb/IDNkVoeRJtNeYVED8TbWgQiUX/i'

echo "🔧 检查 admin 用户状态..."

# 检查密码哈希长度
HASH_LEN=$(sqlite3 "$DB_PATH" "SELECT length(password_hash) FROM users WHERE id='admin'")

if [ "$HASH_LEN" -eq "0" ] || [ "$HASH_LEN" -lt "60" ]; then
    echo "⚠️  检测到 admin 密码异常 (长度: $HASH_LEN)，正在修复..."

    # 修复密码和邮箱
    sqlite3 "$DB_PATH" << EOF
UPDATE users
SET email = '$ADMIN_EMAIL',
    password_hash = '$ADMIN_PASS_HASH'
WHERE id = 'admin';
EOF

    echo "✅ admin 密码已修复"
    echo "📧 邮箱: $ADMIN_EMAIL"
    echo "🔑 密码: admin123"
else
    echo "✅ admin 密码正常 (哈希长度: $HASH_LEN)"
fi

# 显示当前用户状态
echo ""
echo "📊 当前用户状态:"
sqlite3 "$DB_PATH" "SELECT id, email, length(password_hash) as hash_len FROM users"

echo ""
echo "🤖 交易员数量:"
sqlite3 "$DB_PATH" "SELECT user_id, COUNT(*) as count FROM traders GROUP BY user_id"
