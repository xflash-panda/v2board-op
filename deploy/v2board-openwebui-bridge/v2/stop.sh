#!/bin/bash

echo "正在停止Open WebUI OIDC容器..."
docker-compose down

echo "是否要删除其他可能存在的相关容器？(y/n)"
read -r answer

if [ "$answer" = "y" ] || [ "$answer" = "Y" ]; then
    echo "正在检查并删除其他Open WebUI OIDC相关容器..."
    remaining_containers=$(docker ps -a | grep "open-webui-oidc" | awk '{print $1}')
    if [ -n "$remaining_containers" ]; then
        docker rm $remaining_containers
        echo "其他相关容器已删除"
    else
        echo "没有找到其他相关容器"
    fi
fi

echo "操作完成"