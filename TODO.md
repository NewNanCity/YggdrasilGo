# 当前任务

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
