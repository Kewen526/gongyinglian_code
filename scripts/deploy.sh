#!/bin/bash
#
# 供应链管理系统 - 一键部署脚本
# 使用方式: bash deploy.sh
#
# 前置要求:
#   1. 服务器已安装 Go 1.21+
#   2. 服务器已安装 MySQL 并已执行 docs/schema.sql 建表
#   3. 服务器已安装 Elasticsearch（可选，不装则搜索功能不可用）
#   4. 服务器已安装 git
#

set -e

# ============================================================
# 配置区 - 根据你的服务器实际情况修改
# ============================================================
REPO_URL="https://github.com/Kewen526/gongyinglian_code.git"
BRANCH="main"
DEPLOY_DIR="/opt/supply-chain"
APP_NAME="supply-chain"
APP_PORT=8080

# MySQL 配置
MYSQL_USER="root"
MYSQL_PASS="password"
MYSQL_HOST="127.0.0.1"
MYSQL_PORT="3306"
MYSQL_DB="supply_chain"

# Elasticsearch 配置
ES_HOST="http://127.0.0.1:9200"

# JWT 配置
JWT_SECRET="supply-chain-jwt-$(openssl rand -hex 16)"

# ============================================================
# 开始部署
# ============================================================

echo "=========================================="
echo "  供应链管理系统 - 一键部署"
echo "=========================================="

# 1. 检查 Go 环境
echo ""
echo "[1/7] 检查 Go 环境..."
if ! command -v go &> /dev/null; then
    echo "❌ Go 未安装，正在安装 Go 1.21..."
    wget -q https://go.dev/dl/go1.21.13.linux-amd64.tar.gz -O /tmp/go.tar.gz
    sudo rm -rf /usr/local/go
    sudo tar -C /usr/local -xzf /tmp/go.tar.gz
    rm /tmp/go.tar.gz
    export PATH=$PATH:/usr/local/go/bin
    echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
    echo "✅ Go 安装完成: $(go version)"
else
    echo "✅ $(go version)"
fi

# 2. 拉取代码
echo ""
echo "[2/7] 拉取代码..."
if [ -d "$DEPLOY_DIR" ]; then
    echo "   目录已存在，拉取最新代码..."
    cd "$DEPLOY_DIR"
    git fetch origin "$BRANCH"
    git reset --hard "origin/$BRANCH"
else
    echo "   克隆仓库..."
    sudo mkdir -p "$DEPLOY_DIR"
    sudo chown "$(whoami):$(whoami)" "$DEPLOY_DIR"
    git clone -b "$BRANCH" "$REPO_URL" "$DEPLOY_DIR"
    cd "$DEPLOY_DIR"
fi
echo "✅ 代码已更新到最新"

# 3. 生成配置文件
echo ""
echo "[3/7] 生成配置文件..."
MYSQL_DSN="${MYSQL_USER}:${MYSQL_PASS}@tcp(${MYSQL_HOST}:${MYSQL_PORT})/${MYSQL_DB}?charset=utf8mb4&parseTime=True&loc=Local"

cat > "${DEPLOY_DIR}/configs/config.json" <<CFGEOF
{
  "server": {
    "port": ${APP_PORT}
  },
  "mysql": {
    "dsn": "${MYSQL_DSN}",
    "max_open_conns": 100,
    "max_idle_conns": 20,
    "conn_max_lifetime_minutes": 30
  },
  "elasticsearch": {
    "addresses": ["${ES_HOST}"],
    "product_index": "products"
  },
  "jwt": {
    "secret": "${JWT_SECRET}",
    "expire_hour": 24
  },
  "oss": {
    "endpoint": "oss-cn-hangzhou.aliyuncs.com",
    "access_key_id": "",
    "access_key_secret": "",
    "bucket_name": ""
  }
}
CFGEOF
echo "✅ 配置文件已生成: configs/config.json"

# 4. 下载依赖 & 编译
echo ""
echo "[4/7] 下载依赖并编译..."
cd "$DEPLOY_DIR"
export GOPROXY=https://goproxy.cn,direct
go mod tidy
go build -o "${APP_NAME}" ./cmd/server/
echo "✅ 编译完成: ${DEPLOY_DIR}/${APP_NAME}"

# 5. 创建 systemd 服务
echo ""
echo "[5/7] 创建系统服务..."
sudo tee /etc/systemd/system/${APP_NAME}.service > /dev/null <<SVCEOF
[Unit]
Description=Supply Chain Management System
After=network.target mysql.service

[Service]
Type=simple
User=$(whoami)
WorkingDirectory=${DEPLOY_DIR}
ExecStart=${DEPLOY_DIR}/${APP_NAME}
Restart=always
RestartSec=5
LimitNOFILE=65536

Environment=GIN_MODE=release

[Install]
WantedBy=multi-user.target
SVCEOF

sudo systemctl daemon-reload
echo "✅ 系统服务已创建"

# 6. 启动服务
echo ""
echo "[6/7] 启动服务..."
sudo systemctl stop ${APP_NAME} 2>/dev/null || true
sudo systemctl enable ${APP_NAME}
sudo systemctl start ${APP_NAME}
sleep 2

if sudo systemctl is-active --quiet ${APP_NAME}; then
    echo "✅ 服务启动成功"
else
    echo "❌ 服务启动失败，查看日志:"
    sudo journalctl -u ${APP_NAME} -n 20 --no-pager
    exit 1
fi

# 7. 验证
echo ""
echo "[7/7] 验证服务..."
sleep 1
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:${APP_PORT}/api/v1/login 2>/dev/null || echo "000")

if [ "$HTTP_CODE" != "000" ]; then
    echo "✅ 服务响应正常 (HTTP ${HTTP_CODE})"
else
    echo "⚠️  服务可能还在启动中，请稍后手动验证"
fi

# 完成
echo ""
echo "=========================================="
echo "  ✅ 部署完成!"
echo "=========================================="
echo ""
echo "  服务地址:  http://$(hostname -I | awk '{print $1}'):${APP_PORT}"
echo "  项目目录:  ${DEPLOY_DIR}"
echo "  配置文件:  ${DEPLOY_DIR}/configs/config.json"
echo ""
echo "  超级管理员账号: admin"
echo "  超级管理员密码: admin123"
echo "  ⚠️  请登录后尽快修改密码!"
echo ""
echo "  常用命令:"
echo "    查看状态:  sudo systemctl status ${APP_NAME}"
echo "    查看日志:  sudo journalctl -u ${APP_NAME} -f"
echo "    重启服务:  sudo systemctl restart ${APP_NAME}"
echo "    停止服务:  sudo systemctl stop ${APP_NAME}"
echo ""
echo "  测试登录:"
echo "    curl -X POST http://127.0.0.1:${APP_PORT}/api/v1/login \\"
echo "      -H 'Content-Type: application/json' \\"
echo "      -d '{\"username\":\"admin\",\"password\":\"admin123\"}'"
echo "=========================================="
