# Open WebUI OIDC 集成

本项目提供了一个支持OIDC认证的Open WebUI Docker镜像，可以与V2Board等支持OIDC的身份提供者集成。

## 快速开始

### 1. 环境准备

确保您的系统已安装：
- Docker
- Docker Compose
- Git

### 2. 配置环境变量

复制`.env.example`文件为`.env`，并根据您的实际情况修改：

```bash
cp .env.example .env
nano .env  # 或使用您喜欢的编辑器
```

主要配置项说明：
- `WEBUI_PORT`: Web界面访问端口（默认3080）
- `OPENID_PROVIDER_URL`: V2Board的OIDC配置URL
- `OAUTH_CLIENT_ID`: OIDC客户端ID
- `OAUTH_CLIENT_SECRET`: OIDC客户端密钥
- `OPENID_REDIRECT_URI`: 认证后的回调URL

### 3. 启动服务

使用提供的启动脚本：

```bash
chmod +x start.sh
./start.sh
```

脚本会自动：
- 检查并创建必要的配置文件
- 创建数据目录
- 启动Docker容器
- 显示访问信息

### 4. 访问Open WebUI

服务启动后，您可以通过以下地址访问Open WebUI：

```
http://localhost:3080  # 或您在.env中配置的端口
```

## 管理命令

### 停止服务

```bash
./stop.sh
```

### 重启服务

```bash
./restart.sh
```

### 清理服务

```bash
./clean.sh
```

清理选项包括：
1. 停止并删除容器
2. 停止并删除容器和镜像
3. 停止并删除容器、镜像和数据卷

### 登录容器

```bash
./login.sh
```

## OIDC配置说明

### V2Board OIDC配置

1. 登录到您的V2Board管理面板
2. 配置OIDC提供者，记录以下信息：
   - 客户端ID (Client ID)
   - 客户端密钥 (Client Secret)
   - OIDC配置端点URL

### 角色和组配置

#### 角色管理
- `ENABLE_OAUTH_ROLE_MANAGEMENT`: 是否启用角色管理
- `OAUTH_ROLES_CLAIM`: 角色信息在令牌中的字段名
- `OAUTH_ALLOWED_ROLES`: 允许登录的角色列表
- `OAUTH_ADMIN_ROLES`: 管理员角色列表

#### 组管理
- `ENABLE_OAUTH_GROUP_MANAGEMENT`: 是否启用组管理
- `OAUTH_GROUP_CLAIM`: 组信息在令牌中的字段名

## 数据持久化

数据默认存储在`DATA_DIR`环境变量指定的目录中（默认为`./data`）。您可以修改此环境变量来自定义数据存储位置：

```bash
# 使用相对路径
DATA_DIR=./my-data

# 或使用绝对路径
DATA_DIR=/opt/openwebui/data
```

## 故障排除

如果遇到问题，请检查：

1. 容器日志：
```bash
docker-compose logs -f open-webui-oidc
```

2. 确保OIDC提供者配置正确
3. 检查环境变量是否正确设置
4. 确认端口未被占用

## 参考资料

- [Open WebUI SSO文档](https://docs.openwebui.com/features/sso/)
- [Open WebUI环境变量配置](https://docs.openwebui.com/getting-started/env-configuration/)