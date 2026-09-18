# 当前任务

## blocked 身份可审计解析（2026-09-18）

目标：在不删除身份行、不重生成 UUID、不改变 BlessingSkin 角色及 NNM 处罚的前提下，将六个已人工确认的 blocked/reserved 对解析为稳定 active 身份。

- [x] 运行时兼容 schema v1/v2，未知版本继续 fail closed。
- [x] 提供独立的 v2 schema upgrade/activate/deactivate/downgrade，并在隔离 MySQL 演练正向和回退。
- [x] 提供私有解析决定 → 解析计划 → 摘要确认 → apply/verify/activate/deactivate/rollback 流程；生产数据不写入仓库或日志。
- [x] 覆盖正常、漂移、引用、错误摘要、并发/回退边界，完成 race、vet、build 和自审。
- [ ] 发布不可变镜像，先滚动兼容运行时，再执行 v2 schema 和六项解析，完成数据库与公开只读 profile 验收。

非目标：不处理 PHP OAuth/OIDC 旧读路径，不改玩家名称、旧 `uuid` 表、NNM 绑定/封禁或其他部署组件。任何玩家/UUID 占用漂移、相关引用新增、迁移回退失败或生产健康异常均立即停止。

## shared_mysql 生产发布

目标：在保留 BlessingSkin 用户、玩家、UUID 和材质数据的前提下，将四地 Yggdrasil API 切换到同一 Aliyun RDS MySQL 8 事务事实源。

- [x] 实现 shared_mysql 认证、稳定 pid 身份、迁移操作器、请求边界和验证 TLS。
- [x] 通过隔离 MySQL、race、vet、build 和可达漏洞检查。
- [x] 使用真实 RDS CA 完成主机名/TLS 探针和生产只读 dry-run。
- [ ] 构建并推送阿里云不可变镜像，记录 source commit、tag 与 digest。release 分支已从干净 `v0.0.13` 基线提交，并排除 legacy 分布式锁 WIP。
- [ ] 为四地准备 CA 和配置；NewNanCity 仅更新配置，由用户通过 MCSM 操作实例。
- [ ] 冻结 BlessingSkin 玩家写入和四个 legacy Go 实例，重算最终快照。
- [ ] 依次执行 schema-upgrade、verify-hooks、apply、verify、activate。
- [ ] canary 后逐地发布，验收 TLS、健康、认证/刷新/撤销及 Join/HasJoined。

硬边界：任何生产写步骤失败后停止，不盲重试或自动 downgrade；不删除或重生成用户、玩家、旧 UUID、材质和迁移身份数据。可信回源身份未确定前保持 `trusted_proxies: []`，不信任公网或容器 bridge 网段。

已知非目标：legacy 分布式锁 WIP 不属于此 release，保持 `v0.0.13` 基线；其连接所有权问题另行处理。

维护记录：根级 `go test ./...` 会编译 Git 忽略的 `tmp/security-review-20260906` 与 `tmp/sttothome-review-20260906` 历史审查包；它们仍引用已删除的 legacy 分布式锁 API。该问题不影响受版本控制的项目包测试，但应在单独的本地临时目录清理任务中处理。
