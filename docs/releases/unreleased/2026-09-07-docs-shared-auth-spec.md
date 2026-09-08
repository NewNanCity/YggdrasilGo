---
type: docs
scope: docs
audience: internal
summary: 细化 BlessingSkin 共享认证与 UUID 迁移规格，并用确定性模型验证撤销和身份不变量
breaking: false
demo_ready: false
tests:
  - "go test -race -count=1 -timeout 30s -v ./working-delta/shared-auth-spec"
  - "go vet ./working-delta/shared-auth-spec"
  - "git diff --check"
artifacts:
  - docs/shared-auth-design.md
  - working-delta/shared-auth-spec/model_test.go
  - working-delta/shared-auth-spec/README.md
  - working-delta/2026-09-06-remediation-brief.md
  - README.md
  - TODO.md
---

## What changed

将已确认方向细化为五张 Go 自有表、单库认证事务、稳定 pid 身份、旧 UUID 分类迁移、显式 shared_mysql 模式和四地切换/回滚门槛。提出 users 安全触发器与撤销版本同事务的方案，说明仅比较最终密码哈希的 ABA 缺口，以及触发器故障会使 BlessingSkin 相应修改失败的代价。提出原短期窗口内可重复 HasJoined，明确它与早期一次性消费草案的区别。

新增 11 个顶层确定性模型测试，枚举 720 种原子事件顺序，并完成 race 与 vet 验证。同步 brief、README 和 TODO；未修改生产业务实现、配置结构或数据库。

## Why it matters

后续实现有明确的事务提交点、失败语义和身份保护规则。旧映射无法唯一归属时阻止自动分配，避免以新 UUID 掩盖存档归属问题；安全撤销覆盖改密往返变化，不把轮询假称即时通知。NewNanCity 仍由用户通过 MCSM 更新，不能由其余节点升级推断四地已经统一。

## Demo posture / limitations

这次不代表运行时认证已经修复、真实 MySQL 并发通过、生产已迁移或客户端已兼容。模型使用合成状态，事务原子性属于建模前提，不是测试结论。具体 schema、配置/错误契约、安全触发器和会话重试语义待用户确认；生产 DDL、回填、角色冻结、统一失效和发布另行授权。没有生产访问、提交、推送或部署。
