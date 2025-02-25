#!/bin/bash

# 检查.env文件是否存在
if [ ! -f .env ]; then
    echo "未找到.env文件，将从.env.example创建..."
    if [ -f .env.example ]; then
        cp .env.example .env
        echo ".env文件已创建，请编辑配置后重新运行此脚本"
        echo "命令: nano .env"
        exit 1
    else
        echo "错误：.env.example文件不存在！"
        exit 1
    fi
fi

# 加载环境变量
source .env

# 确保数据目录存在
DATA_DIR=${DATA_DIR:-./data}
if [[ "$DATA_DIR" == /* ]]; then
    # 绝对路径
    mkdir -p "$DATA_DIR"
else
    # 相对路径
    mkdir -p "$(pwd)/$DATA_DIR"
fi

echo "数据目录: $DATA_DIR"

# 启动容器
echo "正在启动Open WebUI OIDC容器..."
docker-compose up -d

# 检查容器状态
echo "检查容器状态..."
sleep 3
docker-compose ps

# 显示访问信息
PORT=${WEBUI_PORT:-3080}
echo ""
echo "Open WebUI OIDC已启动！"
echo "您可以通过以下地址访问："
echo "http://localhost:$PORT"
echo ""
echo "查看日志："
echo "docker-compose logs -f open-webui-oidc"