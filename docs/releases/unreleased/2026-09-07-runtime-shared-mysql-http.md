---
type: feature
scope: runtime
audience: internal
summary: 将共享 MySQL 认证核心接入 BlessingSkin 同库启动、HTTP 会话和稳定角色资料路径
breaking: false
demo_ready: false
tests:
  - "YGG_TEST_MYSQL=1 go test -race -count=1 -timeout 240s -v ./src/sharedauth/... ./src/handlers ./src/storage/blessing_skin"
  - "go test -race -count=1 -timeout 120s ./..."
  - "go vet ./..."
  - "go build ./..."
  - "git diff --check"
artifacts:
  - main.go
  - conf/example.yml
  - src/config/config.go
  - src/sharedauth/profiles.go
  - src/handlers/auth.go
  - src/handlers/session.go
  - src/handlers/profile.go
  - src/handlers/shared_auth.go
  - src/storage/blessing_skin/storage.go
  - src/storage/blessing_skin/profiles.go
  - src/handlers/shared_auth_test.go
  - src/storage/blessing_skin/shared_auth_test.go
  - src/sharedauth/README.md
  - docs/shared-auth-design.md
  - TODO.md
---

## What changed

新增显式 `auth.mode: shared_mysql` 启动分支，复用 BlessingSkin 存储已经建立的同一 `sql.DB` 连接池。该模式要求 BlessingSkin 后端和 active 共享 schema，不实例化 legacy token/session cache，不预热旧名称 UUID 或账号缓存，也不初始化未使用的 JWT secret；省略 mode 继续采用 legacy。

Authenticate、Refresh、Validate、Invalidate、Signout、Join 和 HasJoined 已接入共享事务服务。公开 UUID/名称/批量资料查询改用 pid 稳定身份，按当前玩家名称和所有者读取；新水位后的角色只在显式查询时申请 v4，旧水位内缺映射和重复名称保持隔离。批量查询按 pid 稳定锁序并去重，避免相反请求顺序制造死锁。

BlessingSkin 适配器从同库读取当前玩家和纹理，未知密码方法拒绝启动 shared_mysql，DSN 缺少 `parseTime=true` 同样拒绝启动。共享资料查询传播请求取消并使用 `security.read_timeout`；不再宣告 `uploadableTextures`。现有材质上传/删除路由保留并对 BlessingSkin 返回 501。签名、资料或数据库故障返回 5xx，不伪装成功，也不消费可重复 HasJoined 会话。

## Why it matters

四个实例可以把令牌、撤销版本、入服会话和 UUID 归属放在同一 RDS MySQL 事务事实源中，不再依赖各节点本地 cache 或旧 `uuid.name` 映射协调。角色改名后 UUID 保持不变，旧名称复用不会继承原身份；密码或权限变化后的令牌判定也由当前数据库状态决定。

legacy 默认和构造函数保持可用，生产未显式选择新模式时行为不变。共享模式在 schema、后端、状态门闩或密码方法不满足时拒绝启动，不静默回退到已知不一致的旧路径。

## Demo posture / limitations

这次不代表生产已迁移、四地已切换或真实客户端已经验收。没有连接生产、执行 DDL/回填、安装触发器、修改线上配置、使现有会话失效、重启实例或发布镜像。NewNanCity 仍由用户通过 MCSM 更新实例。

生产 BlessingSkin 版本、封禁值、实际密码方法/盐参数、RDS 权限与跨地域延迟仍需迁移前核实。旧 UUID 分类和回填工具、可信代理、请求体上限、过期记录清理、签名公钥指纹核对及启动器/游戏服验收尚未完成。“资料查询进入等待后取消”和“批量候选解析后删除单项”的受控交错仍缺定向测试；shared_mysql 的本地正确性不等于完整安全整改结束。

## Verification

隔离 MySQL 8.0.46 测试覆盖原共享认证场景及新增的稳定资料解析、相反批量顺序并发、HTTP 全流程、错误映射、BlessingSkin 同库账号/纹理适配、上传能力声明和签名失败。所有容器均由测试按精确 ID 清理，未访问外部数据库。
