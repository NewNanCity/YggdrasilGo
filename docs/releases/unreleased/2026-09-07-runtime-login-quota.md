---
type: feature
scope: runtime
audience: internal
summary: 实现共享认证登录及满额淘汰最早旧令牌的原子规则，保留既有 UUID 和失败前状态
breaking: false
demo_ready: false
tests:
  - "YGG_TEST_MYSQL=1 go test -race -count=1 -timeout 180s -v ./src/sharedauth/..."
  - "go test -race -count=1 -timeout 120s ./..."
  - "go vet ./..."
  - "go build ./..."
  - "git diff --check"
artifacts:
  - src/sharedauth/authenticate.go
  - src/sharedauth/auth.go
  - src/sharedauth/authenticate_test.go
  - src/sharedauth/authenticate_concurrency_test.go
  - src/sharedauth/README.md
  - docs/shared-auth-design.md
  - working-delta/2026-09-06-remediation-brief.md
  - TODO.md
---

## What changed

按用户批准实现 Authenticate：先验证密码，再在同库事务内复核当前账号、登录标识、撤销版本和角色归属。缺失锚点只创建一次；UUID 仍绑定 pid，已有值保留，旧未映射和冲突角色不会获发猜测值。邮箱支持单角色自动选择和未选角令牌，名称登录解析当前唯一 pid。

在账号锁下统计当前 generation 的未到期令牌，满额时按签发时间淘汰必要数量的旧记录，同一时间用二进制摘要稳定排序。先锁定旧值，成功插入新值后才删除选定旧值，同事务提交；过期、旧版本和其他 uid 不计数或附带清理。上限由可信调用方传入正整数，不改已有构造函数、schema、HTTP 或 legacy 路径。

登录与 Refresh 复用一个内部令牌签发函数，保留不透明随机值、数据库仅存摘要、三次唯一键碰撞上限和刷新旧值避让。未添加依赖、配置默认值、明文响应账本或自动重试事务。

## Why it matters

用户达到上限仍能登录；只有新登录确实提交才会失去最早的旧令牌。被淘汰令牌的会话因权威记录不存在而失效。错误密码、无效角色、写入/删除错误、随机失败和取消不能提前挤掉原有效登录；未知提交仍如实返回，不能假称回滚或补发。

新增 22 个登录场景与 12 个并发/快照场景，覆盖四个服务对象的 8 个并发登录、首次锚点、双向刷新交错、后提交改密/Signout、锁等待取消，以及验证密码后账号/归属/名称变化。关键交错先观测真实 InnoDB 锁等待，再释放控制事务。独立只读复核未发现新的 P0/P1/P2。

## Demo posture / limitations

这次不代表现有 API 已使用新登录规则、四地配置已更新、真实 BlessingSkin 账号适配已验收、旧 UUID 已迁移或生产已上线。本轮只做本地实现和临时 MySQL 验证，没有生产访问、DDL、令牌失效、提交、推送或部署；NewNanCity 仍由用户通过 MCSM 更新。

测试的账号策略为合成 fixture，不证明实际安装版本的权限、邮箱验证或密码算法。四个 Service 共用本地数据库连接池，不是跨地域多进程/启动器验收；提交应答丢失仍仅经驱动模型验证。真实配置值、RDS 权限/延迟、请求体/批量限制、资料签名和清理调度仍需各自完成。

## Completed Plan

- 已先复现满额登录未签发 token，再实现并通过该用例和原 19 个认证场景。
- 已完成 22 个正常/边界/失败场景，包含最小运行权限；失败不发布新 token/UUID。
- 已完成 12 个并发/快照复核场景，并完成独立只读审查。
- 已完成全套 MySQL 与全仓回归、文档/格式核验和临时容器清理；本轮范围之外的接入和发布任务保留在 TODO。

## Verification

最终 `YGG_TEST_MYSQL=1 go test -race -count=1 -timeout 180s -v ./src/sharedauth/...` 通过 68 个 MySQL 子场景，含本轮新增 34 个登录/并发场景，并继续通过 5 个事务驱动场景及 SQL 换行回归。全仓 `go test -race -count=1 -timeout 120s ./...`、`go vet ./...`、`go build ./...` 通过；普通全仓测试不自行启用 MySQL，真实数据库证据来自上述单独命令。

`gofmt -l`、`git diff --check` 及本轮文档的本地链接/行尾空白检查通过。测试标签下没有残留容器；测试只删除自行创建的精确容器 ID 及其合成数据，固定镜像缓存保留，未清理用户容器或其他卷。
