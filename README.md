# Yggdrasil API Server (Go)

[![Go Version](https://img.shields.io/github/go-mod/go-version/NewNanCity/YggdrasilGo?logo=go)](go.mod)
[![License](https://img.shields.io/github/license/NewNanCity/YggdrasilGo)](LICENSE)
[![Release](https://img.shields.io/github/v/release/NewNanCity/YggdrasilGo?logo=github)](https://github.com/NewNanCity/YggdrasilGo/releases/tag/v0.0.14)
[![Build Test](https://img.shields.io/github/actions/workflow/status/NewNanCity/YggdrasilGo/build-test.yml?branch=main&logo=github-actions&label=build)](https://github.com/NewNanCity/YggdrasilGo/actions/workflows/build-test.yml)
[![Stars](https://img.shields.io/github/stars/NewNanCity/YggdrasilGo?logo=github)](https://github.com/NewNanCity/YggdrasilGo/stargazers)
[![Forks](https://img.shields.io/github/forks/NewNanCity/YggdrasilGo?logo=github)](https://github.com/NewNanCity/YggdrasilGo/forks)
[![Issues](https://img.shields.io/github/issues/NewNanCity/YggdrasilGo?logo=github)](https://github.com/NewNanCity/YggdrasilGo/issues)

面向 Minecraft 客户端、启动器和游戏服务器的 Yggdrasil API 服务。项目使用 Go 和 Gin 实现认证、会话、角色资料与材质签名接口；可作为独立的 legacy 服务运行，也可与 BlessingSkin 共用 MySQL 8 事实源。

当前版本：`v0.0.14`。

## 适用边界

在 `shared_mysql` 模式下，职责固定如下：

- BlessingSkin 管理用户、玩家、皮肤与管理页面。
- 本服务提供游戏认证 API，保存游戏令牌、入服会话和稳定 UUID 身份记录。
- 玩家身份按 BlessingSkin 的 `players.pid` 绑定。改名不改变 UUID；名称复用不会继承原玩家的 UUID。
- 本服务不提供 BlessingSkin 网页登录，也不写入 BlessingSkin 的用户、玩家或材质业务数据。

`legacy` 模式仍保留，适用于既有的文件、内存、Redis 或数据库缓存部署。它不是四地共享认证状态的替代方案。

## 接口

所有路由可通过 `server.base_url` 加前缀。根路径会返回 Yggdrasil 元数据，并携带 `X-Authlib-Injector-API-Location` 响应头。

| 类别 | 方法 | 路径 |
| --- | --- | --- |
| 元数据与指标 | `GET` | `/`、`/metrics` |
| 认证 | `POST` | `/authserver/authenticate`、`/refresh`、`/validate`、`/invalidate`、`/signout` |
| 游戏会话 | `POST` / `GET` | `/sessionserver/session/minecraft/join`、`/hasJoined` |
| 角色资料 | `GET` / `POST` | `/sessionserver/session/minecraft/profile/{uuid}`、`/api/profiles/minecraft`、`/api/users/profiles/minecraft/{username}` |
| 材质接口 | `PUT` / `DELETE` | `/api/user/profile/{uuid}/{textureType}` |

材质上传和删除路由为兼容性保留。BlessingSkin 存储是只读材质适配器，这两个接口会返回 `501 NotImplemented`；皮肤管理应在 BlessingSkin 完成。

## 快速开始

前提：Go `1.26.6`，以及一个可写的本地工作目录。

```powershell
git clone https://github.com/NewNanCity/YggdrasilGo.git
Set-Location YggdrasilGo
Copy-Item conf/example.yml conf/config.yml
go test ./cmd/... ./internal/... ./src/... ./test/...
go run . -config conf/config.yml
```

示例配置监听 `0.0.0.0:8080`，可从本机通过 `http://127.0.0.1:8080/` 访问。它用于本地 legacy 启动；不要将占位密钥、DSN 或路径直接用于生产。

构建二进制：

```powershell
go build -o yggdrasil-api-server .
./yggdrasil-api-server -config conf/config.yml
```

容器镜像需要只读挂载配置目录：

```bash
docker run --rm -p 8080:8080 \
  -v "$PWD/conf:/app/conf:ro" \
  registry.cn-hangzhou.aliyuncs.com/newnancity/yggdrasil-go:v0.0.14
```

生产部署应使用不可变 digest，而不是可变标签。配置、私钥、数据库凭证和 RDS CA 文件必须由部署平台的 Secret 或受控挂载提供，不能提交到仓库或镜像。

## 配置模式

完整字段见 [配置示例](conf/example.yml)。以下只列出会改变运行语义的字段。

### `legacy`

省略 `auth.mode` 时默认使用 `legacy`。该模式需要至少 32 字符的 `auth.jwt_secret`，并按 `cache.token`、`cache.session` 和 `storage` 的既有配置初始化。

```yaml
auth:
  mode: legacy
  jwt_secret: "replace-with-a-secret-from-your-secret-manager"

storage:
  type: file
  file_options:
    data_dir: data
```

### `shared_mysql`

`shared_mysql` 只支持 `storage.type: blessing_skin`，并要求服务与 BlessingSkin 使用同一个 MySQL 8 schema。服务启动会校验认证 schema 状态；状态不满足时会拒绝就绪，不会回退到 legacy。

```yaml
server:
  api_location: "https://auth.example.com/"
  trusted_proxies: []

auth:
  mode: shared_mysql
  token_expiration: 72h
  tokens_limit: 10

storage:
  type: blessing_skin
  blessingskin_options:
    database_dsn: "provided-at-runtime"
    database_tls:
      ca_path: /app/conf/ApsaraDB-CA-Chain.pem
      server_name: rds.example.com
```

shared MySQL 配置要求：

- DSN 必须指定数据库并包含 `parseTime=true`，以及连接、读写超时。
- `database_tls.ca_path` 和 `database_tls.server_name` 为必填项。服务使用 CA 链和主机名校验 MySQL TLS，拒绝 `skip-verify`、`preferred` 等降级传输配置。
- `server.api_location` 应填写公开 HTTPS 根地址。只有来源属于 `server.trusted_proxies` 时，服务才会读取转发头；不要把 `0.0.0.0/0`、全量私网或容器 bridge 网段填入可信代理。
- 所有共享实例必须保持相同的令牌期限、令牌上限、签名密钥、皮肤域名、认证配置与数据库事实源。

## UUID 与迁移

旧 UUID 不会因升级或改名被重生成。生产迁移使用受控 CLI，计划文件必须位于 Git 忽略的 `.local/shared-auth/`；不要把计划、数据库凭证或逐行玩家数据提交到 Git。

```powershell
go run ./cmd/shared-auth-migrate dry-run `
  -config conf/config.yml `
  -plan .local/shared-auth/plan.json
```

`schema-upgrade`、`apply`、`activate` 和 `deactivate` 都是生产写操作，需要明确的数据库与计划摘要确认。完整顺序、冻结条件与回退限制见 [共享认证部署说明](docs/shared-auth-deployment.md)；身份模型与失败语义见 [共享认证设计](docs/shared-auth-design.md)。

## 开发与验证

常规检查：

```powershell
go test -race -count=1 -timeout 120s ./cmd/... ./internal/... ./src/... ./test/...
go vet ./cmd/... ./internal/... ./src/... ./test/...
go build ./cmd/... ./internal/... ./src/... ./test/...
git diff --check
```

共享认证的 MySQL 测试默认只允许项目测试容器。需要运行它们时显式设置：

```powershell
$env:YGG_TEST_MYSQL = "1"
go test -race -count=1 -timeout 240s ./src/sharedauth/... ./src/handlers ./src/storage/blessing_skin
```

这些测试不连接生产 RDS，也不能替代真实启动器、游戏服或跨地域容量验收。

## 文档

- [BlessingSkin 数据适配](docs/blessingskin.md)
- [共享认证设计](docs/shared-auth-design.md)
- [共享认证迁移工具](src/sharedauth/migrationplan/README.md)
- [共享认证模块](src/sharedauth/README.md)
- [四地部署与回退边界](docs/shared-auth-deployment.md)
- [未发布变更条目](docs/releases/unreleased/)

## 贡献与安全

提交前运行上述验证，并保持公共接口、数据库 schema 和配置契约兼容。涉及认证、权限、数据库迁移、UUID、密钥、生产部署或删除数据的修改必须先完成边界审查。

请勿在 Issue、提交、日志或文档中提交密码、DSN、私钥、SOPS/age 私钥、迁移计划或真实玩家标识。安全问题请通过仓库维护者的私密渠道报告。

## 许可证

[MIT](LICENSE)
