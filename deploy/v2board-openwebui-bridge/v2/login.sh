#!/bin/bash

echo "===== 登录到 Open WebUI OIDC 容器 bash shell ====="

# 检查容器是否运行
if ! docker ps | grep -q "open-webui-oidc"; then
    echo "错误: open-webui-oidc 容器未运行!"
    echo "请先启动容器后再尝试登录。"
    exit 1
fi

echo "正在连接到容器 bash shell..."
echo "输入 'exit' 退出容器 shell"
echo "-----------------------------------"

# 连接到容器的 bash shell
docker exec -it open-webui-oidc /bin/bash

echo "已退出容器 shell"
