# Open WebUI OIDC 集成

本项目提供了一个支持OIDC认证的Open WebUI Docker镜像，可以与V2Board等支持OIDC的身份提供者集成。

## 快速开始

### 1. 配置环境变量

复制`.env.example`文件为`.env`，并根据您的实际情况修改：

```bash
cp .env.example .env
nano .env  # 或使用您喜欢的编辑器
```

### 2. 构建并启动容器

```bash
docker-compose up -d
```

或者使用提供的启动脚本：

```bash
chmod +x start.sh
./start.sh
```

### 3. 访问Open WebUI

服务启动后，您可以通过以下地址访问Open WebUI：

```
http://localhost:3080  # 或您在.env中配置的端口
```

### 4. 停止和清理

停止服务：

```bash
docker-compose down
```

或使用提供的停止脚本（可选择是否删除镜像）：

```bash
chmod +x stop.sh
./stop.sh
```

完全清理（包括容器、镜像和数据）：

```bash
chmod +x clean.sh
./clean.sh
```

## OIDC配置说明

### V2Board OIDC配置

您需要在V2Board中配置OIDC提供者：

1. 登录到您的V2Board管理面板
2. 配置OIDC提供者，并记录以下信息：
   - 客户端ID (Client ID)
   - 客户端密钥 (Client Secret)
   - OIDC配置端点URL (通常为`https://your-v2board-domain/.well-known/openid-configuration`)

### 重要环境变量说明

| 环境变量 | 说明 | 默认值 |
|---------|------|-------|
| WEBUI_PORT | Web界面访问端口 | 3080 |
| DATA_DIR | 数据存储目录 | ./data |
| OPENID_PROVIDER_URL | OIDC提供者配置URL | - |
| OAUTH_CLIENT_ID | OIDC客户端ID | - |
| OAUTH_CLIENT_SECRET | OIDC客户端密钥 | - |
| OPENID_REDIRECT_URI | 认证后的回调URL | - |
| ENABLE_OAUTH_SIGNUP | 是否允许通过OAuth注册新账号 | true |
| OAUTH_MERGE_ACCOUNTS_BY_EMAIL | 是否允许通过邮箱合并账号 | true |

## 角色和组配置

如果您的OIDC提供者支持角色和组信息，您可以配置以下参数：

### 角色管理

| 环境变量 | 说明 | 默认值 |
|---------|------|-------|
| ENABLE_OAUTH_ROLE_MANAGEMENT | 启用角色管理 | true |
| OAUTH_ROLES_CLAIM | 角色信息在令牌中的字段名 | roles |
| OAUTH_ALLOWED_ROLES | 允许登录的角色列表 | user,member |
| OAUTH_ADMIN_ROLES | 管理员角色列表 | admin |

### 组管理

| 环境变量 | 说明 | 默认值 |
|---------|------|-------|
| ENABLE_OAUTH_GROUP_MANAGEMENT | 启用组管理 | true |
| OAUTH_GROUP_CLAIM | 组信息在令牌中的字段名 | groups |

## 数据持久化

默认情况下，Open WebUI的数据会存储在`DATA_DIR`环境变量指定的目录中（默认为`./data`）。您可以修改此环境变量来自定义数据存储位置：

```
# 使用相对路径
DATA_DIR=./my-data

# 或使用绝对路径
DATA_DIR=/opt/openwebui/data
```

确保指定的目录具有适当的权限，以便Docker容器可以读写数据。

## 脚本说明

本项目提供了几个实用脚本：

| 脚本 | 说明 |
|------|------|
| start.sh | 启动服务，自动创建数据目录和环境配置 |
| stop.sh | 停止服务，可选择是否删除镜像 |
| clean.sh | 清理工具，提供多种清理选项（容器、镜像、数据） |

## 故障排除

如果遇到问题，请检查以下几点：

1. 确保OIDC提供者配置正确
2. 检查环境变量是否正确设置
3. 查看Docker容器日志：

```bash
docker-compose logs open-webui-oidc
```

## 参考资料

- [Open WebUI SSO文档](https://docs.openwebui.com/features/sso/)
- [Open WebUI环境变量配置](https://docs.openwebui.com/getting-started/env-configuration/)