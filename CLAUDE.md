# Yggdrasil API Go

Go 1.26.6 + Gin 实现的 Minecraft Yggdrasil API。生产主路径使用 BlessingSkin 同库 MySQL 事务认证；`legacy` 后端保留兼容。

## 目录索引

- `main.go`：服务启动、依赖装配和后台清理。
- `cmd/shared-auth-migrate/`：初始迁移、schema v2、身份解析和门闩操作 CLI。
- `src/handlers/`：Yggdrasil HTTP 端点与错误映射。
- `src/sharedauth/`：shared_mysql 事务、身份、token 和 session 领域逻辑。
- `src/sharedauth/migrations/`：显式 MySQL schema 与安全触发器。
- `src/sharedauth/migrationplan/`：初始 UUID 分类和回填计划。
- `src/sharedauth/resolutionplan/`：人工确认 blocked/reserved 身份的可审计解析。
- `src/storage/`、`src/cache/`：BlessingSkin 与 legacy 存储/缓存适配。
- `src/config/`、`conf/example.yml`：配置结构、校验和示例。
- `internal/mysqltest/`：本机固定摘要 MySQL 测试容器。
- `docs/`：架构、迁移、部署和发布事实源。

## 开发命令

```powershell
go test -race -count=1 -timeout 120s ./cmd/... ./internal/... ./src/... ./test/...
go vet ./cmd/... ./internal/... ./src/... ./test/...
go build ./cmd/... ./internal/... ./src/... ./test/...
git diff --check
```

共享认证集成测试只允许本机 Docker socket，使用固定 MySQL 8.0.46 摘要：

```powershell
$env:YGG_TEST_MYSQL = '1'
go test -race -count=1 -timeout 300s ./src/sharedauth/... ./src/handlers ./src/storage/blessing_skin
Remove-Item Env:YGG_TEST_MYSQL
```

服务启动：

```powershell
go run . -config conf/config.yml
```

## 架构边界

- 账号、角色、名称和材质事实来自 BlessingSkin；稳定游戏身份绑定 `players.pid`，改名不更换 UUID。
- `auth.mode: shared_mysql` 使用同一主 MySQL 连接池完成认证、身份、token 和 session 事务，要求验证 TLS；不回退 legacy。详见 [docs/shared-auth-design.md](docs/shared-auth-design.md)。
- 初始 UUID 迁移保留旧值并隔离歧义；schema v2 只解析经人工确认的 blocked/reserved 对，保留 resolved 审计行。操作顺序见 [src/sharedauth/migrationplan/README.md](src/sharedauth/migrationplan/README.md)。
- 运行时不执行 AutoMigrate。DDL、数据计划、门闩和回退均由显式 CLI 管理；计划与凭证只能放在 Git 忽略的 `.local/`。
- 生产发布必须使用不可变镜像摘要、显式 kubeconfig 和维护窗口检查点。详见 [docs/shared-auth-deployment.md](docs/shared-auth-deployment.md)。
- HTTP、代理信任、请求上限和 BlessingSkin 适配细节见 [README.md](README.md)、[docs/blessingskin.md](docs/blessingskin.md) 和 [src/sharedauth/README.md](src/sharedauth/README.md)。

## 编码约定

- 遵循现有 Go 包边界，错误显式返回；认证和迁移失败默认 fail closed。
- 公共 API、schema、认证语义和生产配置变更先评估兼容性并补回归测试。
- Bug 先写复现测试；共享状态、锁序、重试和回退优先使用固定 MySQL 集成测试。
- 不记录或提交 DSN、密码、token、私钥、邮箱、QQ、IP、真实玩家计划或逐行身份数据。
- 每轮有意义改动更新 `TODO.md`、相关 README/docs 和 `docs/releases/unreleased/`；完成任务族后从 TODO 移除。

## 当前重点

完成 schema v2 身份解析的发布、生产迁移和只读 profile 验收；其他 legacy 架构问题不在本任务范围内。
