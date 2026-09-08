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
```

`-config -` 可从 stdin 接收配置，用于把 Secret 直接管道到工具而不新建明文配置文件；输入硬限制为 4 MiB。迁移连接同样强制 `database_tls` CA/主机名校验及 DSN 连接、读取、写入超时。

`schema-upgrade` 只建五张 Go 表和两个 users 安全触发器；`apply` 只写 staged 身份和状态；`activate` 才开放 runtime。三者必须在全部旧 Go 写者停止的维护窗口内分开执行。`deactivate` 可将 active 改回 staged，但保留所有表和 UUID；运行一段时间后来源数据可能已变化，再次激活前必须重新协调计划。

分类器使用名称的字节精确值和 MySQL `WEIGHT_STRING` 排序权重；PAD SPACE 排序规则按实际列长度生成固定长度权重，覆盖普通尾空格及同权重 Unicode 字符，NO PAD 保持原始权重。没有旧映射且名称唯一的现有玩家按 `OfflinePlayer:<name>` 生成确定性 v3，但只有在不与全部旧 UUID 及同批候选冲突时才 active。名称/UUID 重复、排序规则等价但拼写不同、格式异常、多映射或生成值碰撞均不会猜测归属；相关玩家进入 blocked。合法孤立旧 UUID 进入 reserved，格式异常的旧行仍保留在原表作为证据。
