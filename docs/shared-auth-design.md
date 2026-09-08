# BlessingSkin 与 Go 共享认证实施规格

状态：2026-09-07，具体方案已批准进入本地实现和隔离数据库验证，包含安全触发器及其故障耦合。本文件不是生产执行授权。当前 [共享认证核心](../src/sharedauth/README.md) 及 shared_mysql 启动/HTTP/资料适配已在本地实现并做隔离测试，尚未迁移旧数据或发布。

## 1. 本轮契约与事实源

Outcome：明确稳定角色身份、改密撤销和四地共享认证的可实现契约，使下一轮能够逐单元编码，不再由实现者临时决定身份归属。

Success criteria：字段与唯一约束、事务提交点、迁移分类、异常响应、配置兼容、切换和回滚条件均明确；本地确定性模型覆盖关键反例和交错场景。真实 SQL、驱动、HTTP 和客户端验收仍在后续阶段。

Constraints：保留所有已有用户、玩家、UUID 和材质；不覆盖其他工作区修改；不执行生产读取扩容、DDL、回填、触发器安装、令牌失效、发布或凭据轮换。NewNanCity 只准备约定的配置，由用户通过 MCSM 更新实例。

Non-goals：重查历史中文名称事件、迁移 PostgreSQL、引入 Redis、重新启用 PHP Yggdrasil 插件、补 Go 网页登录/材质上传、一次性重写其他存储后端。

Stop rules：需要新增授权、无法唯一归属旧 UUID、发现实际 BlessingSkin/数据库语义不同、验证连续失败或只能靠降级授权绕过时，暂停对应单元；不自动扩大扫描、修改业务记录或放宽断言。

| 分类 | 依据与边界 |
| --- | --- |
| 已确认的目标 | BlessingSkin 管理账号/密码/角色/皮肤；Go 提供游戏 API；UUID 绑定 pid，改名保 UUID，新身份用 v4；改密撤销旧令牌 |
| 当前源码 | [认证处理器](../src/handlers/auth.go)、[会话处理器](../src/handlers/session.go)、[存储接口](../src/storage/interface/types.go)、[账号实现](../src/storage/blessing_skin/users.go)、[配置](../src/config/config.go)；不能当作所有线上镜像的源码证明 |
| 已有线上证据 | 2026-09-07 本地审查生成的四地运行清单、UUID 表元数据和定向映射查询；这些临时证据不随 release 分支发布 |
| 未知 | 实际 BlessingSkin 版本、账号表引擎/触发器、完整冲突数量、四地有效配置、RDS 小版本/主库路由/权限及客户端兼容性 |

本规格取代 2026-09-06 本地整改 brief 中的候选实现细节，尤其是 owner 优先锁序、只扩展 Cache CRUD 和一次性 Consume；产品方向与数据保留边界不变。文末三项及本节后续的令牌超额淘汰规则已获本地实施批准，未知的实际账号语义仍不能静默决定。

## 2. 领域与所有权

| 概念 | 事实源与不变量 |
| --- | --- |
| 账号 | BlessingSkin `users.uid`；密码和权限状态由 BlessingSkin 解释。外部 `user.id` 本期保持现有序列化，不顺手改成另一个 UUID |
| 角色 | BlessingSkin `players.pid`；同一 uid 可以有多个 pid。名称、当前所有者和材质都是可变属性 |
| 游戏身份 | Go 身份表的 `player_id -> uuid`；一经启用不可重绑。UUID 与名字、密码、邮箱、节点无关 |
| 撤销版本 | Go 账号锚点上的单调 `generation`；令牌携带签发时的版本，必须与当前版本相同 |
| 令牌 | Go 持久化记录；随机不透明凭证，客户端按字符串持有。签名正确、节点缓存命中都不能代替共享状态校验 |
| 入服会话 | 短期 `server_id -> token` 绑定；每次 HasJoined 重新核验令牌和当前角色状态 |

UUID 响应保留现有无连字符小写十六进制形式；解析旧值后只改变存储表示，不改变 128 位值。只接受已明确支持的 32 位十六进制或规范带连字符格式，其他格式进入冲突清单。禁止对旧值重新运行 v3/v4 算法。

Go 的新主路径不再读写旧 `uuid.name` 作为实时身份映射。按名称查当前 pid，按 pid 查稳定 UUID；反向用 UUID 找 pid，再读当前名称/材质。首次公开查询新角色也可能需要显式申请身份，但不能让普通 `GetUser` 或读取旧玩家的代码隐式制造 UUID。

