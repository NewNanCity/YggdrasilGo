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

代码与固定 MySQL 8.0.46 正向/回退验证已完成。本条目不代表镜像或生产 schema/身份数据已经发布；PHP OAuth/OIDC 旧读路径不在本轮范围内。
