# MySQL 共享认证核心

对应 [已批准规格](../../docs/shared-auth-design.md)。事务核心现已接入 `shared_mysql` 启动模式及认证、会话、公开资料 HTTP 路径；省略模式仍为 `legacy`。这只代表本地代码和隔离 MySQL 验证完成，不能作为生产迁移或四地切换完成的依据。

## 当前接口

| 接口 | 行为 |
| --- | --- |
| `New` / `EnsureIdentity` | 使用调用方提供的同库连接池；当前 pid 解析稳定身份，只有切换水位之后的新 pid 可申请 v4 |
| `NewAuth` | 另要求显式 `AccountPolicy`、事务期限和令牌期限；账号规则由已核实版本的 BlessingSkin 适配器负责，不能使用测试策略上线 |
| `Authenticate` | 先验证密码再在事务中复核当前账号；申请/列出已确认身份，在账号限额内签发令牌，满额则淘汰最早旧令牌 |
| `Ready` | 检查 active 门闩、schema 版本、主库只读标志及必要 InnoDB 表；不创建/修复 schema |
| `Validate` / `Refresh` | 核验当前账号、撤销版本、角色归属和客户端绑定；刷新原子删旧插新，提交后才返回不透明 token |
| `Invalidate` / `Signout` | 精确撤销或推进账号撤销版本；不存在 token 可幂等处理，数据库故障不能当作成功 |
| `Join` / `HasJoined` | 30 秒共享会话；同绑定重试不延长期限，验证每次重查授权与当前名称，不消费会话 |
| `ResolveIdentityByUUID` / `ResolveIdentityByName` | 公开资料使用 pid 稳定身份和当前名称；新水位后的角色可在显式查询时申请 v4 |
| `ResolveIdentitiesByNames` | 最多 10 个名称在单事务内解析，按 pid 稳定加锁并去重；缺失和隔离身份不返回 |
| `CleanupExpired` | 每表最多 10,000 行的有界事务清理；四地可并发执行，不依赖连接级全局锁 |

连接池由调用方管理，要求真实 MySQL 8.0.22+、主库路由、`parseTime=true`、UTC 时间解析及有界连接/读写超时；shared_mysql 强制使用单独 CA 文件、匹配 DSN endpoint 的主机名和 TLS 1.2+，拒绝 DSN 自带的宽松 TLS 模式。所有领域事务使用一个 `sql.Tx` 和 READ COMMITTED；不能把三个独立 cache DSN 拼成一个事务。前置非锁定读取只定位不可变 ID，授权条件在按 users、subjects、token、players 的顺序锁定后重查。

`AccountPolicy` 只解释已读取的账号状态与密码，不另开数据库连接或从节点缓存返回账号授权结论。BlessingSkin 适配器复用存储层的同一个 `sql.DB`；未知密码方法拒绝启动 shared_mysql，不回退猜测。当前封禁值和密码实现仍来自仓库既有适配器，生产安装版本及实际配置必须在迁移前核实。

返回值只在确认提交后发布。`ErrCommitUnknown` 表示 COMMIT 可能已生效但应答丢失，调用者不能自行补发第二个 token，也不能向用户保证已回滚。HTTP 已区分无效凭据/令牌、无匹配会话、未就绪和后端故障；签名或资料读取失败返回 5xx，不消费可重复会话。

## 登录限额

`Authenticate(ctx, username, password, clientToken, tokenLimit)` 返回 `AuthenticateResult`，其中 `Token` 是签发结果，`AvailableProfiles` 是当前可用身份。新增方法不改变已有 NewAuth/Refresh 签名。`tokenLimit` 必须来自可信服务配置且为正整数，不能来自客户端输入；0/负数返回 `ErrNotReady`，不静默解释为无限额。四地实际限额值及配置接入仍待核实。

只统计同一 uid、当前 generation、数据库查询时未过期的令牌。达到上限时，按 `created_at` 升序淘汰恰好足够腾出一个名额的旧令牌；同一微秒签发时按二进制摘要排序。刷新后的令牌以其新的签发时间参与排序。旧版本、到期记录及其他账号不占当前额度，本方法也不顺手删除它们。

账号 subject 锁串行化各实例的登录、刷新与撤销。先锁定旧令牌，再读取当前玩家/申请新身份；新 token 插入成功后才删除选定旧值，全部操作一并提交。碰撞候选不能复用待淘汰的旧凭证；密码错误、存储/随机错误、无效选角、取消或明确回滚时不提前淘汰。已淘汰令牌对应的会话即使尚未清理，也因 token 不存在而拒绝。

邮箱登录有一个可用角色时自动选择，多个或没有可用角色时签发未选角令牌；角色名登录解析当前唯一 pid，不能用客户端拼写或旧映射绑定身份。迁移时无旧映射且无碰撞的角色补齐确定性 v3；真实名称冲突、blocked/retired 角色不进入可用列表，显式选中它们返回 `ErrIdentityConflict`。新角色申请身份失败使整个登录回滚，不伪装成角色列表为空。省略 clientToken 时生成兼容的无连字符随机 v4 字符串；调用方提供的值按字节保存并返回。