实时名称解析沿用经核实的 BlessingSkin 名称比较规则，必须检查是否唯一，不能用 SQL `First` 吞掉第二个候选；响应使用站点当前规范名称。HasJoined 通过名称解析得到的 pid 与令牌绑定比较，不以客户端拼写作为归属键。大小写兼容和排序规则冲突需由数据库/HTTP 测试覆盖；迁移采用字节精确匹配是更严格的历史归属门槛，不等于偷偷重定义实时名称规则。

## 3. 改密撤销必须来自变更源

### 3.1 当前哈希比较的缺口

当前账号模型没有独立的单调密码版本。源码支持若干确定性密码哈希方式，因此不能假设每次保存都产生不同的随机盐哈希。若 Go 两次观察之间发生 `hash-A -> hash-B -> hash-A`，仅比较最终哈希无法知道密码曾变更；轮询或提交后才通知的 webhook 也有延迟/漏事件窗口。

前期本地确定性模型验证了这个反例。它不是线上发生过该事件的证据，也不说明生产正使用哪一种密码算法；临时模型不随 release 分支发布。

### 3.2 推荐：同库事务内的窄范围安全触发器

建议在 BlessingSkin `users` 上安装受版本管理的两个触发器，仅维护 Go 的 `auth_subjects`：

- `AFTER UPDATE`：密码哈希的**字节值**改变，或 `permission` 数值改变时，使该 uid 的 generation 加一。密码比较使用二进制、NULL 安全比较，不能受 `utf8mb4_unicode_ci` 的大小写等价影响。
- `AFTER DELETE`：使该 uid 的 generation 加一，保留锚点；删除用户不删除其旧认证版本。

锚点缺失时插入 generation=1；已存在时递增。新账号第一次认证由 Go 建锚点；generation 不允许归零、溢出或删除重建。所有权限值变化均撤销是保守策略，包括管理员提权/降权，不只封禁；昵称、积分等无关字段不触发撤销。

这是对“仅 Go 写 Go 自有状态”的**显式例外**：BlessingSkin 的数据库事务通过触发器推进一个 Go 自有字段。Go 仍不修改 BlessingSkin 的密码和业务字段，旧 Yggdrasil 插件保持停用。不能把此处写成已经批准安装。

