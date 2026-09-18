# 旧 UUID 迁移计划

本包只把 BlessingSkin `players` 与旧 `uuid` 表分类为可保留身份、需人工处理、孤立保留三类。计划以来源快照 SHA-256、数据库名、排序规则、行数和 player 水位绑定；apply/verify/activate 会在事务内重新锁定来源、重建完整计划并逐摘要比对，任何来源漂移或动作改写均拒绝。

操作者入口是 `cmd/shared-auth-migrate`。计划文件可能包含玩家名、旧映射 ID 和 UUID，只能放在仓库忽略的 `.local/shared-auth/`，不得提交、复制到镜像或粘贴到日志。命令只输出迁移 ID、摘要、脱敏计数和水位。

```powershell
go run ./cmd/shared-auth-migrate dry-run -config conf/config.yml -plan .local/shared-auth/plan.json
go run ./cmd/shared-auth-migrate schema-upgrade -config conf/config.yml -confirm-database <database>
go run ./cmd/shared-auth-migrate verify-hooks -config conf/config.yml
go run ./cmd/shared-auth-migrate apply -config conf/config.yml -plan .local/shared-auth/plan.json -confirm-database <database> -confirm-plan-sha256 <sha256>
go run ./cmd/shared-auth-migrate verify -config conf/config.yml -plan .local/shared-auth/plan.json
go run ./cmd/shared-auth-migrate activate -config conf/config.yml -plan .local/shared-auth/plan.json -confirm-database <database> -confirm-plan-sha256 <sha256>
go run ./cmd/shared-auth-migrate deactivate -config conf/config.yml -plan .local/shared-auth/plan.json -confirm-database <database> -confirm-plan-sha256 <sha256>
```

`-config -` 可从 stdin 接收配置，用于把 Secret 直接管道到工具而不新建明文配置文件；输入硬限制为 4 MiB。迁移连接同样强制 `database_tls` CA/主机名校验及 DSN 连接、读取、写入超时。

`schema-upgrade` 只建五张 Go 表和两个 users 安全触发器；`apply` 只写 staged 身份和状态；`activate` 才开放 runtime。三者必须在全部旧 Go 写者停止的维护窗口内分开执行。`deactivate` 可将 active 改回 staged，但保留所有表和 UUID；运行一段时间后来源数据可能已变化，再次激活前必须重新协调计划。

分类器使用名称的字节精确值和 MySQL `WEIGHT_STRING` 排序权重；PAD SPACE 排序规则按实际列长度生成固定长度权重，覆盖普通尾空格及同权重 Unicode 字符，NO PAD 保持原始权重。没有旧映射且名称唯一的现有玩家按 `OfflinePlayer:<name>` 生成确定性 v3，但只有在不与全部旧 UUID 及同批候选冲突时才 active。名称/UUID 重复、排序规则等价但拼写不同、格式异常、多映射或生成值碰撞均不会猜测归属；相关玩家进入 blocked。合法孤立旧 UUID 进入 reserved，格式异常的旧行仍保留在原表作为证据。

## 已审查 blocked 身份解析

初始迁移完成后，人工确认的 blocked/reserved 对使用独立的 schema v2 和解析计划处理。审批文件只包含明确的 player、blocked identity、reserved identity 和全部旧映射 ID；dry-run 从数据库读取名称与 UUID，并把当前证据绑定到计划摘要。审批与计划都必须位于 `.local/shared-auth/`，命令只输出摘要和数量。

```powershell
go run ./cmd/shared-auth-migrate schema-upgrade-v2 -config conf/config.yml -confirm-database <database>
go run ./cmd/shared-auth-migrate schema-verify-v2 -config conf/config.yml
go run ./cmd/shared-auth-migrate schema-activate-v2 -config conf/config.yml -confirm-database <database>
go run ./cmd/shared-auth-migrate resolution-dry-run -config conf/config.yml -approvals .local/shared-auth/resolution-approvals.json -plan .local/shared-auth/resolution-plan.json
go run ./cmd/shared-auth-migrate deactivate -config conf/config.yml -plan .local/shared-auth/plan.json -confirm-database <database> -confirm-plan-sha256 <initial-plan-sha256>
go run ./cmd/shared-auth-migrate resolution-apply -config conf/config.yml -plan .local/shared-auth/resolution-plan.json -confirm-database <database> -confirm-plan-sha256 <resolution-plan-sha256>
go run ./cmd/shared-auth-migrate resolution-verify -config conf/config.yml -plan .local/shared-auth/resolution-plan.json
go run ./cmd/shared-auth-migrate resolution-activate -config conf/config.yml -plan .local/shared-auth/resolution-plan.json -confirm-database <database> -confirm-plan-sha256 <resolution-plan-sha256>
```

apply 前必须停止所有 Go 写者并关闭 state 门闩。工具重新锁定并核验名称排序规则唯一性、全部同名旧映射、UUID、两条身份行及 token/session 引用；任何漂移都整笔拒绝。原 blocked 行变成指向最终身份的 resolved 审计行，原 reserved 行保留 identity ID、UUID 和来源映射并变成 active，不删除任何行。

回退先执行 `resolution-deactivate`，再执行 `resolution-rollback`；两者都要求数据库名和解析计划摘要。只有两条身份均无 token/session 引用时才允许恢复 blocked/reserved。回退完成后，原始全量计划可在 schema v2 下重新 `activate`；若还要卸载 v2，依次执行 `schema-deactivate-v2` 和 `schema-downgrade-v2`。存在任一 resolved 行时两步都会拒绝。