## 显式迁移

[migrations](migrations/migrations.go) 包提供 `Upgrade`、`VerifyHooks` 和 `Downgrade`，无 CLI 自动执行入口。七个 SQL 文件各是一条语句，包含五张 Go 表和两个 users 触发器，不包含任何旧 UUID 回填或 state 激活数据。

Upgrade 要求经批准的独占维护窗口：先核实版本、users 引擎和既有触发器；遇到已有目标表停止，不自动采用或覆盖。MySQL DDL 分步提交，失败时可能留下部分新结构，应人工检查，不能盲重试或自动删除。Downgrade 仅允许所有新表完全无数据且所有写者已停止的场景；即使只有撤销锚点或 staged 数据也拒绝删除。代码的空表检查不能代替关闭并发写者。

触发器默认以执行 DDL 的 MySQL 身份作为 definer，生产操作者必须先确定稳定身份及所需长期权限；不能用执行后立即删除的临时账号安装。实际阿里云 RDS 权限、binlog/触发器限制及 BlessingSkin 写路径尚未核验。

`VerifyHooks` 检查两个触发器的名字、所属表、时机、事件及完整正文。该查询需要 users 的 TRIGGER 权限，必须由迁移/运维侧完成，再将经验证的数据计划置 active；运行账号不能为检查元数据获得创建/删除触发器的能力。运行时 Ready 不能发现被高权限操作者擅自改写/删除的触发器，运维必须先关闭认证门闩，再操作/复核安全对象。[MySQL 元数据权限](https://dev.mysql.com/doc/refman/8.0/en/information-schema-triggers-table.html)

[migrationplan](migrationplan/README.md) 与 `cmd/shared-auth-migrate` 已实现全量分类、私有 dry-run、staged apply、verify、activate 和保留数据的 deactivate。工具完成不等于生产授权；禁止手工激活空迁移或绕过 plan SHA-256/数据库名确认。所有已有 UUID 保留，不重新生成或清理孤立行。

## 验证

不使用生产 DSN；测试助手只连接自己新建的本机容器，不接受外部数据库地址。首次需要下载固定镜像，随后测试使用不可变摘要且 `--pull=never`：

```powershell
docker --context desktop-linux pull mysql:8.0.46@sha256:7dcddc01f13bab2f15cde676d44d01f61fc9f99fe7785e86196dfc07d358ae2b
$env:YGG_TEST_MYSQL = '1'
go test -race -count=1 -timeout 180s -v ./src/sharedauth/...
Remove-Item Env:YGG_TEST_MYSQL
go test -race -count=1 -timeout 120s ./...
go vet ./...
go build ./...
```

Windows 从 `desktop-linux` context 读取本地管道，其他平台从 `default` 读取本地 Unix socket；严格拒绝远端命名管道、TCP/SSH 及有歧义的地址。后续命令固定使用已验证的 endpoint，不随 context 变化重新解析。临时密码每次随机生成，端口只绑定 loopback，数据在 tmpfs 中。测试退出时只删除它创建的精确容器 ID 和测试数据，不清理其他容器/卷/镜像。正常 `go test ./...` 不启用 Docker 集成测试。

当前隔离 MySQL 在原 68 个核心场景外，新增稳定资料解析、相反批量顺序并发、HTTP 全流程及 BlessingSkin 同库适配场景。覆盖 UUID 反查、改名保 UUID、新角色 v4、旧角色隔离、批量 pid 锁序、账号策略、纹理 URL、上传能力声明及签名失败。Service 对象共用隔离数据库连接池，不等于四地多进程或客户端验收。

另有 5 个模拟驱动事务结果场景、24 个 Docker endpoint 校验场景及 SQL 文件 LF/CRLF 回归测试。COMMIT 应答丢失只由驱动模型验证，未在真实网络中注入；本地性能不代表四地 RDS 延迟。

## 接入边界与尚未完成

`main.go` 仅在显式 `auth.mode: shared_mysql` 时复用 BlessingSkin 主连接、跳过 legacy token/session/user/UUID 缓存，并要求 active schema；故障不会回退 legacy。认证、会话、资料和 501 材质管理契约已接入，shared_mysql 不再要求或初始化 JWT secret，也不宣告 `uploadableTextures`。

生产 schema 安装、回填和激活、真实 BlessingSkin 版本/封禁/密码参数核验、四地延迟与真实客户端验收仍未完成。HTTP 已启用请求大小/超时配置；代理头只对 `server.trusted_proxies` 中的直连来源生效。shared_mysql 启动时先做一次有界过期清理权限检查，随后每五分钟清理。资料适配器已验证预先取消，但“查询已进入数据库等待后取消”和“批量候选解析后恰好删除一名玩家”的受控交错仍是测试缺口。现有 legacy 路径的问题不因本模块通过而消失；NewNanCity 的实例更新仍由用户通过 MCSM 执行。
