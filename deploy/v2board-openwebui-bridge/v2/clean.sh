#!/bin/bash

echo "===== Open WebUI OIDC 清理工具 ====="
echo "请选择清理选项："
echo "1. 停止并删除容器"
echo "2. 停止并删除容器和镜像"
echo "3. 停止并删除容器、镜像和数据卷"
echo "4. 退出"
echo "请输入选项 (1-4):"
read -r option

case $option in
    1)
        echo "正在停止并删除容器..."
        docker-compose down
        echo "正在删除所有相关容器..."
        docker rm $(docker ps -a | grep "open-webui-oidc" | awk '{print $1}') 2>/dev/null || true
        echo "容器已停止并删除"
        ;;
    2)
        echo "正在停止并删除容器..."
        docker-compose down
        echo "正在删除所有相关容器..."
        docker rm $(docker ps -a | grep "open-webui-oidc" | awk '{print $1}') 2>/dev/null || true
        echo "正在删除镜像..."
        docker rmi $(docker images | grep "v2board-openwebui-bridge_open-webui-oidc" | awk '{print $3}')
        echo "容器和镜像已删除"
        ;;
    3)
        echo "警告：此操作将删除所有数据！"
        echo "确定要继续吗？(y/n)"
        read -r confirm
        if [ "$confirm" = "y" ] || [ "$confirm" = "Y" ]; then
            # 加载环境变量获取数据目录
            if [ -f .env ]; then
                source .env
            fi

            echo "正在停止并删除容器..."
            docker-compose down
            echo "正在删除所有相关容器..."
            docker rm $(docker ps -a | grep "open-webui-oidc" | awk '{print $1}') 2>/dev/null || true

            echo "正在删除镜像..."
            docker rmi $(docker images | grep "v2board-openwebui-bridge_open-webui-oidc" | awk '{print $3}')

            # 删除数据目录
            DATA_DIR=${DATA_DIR:-./data}
            if [ -d "$DATA_DIR" ]; then
                echo "正在删除数据目录: $DATA_DIR"
                rm -rf "$DATA_DIR"
            fi

            echo "容器、镜像和数据已完全删除"
        else
            echo "操作已取消"
        fi
        ;;
    4)
        echo "退出"
        exit 0
        ;;
    *)
        echo "无效选项"
        exit 1
        ;;
esac