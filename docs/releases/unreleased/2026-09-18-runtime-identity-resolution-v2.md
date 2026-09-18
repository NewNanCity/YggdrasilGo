---
type: fix
scope: runtime
audience: internal
summary: 为人工确认的 blocked UUID 身份增加可审计解析和回退流程
breaking: false
demo_ready: false
tests:
  - "$env:YGG_TEST_MYSQL='1'; go test -race -count=1 -timeout 300s ./src/sharedauth/... ./src/handlers ./src/storage/blessing_skin"
  - "go test -race -count=1 -timeout 180s ./cmd/... ./internal/... ./src/... ./test/..."
  - "go vet ./cmd/... ./internal/... ./src/... ./test/..."
  - "go build ./cmd/... ./internal/... ./src/... ./test/..."
  - "git diff --check"
  - "go run ./cmd/shared-auth-migrate resolution-verify -config - -plan .local/shared-auth/resolution-production-20260919.json -timeout 2m"
  - "生产公开 profile、大小写变体单查与六项批量查询断言"
artifacts:
  - src/sharedauth/migrations
  - src/sharedauth/resolutionplan
  - cmd/shared-auth-migrate
  - docs/shared-auth-design.md
  - docs/shared-auth-deployment.md
---

## What changed

运行时同时接受 schema v1/v2，未知版本继续 fail closed。schema v2 增加 resolved 审计链接、自引用外键和数据库级自解析保护；DDL、版本激活和身份数据迁移保持独立。CLI 新增私有审批 dry-run、摘要确认的 apply/verify/activate/deactivate/rollback，并允许原始计划在解析回退后于 v2 重新开闸。

## Why it matters

人工确认的 blocked 玩家与 reserved 历史 UUID 可以在不删除身份行、不重生成 UUID 的前提下恢复稳定归属。

## Demo posture / limitations

`v0.0.15` 已从源码提交 `902a3613779fd2fccee32bfbf80543f0516c5db7` 发布；四个运行位置固定到阿里云多平台摘要 `sha256:c75dc1a17293cb23d49a20aa56005711aa2db7a31ef20f462512caa8d9effc65`。生产 schema v2 已 active，六组解析全部 verify；身份总数保持 3500，状态为 3313 active、181 reserved、6 resolved、0 blocked，相关十二条身份仍无 token/session 引用。六个历史 UUID 的公开 profile、大小写变体名称单查和批量查询均返回原 UUID。

本轮没有使用真实玩家密码执行 Authenticate/Refresh/Join，也不处理 PHP OAuth/OIDC 旧读路径、玩家展示名称、NNM 绑定或处罚。
