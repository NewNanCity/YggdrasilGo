<div align="center">

# 🎯 Yggdrasil API Server (Go)

<p align="center">
  <img src="https://img.shields.io/github/go-mod/go-version/NewNanCity/YggdrasilGo?logo=go" alt="Go Version">
  <img src="https://img.shields.io/github/license/NewNanCity/YggdrasilGo" alt="License">
  <img src="https://img.shields.io/github/v/release/NewNanCity/YggdrasilGo?logo=github" alt="Release">
  <img src="https://img.shields.io/github/actions/workflow/status/NewNanCity/YggdrasilGo/release.yml?logo=github-actions" alt="Build Status">
</p>

<p align="center">
  <img src="https://img.shields.io/github/stars/NewNanCity/YggdrasilGo?logo=github" alt="GitHub Stars">
  <img src="https://img.shields.io/github/forks/NewNanCity/YggdrasilGo?logo=github" alt="GitHub Forks">
  <img src="https://img.shields.io/github/issues/NewNanCity/YggdrasilGo?logo=github" alt="GitHub Issues">
</p>

<h3>🚀 高性能的 Minecraft Yggdrasil API 服务器实现</h3>
<p>使用 Go 语言编写，完全兼容 Minecraft 官方认证协议，支持 BlessingSkin 皮肤站</p>