密码更新和撤销版本更新必须在同一 InnoDB 语句事务内成功；触发器报错时该更新语句也失败并回滚其修改。代价是 Go 状态表/触发器故障会使 BlessingSkin 改密失败，不能静默忽略这个错误后让网页显示成功。[MySQL 触发器错误语义](https://dev.mysql.com/doc/refman/8.0/en/trigger-syntax.html)

安装前必须核对 `users`/目标表均为 InnoDB、已有触发器、RDS 权限与限制、稳定 definer、实际改密和管理后台写路径。普通行级 UPDATE/DELETE 属于覆盖范围；TRUNCATE、换表、库恢复、绕过触发器的运维流程不在保证内，必须先关认证流量，恢复后重新完成撤销/对账再开放。

备选是 BlessingSkin 扩展在**同一数据库事务**内推进版本，前提是确认其版本和所有写路径都可覆盖。单纯提交后事件通知不能替代上述原子保证。若不接受两者之一，需要重新确认“改密后立即撤销”的保证强度，不能以哈希轮询冒充完成。

### 3.3 账号状态语义

封禁、删除账号在所有授权操作中查询当前值，即使令牌尚未清理也拒绝。`permission` 的解释只在 BlessingSkin 适配器中实现；[旧文档](blessingskin.md) 与当前代码的封禁值描述存在差异，安装版本的事实核验是实现前置条件。邮箱验证按当前已确认配置解释，不顺便启用新门槛。

即时性的定义：变更提交后才开始的授权请求必须拒绝旧版本；与变更重叠的请求按数据库事务顺序裁决，不追溯撤回已发出的成功响应，不踢已在线玩家。重设成相同存储哈希且没有任何状态变化不是本方案可观察的改密事件；需要强制撤销时使用 Signout。

## 4. Go 自有表提案

使用同一 RDS **主库、同一数据库**内的固定 `ygg_go_` 前缀，所有表 InnoDB。对应的 [显式 schema 迁移](../src/sharedauth/migrations/migrations.go) 已在隔离 MySQL 验证；数据回填计划尚未实现，运行时禁止 AutoMigrate。

ID 使用 `BIGINT UNSIGNED` 容纳现有非负 uid/pid；Go 使用有范围检查的类型转换，不能通过负数转换“修复”历史记录。UUID 和令牌哈希均二进制存储，不受文本排序规则影响。时间为 UTC `DATETIME(6)`，事务内由数据库当前时间决定过期，节点本地时钟仅用于请求期限。

### 4.1 `ygg_go_identities`

| 字段 | 类型与约束 | 用途 |
| --- | --- | --- |
| identity_id | BIGINT UNSIGNED，PK，自增 | Go 内部引用，不是公开 UUID |
| player_id | BIGINT UNSIGNED，NULL，UNIQUE | 绑定 pid；孤立旧 UUID 没有 pid |
| uuid | BINARY(16)，NULL，UNIQUE | 已有值保持；待处理角色暂不发值 |
| state | VARCHAR(8)，ASCII 二进制排序，NOT NULL | active / retired / reserved / blocked |
| legacy_mapping_id | BIGINT UNSIGNED，NULL，UNIQUE | 唯一可采用旧映射的来源 id；复杂冲突证据保留在旧表及私有迁移计划 |
| created_at / updated_at | DATETIME(6)，NOT NULL | 运维审计时间，不决定归属 |

CHECK 组合：active/retired 必须同时有 pid 与 UUID；reserved 必须有 UUID、无 pid；blocked 必须有 pid、无 UUID。非 NULL 的 ID 必须大于零。UNIQUE 的 NULL 允许多个待处理项，不允许多个实际 UUID 或 pid 归属。

active 的 pid/UUID 不可更改；删除角色后记录保留，后台只可标记 retired，授权路径即使未标记也因实时角色不存在而拒绝。reserved 永不自动分配；blocked 的解除必须另行人工确认数据计划。禁止从身份表 DELETE，禁止把保留旧值当作随机碰撞后可覆盖的记录。

### 4.2 `ygg_go_auth_subjects`

| 字段 | 类型与约束 | 用途 |
| --- | --- | --- |
| user_id | BIGINT UNSIGNED，PK，>0 | 每账号锁锚点，与账号删除解耦 |
| generation | BIGINT UNSIGNED，NOT NULL，>=1 | 改密、权限变化、删除、Signout 递增 |
| updated_at | DATETIME(6)，NOT NULL | 变更时间 |

runtime 不得删除本表记录。这里不存密码、密码哈希副本、明文令牌或签名密钥。新增记录、并发创建和递增都在事务内，不能先无锁读再写 generation+1。

### 4.3 `ygg_go_tokens`

| 字段 | 类型与约束 | 用途 |
| --- | --- | --- |
| token_hash | BINARY(32)，PK | SHA-256(accessToken) |
| user_id | BIGINT UNSIGNED，NOT NULL | FK 到 auth_subjects，RESTRICT |
| generation | BIGINT UNSIGNED，NOT NULL，>=1 | 签发版本 |
| identity_id | BIGINT UNSIGNED，NULL | FK 到 identities，RESTRICT；未选角令牌可为空 |
| client_token | MEDIUMBLOB，NOT NULL | 不透明字符串的 UTF-8 字节，精确往返/比较，不索引；最大长度受已校验请求上限约束 |
| created_at / expires_at | DATETIME(6)，NOT NULL | CHECK expires_at > created_at |

索引 `(user_id, generation, expires_at)` 用于用户令牌限额，`(expires_at, token_hash)` 用于小批量清理；identity_id 的 FK 索引显式声明。不对 BlessingSkin 表建立跨所有权 FK，不使用级联删除。

新 accessToken 为 `crypto/rand` 生成的 32 字节经无填充 base64url 编码，数据库只存摘要。随机失败拒绝签发；确认唯一键碰撞时最多重试 3 个新候选，其他错误不能当成碰撞。响应和日志不泄漏摘要/令牌、密码或客户端绑定值。旧 JWT 不迁入新表，切换时统一失效须在实际窗口再次确认。

新模式尊重已经核定的 `auth.token_expiration`，建议首版保留默认 72 小时，不在本期增设宽限刷新期限；实际四地值尚未读取。刷新重新计算到期时间，不直接复制旧 expires_at。客户端令牌缺省值由服务端生成，客户端提供值按协议原样返回；不能沿用 varchar(255) 静默截断。

### 4.4 `ygg_go_join_sessions`

| 字段 | 类型与约束 | 用途 |
| --- | --- | --- |
| server_id | VARBINARY(255)，PK | 请求会话标识，非空，超长显式 400 |
| token_hash | BINARY(32)，NOT NULL，INDEX | 令牌引用；不设 FK，令牌失效后记录自然过期 |
| client_ip | VARBINARY(16)，NOT NULL | 解析后 IP 字节；IPv4-mapped 地址统一标准化，禁用任意字符串等价判断 |
| created_at / expires_at | DATETIME(6)，NOT NULL | 固定 30 秒 TTL，CHECK expires_at > created_at |

增加 `(expires_at, server_id)` 清理索引。不重复保存 username、owner 或 profile 属性。令牌已删除/到期/版本不符时该会话不可用，无需先级联删除才能撤销成功。

同 server_id 的未过期 Join 只允许完全相同 token/IP 的幂等重试，重试不延长原期限；不同绑定返回认证冲突，禁止覆盖。过期行可以在带条件的写操作中替换。IP 来源必须由可信代理配置决定，不能任信 X-Forwarded-For。

### 4.5 `ygg_go_state`

| 字段 | 类型与约束 | 用途 |
| --- | --- | --- |
| id | TINYINT UNSIGNED，PK，CHECK id=1 | 单行运行门闩 |
| schema_version | INT UNSIGNED，NOT NULL | 兼容性检查，初版 1 |
| phase | VARCHAR(8)，ASCII 二进制排序 | CHECK staged / active |
| player_high_watermark | BIGINT UNSIGNED，NOT NULL | 最终冻结快照时已存在的最大 pid |
| migration_id | BINARY(16)，NOT NULL | 对应经批准的数据计划 |
| activated_at | DATETIME(6)，NULL | active 时必须非 NULL |

新路径每个业务事务先锁定读取此行；仅 active 可以签发、授权或新增身份。staged 用于离线核验；迁移/控制凭证才能修改状态行，runtime 只 SELECT。表、版本或门闩不符合时 readiness 失败、业务返回 503，不能退回旧模式。

## 5. 事务与内部接口

### 5.1 边界与锁序

不继续把跨表授权流程拼成 Cache 的 Get/Delete/Store。候选内部服务 `SharedAuthService` 按 Authenticate、Refresh、Validate、Invalidate、Signout、Join、HasJoined 提供领域操作；`IdentityDirectory` 提供 pid 解析和显式身份申请。HTTP handler 只做解析/响应映射，数据库实现拥有一个事务上下文和一个 SQL 连接池。

复用 BlessingSkin 配置中的主库连接建立事务单元；用户读、撤销版本、令牌、身份和会话必须使用同一个 `*sql.Tx` / GORM tx。不能从 token/session 两个独立 cache DSN 各开事务后声称整体原子。普通用户/角色读取不得通过 AuthenticateUser 触发 UUID 创建。

推荐 READ COMMITTED，统一顺序：state `FOR SHARE`，users `FOR SHARE`，auth_subjects `FOR UPDATE`，目标 token，players `FOR SHARE`，identity，join_session。多个同类对象按主键排序。公开资料查询没有账号/令牌时可跳过对应层，但不能反向取得已跳过层的锁。

users 必须在 subject 之前：BlessingSkin UPDATE 已先持有 user 行锁，触发器随后锁 subject；Go 若反过来锁 subject 再等 users，会制造相反锁序。该顺序降低已知循环风险，不保证外部批量操作绝不死锁；SQL deadlock 仍必须有界回滚、分类处理。

MySQL 8.0.22 起，`FOR SHARE` 只需 SELECT；更早的小版本还需额外写/锁权限。因此“RDS MySQL 8”不足以证明最小权限方案可直接部署，先确认具体版本，不自动给 Go 增加 BlessingSkin 写权限。[MySQL 锁定读取权限与语义](https://dev.mysql.com/doc/refman/8.0/en/innodb-locking-reads.html)

密码昂贵计算在事务外，事务内锁定用户后重新比较实际哈希与已验证快照，发现改变则拒绝此次提交，让客户端重新认证。Refresh 的前置读取只用于定位；事务内必须重新读取旧 token 与当前 generation，不能接受读取快照替代提交条件。

### 5.2 操作契约

| 操作 | 事务内条件与结果 | 可确认失败时 |
| --- | --- | --- |
| Authenticate | 当前账号允许，密码快照仍匹配；按当前 pid 解析角色；在用户锁下执行令牌限额并插入新 token | 不发布 token/UUID；整个事务回滚 |
| Refresh | 旧 token 存在、未过期、generation/客户端/绑定正确；选角来自当前服务端资料；原子删旧插新 | 旧有效令牌保留；同旧值最多一个成功 |
| Validate | token、generation、账号当前状态均合法；若绑定角色，再验证身份 active 和当前所有者 | 无效返回 403；基础设施错误不伪装有效 |
| Invalidate | 按 token/client 精确核验并删除；不存在视为幂等完成 | 存储失败不能返回成功，提交不明单列 |
| Signout | 当前密码验证后 generation 加一；不依赖扫描删除所有 token | 明确回滚时旧状态不变 |
| Join | Validate 全部条件成立、已选角、selectedProfile 相同；条件写入短会话 | 无效返回 403，冲突不覆盖其他会话 |
| HasJoined | 会话未到期，token/版本/账号/当前玩家归属、当前名称和可选 IP 均符合；返回规范签名 profile | 无匹配 204；数据库/签名故障 5xx |

Refresh 未选角令牌可以绑定该 uid 的一个 active 角色；已有绑定不能换成另一角色。客户端传入的 name/properties 不作为资料事实源。无效选角在删旧之前判定。用户多角色列表中有 blocked 身份时，返回其余已确认身份；不得悄悄给该角色发新 UUID，选中冲突角色时返回明确的不可用错误并计数。

现有 legacy 认证流程尚未使用 `TokensLimit` 和 GetUserTokenCount。2026-09-07 用户批准新路径在满额时淘汰最早签发的旧令牌，允许新登录：仅统计同一账号、当前 generation、尚未到期的记录，在 subject 锁下裁决。按 `created_at` 升序选取必要数量，相同时间以二进制 `token_hash` 排序；Refresh 已重新签发的新令牌以自己的创建时间参与排序，不按最初登录时间或最后访问时间排序。

新 Authenticate 的上限由可信调用方传入正整数，不改变已有 NewAuth 签名，也不把 0/负数解释为无限额。锁定候选旧令牌后再读取/申请角色身份；新令牌插入和必要的旧令牌删除处于同一事务，新值未成功插入前不删除旧值。无效凭据、身份冲突、存储/随机错误和明确回滚都不能使旧令牌或旧会话先失效，未知提交仍按 commit_unknown 处理。被淘汰 token 的会话因权威 token 不存在而失效，无需同步扫描全部 session。

该批准不等于已改变四地配置值或现有 HTTP 行为。配置仍需跨实例一致，账号密码适配、启动/HTTP 接入和生产参数核验另行完成。

profile 的当前 owner 必须与 token.owner 相符。没有授权迁移玩家所属账号；若管理操作需要转移角色，先明确撤销和存档转移规则。此规格只保证当前归属检查，不声称角色转出再转回会自动永久撤销旧 token；pid 删除/导入复用禁止作为正常业务路径。

### 5.3 会话重试建议

推荐 HasJoined 在原 30 秒有效期内可重复验证，不删除会话；每次仍查当前授权状态，撤销后立即拒绝。错误用户名/IP 和签名失败也不损坏会话。这样请求超时后的合法重试不因第一次读已消费而失败。

这是对早期一次性 Consume 提案的明确修订，已获本地实施批准。authlib-injector 维护者规范要求检查短期会话和绑定信息，没有要求读后必须删除；它建议内存数据库，本方案选 MySQL 是四地同源架构取舍，必须补延迟/容量测试，不能据协议文本宣称 MySQL 性能已验收。[会话协议](https://yushijinhun.github.io/authlib-injector/zh/Yggdrasil-%E6%9C%8D%E5%8A%A1%E7%AB%AF%E6%8A%80%E6%9C%AF%E8%A7%84%E8%8C%83.html)

签名失败不得返回缺失签名的“成功”资料。可在数据库事务读取并确认授权后签名；与撤销重叠的在途响应按上述即时性定义处理。无 token 的公开 profile 查询不是登录授权，但同样不能从失效名称缓存拼出另一个玩家身份。

### 5.4 超时与失败分类

`invalid / not_found / identity_conflict / backend_unavailable / commit_unknown` 必须是可区分的内部结果。预期无效认证返回既有 403 或 HasJoined 204；格式/大小错误 400/413；存储/签名失败 503/500。用 5xx 区分后端故障是待批准的错误契约调整，不能同时声称所有旧错误响应完全兼容。

COMMIT 应答丢失时返回 commit_unknown 对应的 503，不假称回滚，也不盲目重发新 token。数据库可能已经替换旧 token，用户可能需重新登录；本期不新增保存明文响应的幂等账本。只有驱动明确确认整笔回滚的死锁等错误，才允许在请求 deadline 内有界重试，且重新执行所有授权条件。

请求取消不能进入无限等待；事务有 deadline，数据库锁等待有上限，未知提交不能复用成“已回滚”分支。清理任务使用小批量条件 DELETE 和过期索引；清理滞后只占空间，不恢复权限。不以 GET_LOCK 成功作为授权事务已经协调的依据。

## 6. UUID 迁移：保存值，阻止猜测

### 6.1 分类规则

| 冻结快照中的情况 | 计划动作 | 禁止动作 |
| --- | --- | --- |
| 当前名称字节精确匹配唯一旧行、该 UUID 也唯一归属一个当前 pid | active，采用旧 UUID；记录来源 id | 重算 UUID 或更换算法后覆盖 |
| 旧映射没有当前玩家 | 按 UUID 去重保留 reserved，旧行原样保留 | 按相似名称猜改名、清除孤立行 |
| 多 pid/旧行冲突、仅排序规则等价、UUID 格式异常 | 受影响 pid 标 blocked；可解析的相关 UUID 先 reserved | 取第一条、碰撞后给其中一人新 v4 |
| 已存在 pid 没有可确认映射 | blocked | 认定其“从未登录”后自动发 v4 |
| 切换后新 pid，高于冻结 high watermark，且没有 blocked/retired 记录 | 当前存在/归属验证后，唯一约束下申请新 v4 | 继承其名称曾对应的 UUID |

唯一当前映射可以作为本次迁移的**现状基线**，不证明它在历史上始终归属正确。这一假设遵循用户“不再追查以前问题”的范围；若要证明历史归属，需要另一轮取证，不能在本迁移里默默作出该保证。

阻止不明玩家登录是保护存档的代价，人数当前未知。先给出脱敏分类计数再确定切换窗口，不能把“已有玩家无映射”的估计差值当作精确影响人数。原始逐行清单保存在受限本地位置，Git/聊天仅保留分类计数、计划摘要和审批标识。

### 6.2 可审查的数据流程

1. 批准限定 preflight 后核验引擎、索引、名称排序规则、主库、账号/角色约束和完整冲突计数；每类查询设成本/期限边界。先产出 dry-run，不改表/数据。
2. schema-only upgrade 建新表、约束和经批准的安全钩子；data-only 计划单独保存，包含来源摘要、分类计数、watermark、migration_id 和回退前提。安装钩子仍是生产写操作，另行授权。
3. 经批准的窗口内关闭所有旧 Go/PHP 认证写入口，暂时冻结 BlessingSkin 的角色新增/改名/删除。最终快照重新核对 dry-run 的前提，任何变动需重算并复核，不直接套用旧计划。
4. 新表 staged 状态下执行逐批幂等回填；保留所有旧 uuid 行。核对原/新 128 位值、唯一性、blocked/reserved 分类及 pid 基数。未处理冲突有显式限制，不伪装全站可用。
5. 五个进程均更新或明确从所有入口隔离，旧写者不可达后激活 state；核验新 pid 的自增行为，再恢复 BlessingSkin 角色管理和认证流量。

watermark 只适用于已确认的单调 pid 分配方式；备份导入、手工插入旧 pid、复用主键都需走单独数据计划，不能让运行时靠 `pid > watermark` 自动判断一切导入都是新身份。身份申请并发锁定读取玩家行，靠 UNIQUE(pid)/UNIQUE(uuid) 裁决，提交后才能响应或缓存。

DDL 与数据回填分别具备 upgrade/downgrade 设计，但不能承诺任意时刻无损回到旧身份算法：激活前、确认无新依赖数据时可以按逆序卸载钩子/新表；一旦已签发新身份，禁止直接旧镜像回滚或删除新表。此后只能回到理解同一新 schema 的已验证版本，必要时先关闭认证、保留身份记录再恢复。

备份回退可能让 generation 和 token 一起回到旧值；恢复后必须重新完成统一撤销才开放认证。任何清理/回滚计划不得恢复旧 uuid.name 写者，也不得丢弃已经对外使用的新 UUID。

## 7. 配置、权限与现有后端

已实现显式 `auth.mode: legacy | shared_mysql`，省略保持 legacy；四地目标必须显式选 shared_mysql。生产配置尚未修改，只有完成迁移、激活和部署窗口审批后才能写入。

shared_mysql 要求 BlessingSkin + 经核验的同库 MySQL 主连接。沿用该连接创建新的事务服务，旧 token/session Cache 配置保留解析但明确标为 inactive，不能实例化其工厂、运行 AutoMigrate 或在故障后兜底。连接必须提供 RDS CA 文件、与 DSN endpoint 一致的 `server_name`、`parseTime=true` 和连接/读/写超时；代码注册 TLS 1.2+ 且同时校验证书链与主机名，拒绝 DSN 自带的 `preferred`、`skip-verify` 或其他 TLS 模式。未知模式、不支持的数据库/后端组合、缺表/错误版本或 staged 状态必须可观测地拒绝启动就绪。

Memory/File/SQLite/Redis 等现有 legacy 路径本期保留、不声称已获得新事务保证；原有安全问题仍在 TODO，不能用于本次四地验收。共享接口接入采用新增能力边界，不强迫所有后端实现一个实际无法保证的事务接口。这一范围调整替代早期“先改齐全部 Cache 实现”的建议。

| 主体 | 候选最小权限 |
| --- | --- |
| Go runtime | BlessingSkin 所需表/列 SELECT；identities SELECT/INSERT 和受控状态列 UPDATE；subjects SELECT/INSERT/UPDATE；tokens/sessions SELECT/INSERT/UPDATE/DELETE；state SELECT；不含 DDL/TRIGGER、BlessingSkin 写、身份/锚点 DELETE |
| 迁移操作者 | 仅在批准窗口拥有指定新表/触发器操作权；不把该凭证发给运行实例 |
| 安全触发器 definer | 仅覆盖已核定的触发器执行与 subject 维护权限；权限和生命周期通过隔离 RDS 兼容验收确定 |

实现核验补充：MySQL 的触发器元数据也受 TRIGGER 权限控制。触发器定义完整性由迁移侧 `VerifyHooks` 核验，成功后才能激活；运行时 Ready 不要求或暗中扩大 TRIGGER 权限。运行账号无法独立发现管理员擅自移除/修改触发器，因此相应运维操作必须先关闭认证门闩并重新验收。[MySQL 触发器元数据权限](https://dev.mysql.com/doc/refman/8.0/en/information-schema-triggers-table.html)

四地必须一致核对主库、auth.mode、schema 版本、令牌期限/限额、密码解释配置、皮肤域白名单、签名公钥指纹、可信直连代理和 RDS CA 摘要。不得输出 DSN、APP_KEY、私钥或实际凭证正文。TLS 主机验证、连接池总数和 RDS 连接预算按五个进程合计，不把四个地区等同于四个连接池。

新模式只读取明确列入清单的 BlessingSkin 账号/玩家/皮肤事实及必要密码解释配置；旧 `ygg_uuid_algorithm` 不再参与分配，旧插件的 token/session/JWT 配置不作为并列事实源。启动时不得预热旧名称 UUID 缓存或缓存账号授权状态；缺少必要密码参数显式失败，不套用占位 APP_KEY/盐值。改变密码解释参数或恢复数据库时，先关闭认证并按经批准计划统一撤销，再更新全部节点，不依赖 users 触发器捕捉配置文件变化。

Go 管理游戏属性签名密钥，但初次切换应保留现有有效密钥材料和公开指纹，单独决定后续轮换，不能启动时每实例随机生成。皮肤上传由 BlessingSkin 承担；新模式停止宣告 Go 支持 uploadableTextures，保留查询、纹理 URL 和签名响应。现有上传路由如何返回不支持需在 HTTP 契约测试中明确，不能无记录地删全局路由。

## 8. 四地切换与验证门槛

当前清单为 SttotHome 两个 K3s 副本、NewNanCity 一个 MCSM 管理的容器、两台阿里云各一个容器。NewNanCity 为 v0.0.12，其余 v0.0.13；这是此前只读快照，不证明本轮它们仍未变化或加载相同配置。

先产出同一不可变镜像摘要和脱敏配置对比计划，再经授权做测试实例/隔离数据库验证和生产窗口。SttotHome 遵从其部署仓库的人工生产操作边界；NewNanCity 只准备实际 `/minecraft/yggrasil-go` 挂载中的约定配置文件，由用户用 MCSM 更新。两台阿里云创建方式仍待核实，不预设 compose 命令。

不得混用新旧认证写者；升级中未就绪节点需从负载入口及可直连入口隔离，不能只改一个上游负载均衡就断言旧节点不可达。NewNanCity 尚未由用户更新时不能悄悄把它作为新方案验证通过的节点。

| 单元 | 完成证据 |
| --- | --- |
| 本轮模型 | 11 个顶层测试，720 种原子事件顺序；覆盖哈希 ABA、撤销交错、明确回滚/未知提交、会话重试、当前归属和 UUID 保留 |
| schema/驱动 | 隔离 MySQL 8：upgrade/downgrade、CHECK/唯一约束、二进制比较、同 pid 竞争、已确认回滚、commit 应答丢失；不能用 SQLite 替代 |
| 安全钩子 | 真实测试库执行普通改密/权限变更/删除；事务回滚不推进版本；触发器失败不得留下已改密码；密码哈希大小写字节变化和 ABA 均撤销 |
| 账号适配 | 实际 BlessingSkin 版本 fixture 校验封禁/密码算法/多角色；禁止把旧文档里的 permission 数值复制进 handler |
| HTTP | Authenticate/Refresh/Validate/Invalidate/Signout/Join/HasJoined 正常、边界、失败；未选角和选角绑定；签名可验证；错误码明确 |
| 多实例 | A 签发、B 刷新、C Join、D HasJoined；任一节点撤销后其他节点拒绝；覆盖 SttotHome 两个副本 |
| 身份 | 旧值不变；改名 UUID 不变；旧名复用获得不同 UUID；旧未映射/冲突 pid 被阻止；新身份提交失败不对外发布 |
| 客户端/游戏服 | 实际启动器能使用不透明 token；服务端/代理使用认证返回 UUID；允许的测试账号改名后保留同一存档 |
| 性能与失败 | 记录四地到 RDS 的 p50/p95/p99、QPS、池等待/锁等待/死锁、过期清理和故障恢复；容量/延迟阈值根据基线与业务承受度批准，不虚构数字 |
| 安全上线门槛 | 可信代理、请求体/批量数量上限、认证限流和依赖漏洞整改各自验证；共享状态正确不等于完整安全审查结束 |

## 9. 规格验证与批准状态

前一轮仅新增规格和本地确定性模型。模型将事务当作原子步骤，使用合成 ID 和手动时间；令牌场景限定已选角，名称只模拟精确相等。不验证 SQL 锁/死锁、哈希强度、真实 UUID 格式、完整回填分类器、未选角流程或 HTTP。后续实现和真实 MySQL 证据见 [模块说明](../src/sharedauth/README.md)，不能把该模型本身当作数据库并发证明。

本地运行：

```powershell
YGG_TEST_MYSQL=1 go test -race -count=1 -timeout 300s ./...
go vet ./...
git diff --check
```

2026-09-07 全仓命令通过；2026-09-08 另完成真实 RDS 验证 TLS 和只读迁移预检。尚未完成真实启动器和游戏服验收，不能用测试结果替代生产切换验收。

用户已批准以下变更的本地实现及隔离数据库验证，生产执行仍另设门槛：

1. 采用五张 Go 自有表、单库事务服务、opaque token 和显式 shared_mysql 模式；其他后端留在 legacy，不宣称一起修复。
2. 允许设计并在隔离环境实现两个 users 安全触发器，改密/权限变化/删除与撤销版本同事务；接受钩子失败会使 BlessingSkin 对应修改失败的耦合。生产安装另行批准。
3. HasJoined 改为原短期窗口内可重复校验；真实基础设施失败用 5xx，不伪装认证成功/无匹配。

用户另已批准 Authenticate 超额淘汰最早旧令牌的规则。实际接入前还需核实 BlessingSkin 版本与账号语义；发布前需确认全量冲突清单及受影响人数、配置一致性、短时角色变更冻结、所有旧入口隔离、会话统一失效和各节点操作者。上述未知项不得自动补成默认生产行为。
