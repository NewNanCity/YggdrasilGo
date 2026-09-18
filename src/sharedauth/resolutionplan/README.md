# blocked 身份解析计划

本包只处理已经人工确认的 blocked/reserved 身份对。它不按名称猜测归属，不生成 UUID，也不删除身份、旧映射、token 或 session。

`Build` 从私有审批 ID 读取实时 MySQL 证据，要求 schema v2、排序规则下唯一玩家、完整同名旧映射集合、相同历史 UUID、精确 blocked/reserved 行和零引用。`Apply` 在 staged 门闩下重新锁定全部证据，并在一个 REPEATABLE READ 事务中把 blocked 行改为 resolved 审计链接、reserved 行改为 active。`Verify` 核验最终指向；`Activate` 复核 schema/安全触发器后开放门闩。

`Deactivate` 只关闭门闩，供异常时立即止流。`Rollback` 仅在 staged 且两条身份都无 token/session 引用时恢复原 blocked/reserved 形态。任何选择器、行状态、名称等价集合、旧映射集合、UUID、引用或计划摘要漂移都会失败关闭。

文件读写使用 `0600` 临时文件和拒绝覆盖语义。调用 CLI 时，审批与计划路径还会被限制在 Git 忽略的 `.local/shared-auth/` 下。完整顺序见 [迁移操作说明](../migrationplan/README.md) 和 [部署检查点](../../../docs/shared-auth-deployment.md)。