[📖 文档](https://github.com/NewNanCity/YggdrasilGo/wiki) •
[🚀 快速开始](#-快速开始) •
[🐳 Docker](#-docker-部署) •
[📊 监控](#-性能监控) •
[🤝 贡献](#-贡献)

</div>

---

## ✨ 核心特性

<table>
<tr>
<td width="50%">

### 🚀 **高性能架构**
- 基于 **Gin** 框架，支持高并发
- **JWT 优先验证**架构
- **多层缓存**优化响应速度
- **对象池**减少内存分配
- **Sonic JSON**高性能序列化

</td>
<td width="50%">

### 🔐 **安全可靠**
- 完整的 **JWT Token** 管理
- **RSA 数字签名**支持
- **速率限制**防护
- **CORS** 跨域支持
- **用户状态验证**

</td>
</tr>
<tr>
<td width="50%">

### 💾 **多存储后端**
- 📁 **文件存储** - 轻量级部署
- 🗄️ **数据库存储** - 高可用性
- 🎨 **BlessingSkin** - 完全兼容

</td>
<td width="50%">

### 🗄️ **智能缓存**
- 🧠 **内存缓存** - 极速响应
- 📁 **文件缓存** - Laravel 兼容
- 🔴 **Redis 缓存** - 分布式支持
- 🗃️ **数据库缓存** - 持久化存储

</td>
</tr>
</table>

### 🎯 **完全兼容**
- ✅ **100% 兼容** Minecraft 官方 Yggdrasil API
- ✅ **authlib-injector** 完全支持
- ✅ **BlessingSkin** 数据库兼容
- ✅ **Laravel 缓存**格式兼容

### 📊 **监控与运维**
- 📈 **实时性能监控** - QPS、响应时间、错误率
- 🏥 **健康检查** - 自动故障检测
- 📋 **结构化日志** - 便于问题排查
- 🔧 **优雅关闭** - 零停机部署

## 🚀 快速开始

<details>
<summary><b>📦 方式一：下载预编译二进制文件（推荐）</b></summary>

1. 前往 [Releases](https://github.com/NewNanCity/YggdrasilGo/releases) 页面
2. 下载适合您系统的二进制文件
3. 解压并运行：

```bash
# Linux/macOS
chmod +x yggdrasil-api-server-*
./yggdrasil-api-server-* -version

# Windows
yggdrasil-api-server-*.exe -version
```

</details>

<details>
<summary><b>🔨 方式二：从源码编译</b></summary>

```bash
# 克隆仓库
git clone https://github.com/NewNanCity/YggdrasilGo.git
cd yggdrasil-api-go

# 安装依赖
go mod download

# 编译
go build -o yggdrasil-api-server main.go
```

</details>

<details>
<summary><b>🐳 方式三：Docker 部署</b></summary>

```bash
# 拉取镜像
docker pull ghcr.io/NewNanCity/YggdrasilGo:latest

# 运行容器
docker run -d \
  --name yggdrasil-api \
  -p 8080:8080 \
  -v $(pwd)/conf:/app/conf:ro \
  -v $(pwd)/keys:/app/keys:ro \
  ghcr.io/NewNanCity/YggdrasilGo:latest
```

</details>

### ⚙️ 配置服务器

1. **复制配置文件**：
   ```bash
   cp conf/example.yml conf/config.yml
   ```

2. **生成密钥对**：
   ```bash
   mkdir -p keys
   openssl genrsa -out keys/private.pem 2048
   openssl rsa -in keys/private.pem -pubout -out keys/public.pem
   ```

3. **编辑配置文件** `conf/config.yml`，根据需要修改数据库连接、缓存设置等

4. **启动服务器**：
   ```bash
   ./yggdrasil-api-server -config conf/config.yml
   ```

🎉 **服务器启动成功！** 访问 http://localhost:8080 查看 API 状态

## 📋 配置说明

<div align="center">

### 🎛️ 配置概览

| 配置类型   | 说明              | 支持选项                           |
| ---------- | ----------------- | ---------------------------------- |
| 🗄️ **存储** | 用户数据存储      | `file` `blessing_skin` `database`  |
| 🗃️ **缓存** | Token/Session缓存 | `memory` `redis` `file` `database` |
| 🔐 **认证** | JWT和RSA配置      | 自定义密钥、过期时间               |
| 🌐 **网络** | 服务器和CORS      | 端口、域名白名单                   |

</div>

<details>
<summary><b>🔧 基础配置示例</b></summary>

```yaml
# 服务器配置
server:
  host: "0.0.0.0"
  port: 8080
  debug: false
  base_url: ""
  api_location: "https://api.example.com/"  # ALI: 明确告知客户端后续应访问的 API Base URL

# 认证配置
auth:
  jwt_secret: "your-super-secret-jwt-key-change-in-production"
  token_expiration: 72h0m0s
  tokens_limit: 10
  require_verification: false

# 速率限制
rate_limit:
  enabled: true
  auth_interval: 1s

# 存储配置
storage:
  type: "file"  # 可选: file, memory, blessing_skin
  file_options:
    data_dir: "data"

# 缓存配置
cache:
  token:
    type: "memory"  # 可选: memory, redis, file, database
    options: {}
  session:
    type: "memory"
    options: {}

  # 响应缓存（提升性能）
  response:
    enabled: true
    api_metadata: true
    error_responses: true
    cache_duration: 5m

  # 用户信息缓存
  user:
    enabled: true
    duration: 5m
    max_users: 500

# 材质配置
texture:
  base_url: "https://your-domain.com"
  upload_enabled: false
  max_file_size: 2097152  # 2MB
  allowed_types: ["image/png", "image/jpeg"]

# Yggdrasil API 配置
yggdrasil:
  meta:
    server_name: "Yggdrasil API Server (Go)"
    implementation_name: "yggdrasil-api-go"
    implementation_version: "1.0.0"
    links:
      homepage: ""  # 留空则自动根据请求Host生成
      register: ""

  # 皮肤域名白名单
  skin_domains:
    - "localhost"
    - ".localhost"        # 通配符域名
    - "127.0.0.1"
    - "192.168.0.0/16"    # CIDR网段
    - "10.0.0.0/8"

  # 密钥文件路径
  keys:
    private_key_path: "keys/private.pem"
    public_key_path: "keys/public.pem"

  # 功能开关
  feature_non_email_login: true
  feature_legacy_skin_api: true
  feature_username_check: true
  feature_profile_key: true

# 性能监控
monitoring:
  enabled: true
  metrics_endpoint: "/metrics"
  cache_stats: true

# 安全配置
security:
  cors:
    enabled: true
    allowed_origins: ["*"]
    allowed_methods: ["GET", "POST", "PUT", "DELETE", "OPTIONS"]
    allowed_headers: ["*"]
  max_request_size: "1MB"
  read_timeout: "30s"
  write_timeout: "30s"
```

</details>


### ALI（API Location Indicator）配置示例

如果你部署在反向代理/网关后，建议显式配置 `server.api_location`，确保客户端始终拿到稳定的 API 根地址：

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  base_url: ""
  api_location: "https://api.example.com/"
```

如果你希望按请求动态推导（基于请求的 scheme + host + base_url），可以留空：

```yaml
server:
  base_url: "/api/yggdrasil"
  api_location: ""
```

> 留空时，服务会根据请求自动生成类似 `https://your-host/api/yggdrasil/` 的地址。


## 🗄️ 存储配置

### 文件存储（推荐用于小型部署）

```yaml
storage:
  type: "file"
  file_options:
    data_dir: "data"
```

**特点**：
- ✅ 简单易用，无需数据库
- ✅ 适合小型服务器
- ❌ 不支持集群部署
- ❌ 密钥需要从配置文件读取

### BlessingSkin 存储（推荐用于现有BlessingSkin站点）

```yaml
storage:
  type: "blessing_skin"
  blessingskin_options:
    database_dsn: "user:password@tcp(localhost:3306)/blessingskin?charset=utf8mb4&parseTime=True&loc=Local"
    texture_base_url_override: false # false=从options读取site_url, true=使用配置文件的texture.base_url
    debug: false # 开启调试模式查看SQL查询

    # 安全配置 - 与BlessingSkin环境变量保持一致
    security:
      salt: "" # BlessingSkin通常不使用额外的salt，密码直接使用bcrypt
      pwd_method: "BCRYPT" # 与环境变量PWD_METHOD一致
      app_key: "base64:your_app_key_here" # 与环境变量APP_KEY一致
```

**特点**：
- ✅ 与BlessingSkin完全兼容
- ✅ 支持现有用户和角色
- ✅ 密钥从数据库options表读取
- ✅ 支持集群部署
- ❌ 需要MySQL数据库

### 数据库存储（暂未实现）

```yaml
storage:
  type: "database"
  database_options:
    dsn: "user:password@tcp(localhost:3306)/yggdrasil?charset=utf8mb4&parseTime=True&loc=Local"
```

## 🗄️ 缓存配置

### Redis 缓存（推荐用于生产环境）

```yaml
cache:
  token:
    type: "redis"
    options:
      redis_url: "redis://localhost:6379"
  session:
    type: "redis"
    options:
      redis_url: "redis://localhost:6379"
```

**特点**：
- ✅ 高性能，支持集群
- ✅ JWT优先验证架构
- ✅ 自动过期清理
- ✅ 支持持久化
- ❌ 需要Redis服务

### 数据库缓存（推荐用于中型部署）

```yaml
cache:
  token:
    type: "database"
    options:
      dsn: "user:password@tcp(localhost:3306)/cache?charset=utf8mb4&parseTime=True&loc=Local"
      table_prefix: "ygg_cache_"
      cleanup_interval: "5m"
      debug: false # 设置为true可以打印SQL调试日志
  session:
    type: "database"
    options:
      dsn: "user:password@tcp(localhost:3306)/cache?charset=utf8mb4&parseTime=True&loc=Local"
      table_prefix: "ygg_cache_"
      cleanup_interval: "5m"
      debug: false
```

**特点**：
- ✅ 可靠性高，支持事务
- ✅ JWT优先验证架构
- ✅ 定期清理过期数据
- ✅ 支持调试模式
- ❌ 性能略低于Redis

### 文件缓存（推荐用于小型部署）

```yaml
cache:
  token:
    type: "file"
    options:
      cache_dir: "storage/framework/cache/tokens"
  session:
    type: "file"
    options:
      cache_dir: "storage/framework/cache/sessions"
```

**特点**：
- ✅ 无需额外服务
- ✅ Laravel兼容格式
- ✅ JWT优先验证架构
- ❌ 不支持集群部署
- ❌ 性能相对较低

## 🏗️ JWT优先验证架构

本项目采用创新的JWT优先验证架构，大幅提升性能：

### 传统架构 vs JWT优先架构

```
传统架构：
客户端请求 → 查询缓存/数据库 → 验证Token → 返回结果
每次请求都需要查询存储

JWT优先架构：
客户端请求 → JWT本地验证（极快） → 按需查询缓存 → 返回结果
大部分请求无需查询存储，性能提升10倍以上
```

### 性能优势

- **Token验证**: JWT本地验证，无需查询数据库
- **缓存优化**: 只存储JWT中没有的信息（如ClientToken）
- **存储键优化**: 使用`userID:tokenID`作为键，提高查询效率
- **内存占用**: 大幅减少缓存内存占用

## 🐳 Docker 部署

<details>
<summary><b>🚀 快速部署（推荐）</b></summary>

使用 Docker Compose 一键部署完整环境：

```bash
# 克隆仓库
git clone https://github.com/NewNanCity/YggdrasilGo.git
cd yggdrasil-api-go

# 准备环境
cp .env.example .env
cp conf/example.yml conf/config.yml

# 编辑配置文件
nano .env
nano conf/config.yml

# 启动服务
docker-compose up -d
```

</details>

<details>
<summary><b>🔧 自定义部署</b></summary>

```bash
# 仅启动 API 服务器
docker run -d \
  --name yggdrasil-api \
  -p 8080:8080 \
  -v $(pwd)/conf:/app/conf:ro \
  -v $(pwd)/keys:/app/keys:ro \
  -v yggdrasil_storage:/app/storage \
  ghcr.io/NewNanCity/YggdrasilGo:latest

# 启动完整环境（包含监控）
docker-compose --profile monitoring up -d

# 启动带 Nginx 的环境
docker-compose --profile with-nginx up -d
```

</details>

### 🏥 健康检查

```bash
# 检查服务状态
curl http://localhost:8080/

# 查看性能指标
curl http://localhost:8080/metrics

# 查看容器状态
docker-compose ps

# 查看日志
docker-compose logs -f yggdrasil-api
```

## 🌐 API 文档

<div align="center">

### 📋 API 端点概览

| 类别       | 端点                                              | 方法 | 说明             |
| ---------- | ------------------------------------------------- | ---- | ---------------- |
| 🔐 **认证** | `/authserver/authenticate`                        | POST | 用户登录         |
| 🔐 **认证** | `/authserver/refresh`                             | POST | 刷新令牌         |
| 🔐 **认证** | `/authserver/validate`                            | POST | 验证令牌         |
| 🔐 **认证** | `/authserver/invalidate`                          | POST | 撤销令牌         |
| 🔐 **认证** | `/authserver/signout`                             | POST | 登出             |
| 🎮 **会话** | `/sessionserver/session/minecraft/join`           | POST | 客户端加入服务器 |
| 🎮 **会话** | `/sessionserver/session/minecraft/hasJoined`      | GET  | 服务端验证客户端 |
| 👤 **角色** | `/api/profiles/minecraft`                         | POST | 批量查询角色     |
| 👤 **角色** | `/sessionserver/session/minecraft/profile/{uuid}` | GET  | 获取角色档案     |
| 📊 **监控** | `/`                                               | GET  | API 元数据       |
| 📊 **监控** | `/metrics`                                        | GET  | 性能指标         |

</div>

<details>
<summary><b>📖 详细 API 文档</b></summary>

### 🔐 用户认证

#### POST /authserver/authenticate
用户登录认证

```json
// 请求
{
  "username": "user@example.com",
  "password": "password123",
  "agent": {
    "name": "Minecraft",
    "version": 1
  }
}

// 响应
{
  "accessToken": "jwt-token-here",
  "clientToken": "client-token-here",
  "availableProfiles": [
    {
      "id": "uuid-here",
      "name": "PlayerName"
    }
  ],
  "selectedProfile": {
    "id": "uuid-here",
    "name": "PlayerName"
  }
}
```

#### POST /authserver/refresh
刷新访问令牌

```json
// 请求
{
  "accessToken": "old-jwt-token",
  "clientToken": "client-token"
}

// 响应
{
  "accessToken": "new-jwt-token",
  "clientToken": "client-token"
}
```

### 🎮 游戏会话

#### POST /sessionserver/session/minecraft/join
客户端加入服务器

```json
// 请求
{
  "accessToken": "jwt-token",
  "selectedProfile": {
    "id": "player-uuid",
    "name": "PlayerName"
  },
  "serverId": "server-hash"
}

// 响应: 204 No Content
```

#### GET /sessionserver/session/minecraft/hasJoined
服务端验证客户端

```
GET /sessionserver/session/minecraft/hasJoined?username=PlayerName&serverId=server-hash
```

```json
// 响应
{
  "id": "player-uuid",
  "name": "PlayerName",
  "properties": [
    {
      "name": "textures",
      "value": "base64-encoded-texture-data",
      "signature": "rsa-signature"
    }
  ]
}
```

</details>

## 🔧 密钥管理

### BlessingSkin存储

对于BlessingSkin存储，密钥从数据库的`options`表读取：

- **私钥**: 从`ygg_private_key`字段读取
- **公钥**: 从私钥自动提取
- **配置**: 密钥文件路径可以留空

```yaml
yggdrasil:
  keys:
    private_key_path: "" # 留空，从BlessingSkin数据库读取
    public_key_path: ""  # 留空，从BlessingSkin数据库读取
```

### 其他存储类型

对于文件存储等其他类型，密钥从配置文件指定的路径读取：

```yaml
yggdrasil:
  keys:
    private_key_path: "keys/private.pem" # 必填
    public_key_path: "keys/public.pem"   # 必填
```

如果密钥文件不存在，服务器会自动生成新的密钥对。

## 📊 性能监控

<div align="center">

### 🎯 实时监控指标

| 指标类型     | 监控内容              | 访问方式   |
| ------------ | --------------------- | ---------- |
| 🚀 **性能**   | QPS、响应时间、错误率 | `/metrics` |
| 🗃️ **缓存**   | 命中率、内存使用      | `/metrics` |
| 🗄️ **数据库** | 查询次数、平均时间    | `/metrics` |
| 💾 **系统**   | 内存、GC、协程数      | `/metrics` |

</div>

<details>
<summary><b>📈 监控数据示例</b></summary>

访问 `/metrics` 端点获取详细的性能统计：

```json
{
  "performance": {
    "qps": 322.5,
    "avg_response_time_ms": 21.82,
    "error_rate": 0.0,
    "total_requests": 12847,
    "uptime_seconds": 3600
  },
  "cache_stats": {
    "token_cache": {
      "type": "redis",
      "hit_rate": 95.2,
      "total_requests": 10000,
      "cache_hits": 9520,
      "memory_usage_mb": 12.5
    },
    "session_cache": {
      "type": "redis",
      "active_sessions": 150,
      "hit_rate": 88.7
    },
    "uuid_cache": {
      "size": 500,
      "max_size": 1000,
      "hit_rate": 99.1
    }
  },
  "database": {
    "total_queries": 1250,
    "avg_query_time_ms": 5.2,
    "active_connections": 5,
    "max_connections": 100
  },
  "memory": {
    "heap_mb": 45.2,
    "system_mb": 67.8,
    "gc_count": 23,
    "goroutines": 15
  }
}
```

</details>

<details>
<summary><b>🔧 监控配置</b></summary>

启用详细监控和调试：

```yaml
# 性能监控配置
monitoring:
  enabled: true
  metrics_endpoint: "/metrics"
  cache_stats: true

# 数据库调试
storage:
  blessingskin_options:
    debug: true # 打印SQL调试日志

# 缓存调试
cache:
  token:
    type: "database"
    options:
      debug: true # 打印缓存操作日志
```

</details>

### 🎛️ Grafana 仪表板

使用 Docker Compose 启动完整监控环境：

```bash
# 启动监控环境
docker-compose --profile monitoring up -d

# 访问 Grafana
open http://localhost:3000
# 默认账号: admin / admin
```

监控面板包含：
- 📈 **QPS 和响应时间**趋势
- 🗃️ **缓存命中率**统计
- 🗄️ **数据库性能**监控
- 💾 **系统资源**使用情况

## 🧪 测试

### 运行完整测试

```bash
cd test
go run perfect_client.go
```

测试覆盖所有API端点：

- ✅ API元数据获取
- ✅ 角色查询（单个和批量）
- ✅ 用户认证（邮箱和角色名登录）
- ✅ 令牌管理（验证、刷新、撤销）
- ✅ 会话管理（Join/HasJoined）
- ✅ 角色档案获取
- ✅ 性能监控

### 测试结果示例

```
🎯 最终测试结果: 12/12 通过 (100.0%)
🎉 所有测试通过！Yggdrasil API服务器完全可用！

✨ 测试完成的功能:
  ✅ 用户认证（邮箱和角色名登录）
  ✅ 令牌管理（验证、刷新、撤销）
  ✅ 角色查询（单个和批量）
  ✅ 角色档案获取
  ✅ API元数据获取
  ✅ 性能监控
  ✅ 会话管理（Join/HasJoined）
```

## 🚀 部署建议

### 小型部署（< 100用户）

```yaml
storage:
  type: "file"

cache:
  token:
    type: "file"
  session:
    type: "file"
```

### 中型部署（100-1000用户）

```yaml
storage:
  type: "blessing_skin"

cache:
  token:
    type: "database"
  session:
    type: "database"
```

### 大型部署（> 1000用户）

```yaml
storage:
  type: "blessing_skin"

cache:
  token:
    type: "redis"
  session:
    type: "redis"
```

## 🔍 故障排除

### 常见问题

1. **密钥文件不存在**
   - 确保 `keys/` 目录存在
   - 服务器会自动生成密钥对

2. **数据库连接失败**
   - 检查数据库连接字符串
   - 确保数据库服务正在运行

3. **Redis连接失败**
   - 检查Redis连接配置
   - 确保Redis服务正在运行

4. **BlessingSkin兼容问题**
   - 检查安全配置是否与BlessingSkin一致
   - 确认密码加密方法正确

### 调试模式

启用调试模式获取详细日志：

```yaml
server:
  debug: true

storage:
  blessingskin_options:
    debug: true

cache:
  token:
    options:
      debug: true
```

## 🤝 贡献

<div align="center">

### 💝 感谢所有贡献者

</div>

我们欢迎各种形式的贡献！无论是 **Bug 报告**、**功能建议**、**代码贡献** 还是 **文档改进**。

<details>
<summary><b>🚀 如何贡献</b></summary>

### 1. 🐛 报告 Bug
- 使用 [Bug Report 模板](https://github.com/NewNanCity/YggdrasilGo/issues/new?template=bug_report.md)
- 提供详细的复现步骤
- 包含系统信息和日志

### 2. 💡 功能建议
- 使用 [Feature Request 模板](https://github.com/NewNanCity/YggdrasilGo/issues/new?template=feature_request.md)
- 描述使用场景和预期效果
- 考虑向后兼容性

### 3. 🔧 代码贡献
```bash
# 1. Fork 仓库
git clone https://github.com/your-username/yggdrasil-api-go.git
cd yggdrasil-api-go

# 2. 创建功能分支
git checkout -b feature/amazing-feature

# 3. 进行开发
go mod download
go test -v ./...
go build -o yggdrasil-api-server main.go

# 4. 提交更改
git commit -m "feat: add amazing feature"
git push origin feature/amazing-feature

# 5. 创建 Pull Request
```

### 4. 📚 文档贡献
- 改进 README 和 Wiki
- 添加代码注释
- 编写使用示例

</details>

### 🎯 贡献领域

| 领域           | 描述         | 难度 |
| -------------- | ------------ | ---- |
| 🐛 **Bug 修复** | 修复已知问题 | ⭐⭐   |
| 📊 **性能优化** | 提升响应速度 | ⭐⭐⭐  |
| 🔐 **安全增强** | 加强安全防护 | ⭐⭐⭐⭐ |
| 🌐 **国际化**   | 多语言支持   | ⭐⭐   |
| 📚 **文档完善** | 改进文档质量 | ⭐    |
| 🧪 **测试覆盖** | 增加测试用例 | ⭐⭐   |

## 📊 项目统计

<div align="center">

![GitHub repo size](https://img.shields.io/github/repo-size/NewNanCity/YggdrasilGo?style=for-the-badge)
![GitHub code size](https://img.shields.io/github/languages/code-size/NewNanCity/YggdrasilGo?style=for-the-badge)
![GitHub commit activity](https://img.shields.io/github/commit-activity/m/NewNanCity/YggdrasilGo?style=for-the-badge)
![GitHub last commit](https://img.shields.io/github/last-commit/NewNanCity/YggdrasilGo?style=for-the-badge)

</div>

## 📄 许可证

<div align="center">

**MIT License** - 详见 [LICENSE](LICENSE) 文件

```
Copyright (c) 2025 Gk0Wk

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.
```

</div>

## 🙏 致谢

<div align="center">

### 🌟 特别感谢

- [Minecraft](https://minecraft.net) - 游戏本体
- [Yggdrasil](https://wiki.vg/Authentication) - 认证协议规范
- [BlessingSkin](https://github.com/bs-community/blessing-skin-server) - 皮肤站参考实现
- [Gin](https://github.com/gin-gonic/gin) - 高性能 HTTP 框架
- [GORM](https://gorm.io/) - 优秀的 Go ORM 库

</div>

## 📞 联系与支持

<div align="center">

### 💬 获取帮助

[![GitHub Issues](https://img.shields.io/badge/GitHub-Issues-red?style=for-the-badge&logo=github)](https://github.com/NewNanCity/YggdrasilGo/issues)
[![GitHub Discussions](https://img.shields.io/badge/GitHub-Discussions-blue?style=for-the-badge&logo=github)](https://github.com/NewNanCity/YggdrasilGo/discussions)
[![Wiki](https://img.shields.io/badge/GitHub-Wiki-green?style=for-the-badge&logo=github)](https://github.com/NewNanCity/YggdrasilGo/wiki)

### 🚀 快速链接

- 📖 [完整文档](https://github.com/NewNanCity/YggdrasilGo/wiki)
- 🐛 [报告 Bug](https://github.com/NewNanCity/YggdrasilGo/issues/new?template=bug_report.md)
- 💡 [功能建议](https://github.com/NewNanCity/YggdrasilGo/issues/new?template=feature_request.md)
- 🤝 [参与讨论](https://github.com/NewNanCity/YggdrasilGo/discussions)

</div>

---

<div align="center">

**⭐ 如果这个项目对你有帮助，请给它一个 Star！⭐**

Made with ❤️ by [Gk0Wk](https://github.com/Gk0Wk)

</div>
