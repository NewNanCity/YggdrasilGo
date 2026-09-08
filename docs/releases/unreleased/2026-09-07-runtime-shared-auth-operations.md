---
type: feature
scope: runtime
audience: internal
summary: 增加可审计的旧 UUID 迁移操作器、共享状态清理和 HTTP 生产边界
breaking: false
demo_ready: false
tests:
  - "YGG_TEST_MYSQL=1 go test -race -count=1 -timeout 300s ./..."
  - "go vet ./..."
  - "go build ./..."
  - "govulncheck ./..."
artifacts:
  - cmd/shared-auth-migrate/main.go
  - src/sharedauth/migrationplan
  - src/sharedauth/cleanup.go
  - src/middleware/request_limit.go
  - src/config/config.go
  - docs/shared-auth-deployment.md
---

## What changed

新增私有 dry-run、显式 schema upgrade、staged apply、verify、activate 和保留数据 deactivate。计划绑定完整来源快照、数据库名、排序规则、水位与 SHA-256；每个数据库操作都会从当前锁定快照重建动作并比对完整计划，来源漂移、动作改写或重复归属均拒绝。计划只能写入 Git 和镜像忽略的 `.local/shared-auth/`。

分类快照读取排序规则的 PAD 属性和实际字符长度；PAD SPACE 使用 MySQL 固定长度权重，避免 ASCII 尾空格或同权重 Unicode 字符绕过冲突隔离。无旧映射玩家按已批准策略生成确定性 OfflinePlayer v3，并与全部旧 UUID 和同批候选做碰撞预检；真实名称冲突继续 blocked。激活操作自身复核两个 users 安全钩子的完整正文。初次 apply 保留 schema 安装后产生的撤销 generation 锚点，不以清空安全状态换取迁移成功。

shared_mysql 新增按索引、每表 1000 行的周期清理，锁序与业务路径统一为 token 后 session，启动时先验证清理权限。现有请求大小与 HTTP 超时配置开始生效；Gin 与动态元数据共用显式 `server.trusted_proxies`，默认不信任代理头。shared_mysql 和迁移 CLI 强制 RDS CA、主机名、TLS 1.2+ 与三类 DSN 超时，拒绝宽松 TLS 参数。发布工作流 Go 版本与 `go.mod` 对齐。

漏洞扫描发现旧工具链和间接依赖有 10 条可达漏洞后，升级到 Go 1.26.6、`x/text` 0.39.0、`x/net` 0.55.0、`quic-go` 0.59.1 及其兼容的 `x/crypto`/`x/sys`，复扫可达漏洞为 0。

## Why it matters

旧 UUID 可以先审计再回填，不需要凭名称猜测身份；schema、数据和激活各自有检查点。四个地区即使同时清理也只执行幂等有界事务，不依赖存在连接所有权缺陷的 legacy 全局锁。请求体、慢连接和伪造代理头不再绕过已有安全配置。

## Demo posture / limitations

这次不代表生产数据库已建表、旧 UUID 已回填、state 已激活、镜像已发布或四地已切换。2026-09-08 已用控制台 CA 完成真实 RDS 证书链/主机名验证和新策略只读 dry-run：3305 active、6 blocked、187 reserved、0 invalid；规范计划摘要为 `bca43c5ddfae25621f9e488f8fa80261d264ab303f0f4ae1198e50f727969a10`。CA 和新计划仍只在 Git 忽略的本地目录，生产 Secret 尚未满足 TLS/超时契约，冻结后的最终快照也未生成，按现状禁止发布。三个 Docker 节点是宿主机端口直出，不能将容器 bridge 或全网设为可信代理；火山 DCDN 回源身份仍需确定。真实 BlessingSkin 账号语义、RDS 权限/触发器 definer、镜像仓库桥接和客户端验收仍需完成。NewNanCity 的实例更新仍由用户通过 MCSM 执行。
