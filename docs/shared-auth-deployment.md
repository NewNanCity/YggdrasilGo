# 四地 shared_mysql 部署检查点

状态：代码和隔离 MySQL 验证已完成；本文是执行清单，不表示生产 DDL、回填、激活或实例切换已经发生。

## 固定边界

- BlessingSkin 继续管理账号、玩家和皮肤；Go 只提供游戏认证 API。
- 四地连接同一个 Aliyun RDS MySQL 8 主库。旧 UUID 不重生成，身份绑定 `players.pid`，改名不换 UUID；水位后的新 pid 使用 v4。
- NewNanCity 只更新 `/minecraft/yggrasil-go` 下的配置，由用户通过 MCSM 更新实例。其他三地按各自现有管理面切换。
- 计划文件、DSN 和密钥不进 Git、镜像、终端记录或发布说明。
- RDS 连接必须启用可验证的 TLS 并设置连接/读写期限。2026-09-08 使用控制台下载的 `ApsaraDB-CA-Chain.pem` 完成真实 endpoint 的证书链和主机名校验；数据库会话协商 TLS 1.2 且 `Ssl_cipher` 非空。CA 当前仅在 Git 忽略的本地目录，仍需按四地现有配置边界只读挂载。RDS 当前 `require_secure_transport=0`，本服务启用验证 TLS 不会迫使其他客户端同时切换。
- 默认不信任代理头；各入口必须按实际直连来源配置 `server.trusted_proxies`，否则 Join IP 与 HasJoined 的客户端 IP 会不一致，认证限流也会退化为按代理共享。不得把全部私网或 `0.0.0.0/0` 作为方便性默认值。
- 只读网络核对得到 SttotHome Pod CIDR `10.42.0.0/24`；三个 Docker 节点均为宿主机端口直出。容器 bridge 网段不是 DCDN 回源来源，不能写入可信列表；DCDN 回源身份没有可维护证据前保持空列表并阻止发布。

## 维护窗口

1. 记录四地当前镜像摘要、配置摘要、进程数和外部健康状态；确认 RDS 为可写主库，并完成 `dry-run`。2026-09-08 经验证 TLS 的只读快照为 3311 players / 2738 mappings，3305 active、6 blocked、187 reserved、0 invalid；756 个无旧映射玩家已生成确定性 v3，6 个名称冲突继续 blocked。规范计划摘要为 `bca43c5ddfae25621f9e488f8fa80261d264ab303f0f4ae1198e50f727969a10`。冻结角色写入后必须重算最终快照，不能直接把当前计划用于写步骤。
2. 先停止或隔离全部旧 Go 写者。SttotHome 的 NodePort 可绕过入口路由，不能只切 CDN；需将该 Deployment 缩到 0。BlessingSkin 可继续提供账号和皮肤功能，但从最终 dry-run 开始到激活验收完成，必须冻结玩家新建、改名、转移和删除。
3. 用稳定迁移身份依次执行 `schema-upgrade`、`verify-hooks`、`apply`、`verify`。每一步失败即停止；MySQL DDL 可能部分提交，禁止盲重试或自动 downgrade。
4. 对照同一 plan SHA-256 执行 `activate`，再次 `verify`。只在 active 后将所有实例配置为 `auth.mode: shared_mysql`。
5. 先启动一个可控实例，验证元数据、公钥指纹、匿名资料、合成账号认证/刷新/撤销和游戏服 Join/HasJoined；再逐地启动。NewNanCity 留给用户在 MCSM 操作。

## 回退

出现认证错误率、数据库锁等待、UUID 不一致、公钥变化或跨地延迟超标时，先隔离全部入口并停止 Go 写者，再用原计划执行 `deactivate`，使 shared_mysql fail closed。修复后使用同一身份表和 shared_mysql 协议前滚，重新 verify/activate；不删除五张新表、触发器、身份或旧 `uuid` 表，也不承诺保留切换后签发的会话。

一旦水位后的新身份可能已经对外使用，禁止恢复会按 `uuid.name` 分配身份的旧镜像/旧配置。只有能证明尚未产生任何水位后身份、令牌/会话或角色改名，并另行批准回退计划时，才可考虑切回 legacy；不能把镜像回滚当作默认恢复手段。

## 验收证据

- `verify-hooks` 和 `verify` 成功，state 为 active，身份计数与已批准 dry-run 完全一致。
- 四地实际运行镜像均为同一不可变 digest；配置只记录脱敏摘要，`api_location` 为显式 HTTPS 地址。
- TLS 会话的 `Ssl_cipher` 与 `Ssl_version` 均非空且版本至少 TLS 1.2；证书主机名为实际 RDS endpoint。
- 过大请求返回 413 或在流式读取时被硬上限终止；伪造转发头不改变客户端 IP 或 API 地址。
- 启动时过期状态清理权限检查成功，之后每五分钟按每表 1000 行上限清理；无全局锁依赖。
- NewNanCity 配置已准备但实例切换由用户确认完成。
