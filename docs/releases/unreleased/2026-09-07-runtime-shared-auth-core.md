---
type: feature
scope: runtime
audience: internal
summary: 实现未接入业务入口的 MySQL 共享认证核心、稳定身份及显式安全触发器迁移
breaking: false
demo_ready: false
tests:
  - "YGG_TEST_MYSQL=1 go test -race -count=1 -timeout 180s -v ./src/sharedauth/..."
  - "go test -race -count=1 -timeout 120s ./..."
  - "go vet ./..."
  - "go build ./..."
  - "git diff --check"
artifacts:
  - src/sharedauth/service.go
  - src/sharedauth/auth.go
  - src/sharedauth/migrations/migrations.go
  - src/sharedauth/README.md
  - internal/mysqltest/mysql.go
  - internal/mysqltest/mysql_test.go
  - docs/shared-auth-design.md
  - TODO.md
---

## What changed

按用户批准实现五张 Go 自有表和两个 users 安全触发器，提供显式 upgrade、完整触发器定义核验及只允许空新表的 downgrade。数据回填与激活不夹带在 schema 迁移中。实现 pid 稳定身份、原子刷新、单令牌/账号撤销、当前状态校验及短期可重复入服校验；新令牌是只在提交成功后返回的不透明随机值，数据库保存摘要。

隔离 MySQL 8.0.46 的 34 个场景通过，包括身份竞争、改密 ABA、失败回滚、会话绑定/过期、刷新/Signout 竞争及最小运行权限；补充由实际 InnoDB 锁等待约束的改密/刷新双向交错和取消。模拟驱动另测 5 种事务结果，覆盖提交应答不明不能假报回滚或重发；新增 SQL 文件换行兼容回归。测试只创建自己的本机临时容器，不接受外部 DSN，测试数据和容器由精确 ID 清理。

独立复核定位测试助手的 Docker endpoint 校验过宽，已补 24 个纯单元场景并先复现后修复。现在只接受规范本地管道/Unix socket，拒绝远端和有歧义的地址；后续命令固定使用已验证 endpoint，避免 context 被修改后转向其他 Docker 服务。

## Why it matters

认证状态和 UUID 归属现在有真实 MySQL 的库级实现与故障证据，后续 handler 不需要继续用 Get/Delete/Store 拼接非原子操作。改密撤销与密码更新同事务；Go runtime 不获得 BlessingSkin 写或 TRIGGER 权限，触发器验收由迁移操作者负责，避免为元数据健康检查扩大运行权限。

## Demo posture / limitations

这次不代表服务已切换新模式、旧漏洞已在现有 HTTP 入口修好、旧 UUID 已迁移或生产已发布。Authenticate 的超额策略和实际 BlessingSkin 账号语义尚待确认/核实；HTTP/config/资料签名/清理调度、全量回填及激活工具尚未接入。没有生产访问、建表、改配置、失效、提交、推送或部署，NewNanCity 仍由用户通过 MCSM 更新。

MySQL DDL 可能部分提交，downgrade 要求关闭全部写者且没有保留数据；不能拿空表检查当作可并发执行的删除授权。COMMIT 应答丢失只经模拟驱动验证，未做真实网络故障注入；测试不证明阿里云 RDS 权限/definer/故障转移或四地延迟。未修改既有 legacy 后端和用户工作区中的无关实现。
