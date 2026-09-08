---
type: docs
scope: docs
audience: internal
summary: 核实 UUID 复用顺序与批量冲突返回缺陷，保留历史中文串号根因的不确定性
breaking: false
demo_ready: false
tests:
  - "go test -race -overlay tmp/uuid-review-20260907/overlay.json -count=1 -timeout 60s -run '^(TestUUIDReview|TestReviewUUIDInsertFailureStillPublishesIdentity|TestReviewPlayerRenameChangesIdentityAndReusedNameInheritsUUID)' -v ./src/storage/blessing_skin"
artifacts:
  - TODO.md
  - working-delta/2026-09-07-uuid-collision-review.md
---

## What changed

记录当前 Go 和上游历史分配逻辑：先复用名称映射，缺失时才生成，没有碰撞重生成。隔离测试确认已有值不因算法切换而重算；在预置冲突映射时，批量创建会吞掉唯一约束错误并发布另一玩家的 UUID。

对用户补充的两个历史名称做仅内存复核，报告只保留字符数、UTF-8 字节数和比较结论，不保存原名称或具体 UUID。它们各自可能与其他玩家串号，不能视为二者彼此串号。

## Why it matters

UUID 生成算法与身份映射完整性需要分别判断。中文名称的有限生成测试未发现合并，上游 2022 年的公开报告提供了并发、改名和绑定方面的线索，但不能据此确定用户历史事故的根因或修复状态。

## Demo posture / limitations

仅新增诊断记录和临时测试，未修改业务代码、生产配置或数据库，也未发布。临时传入用户样本后，7 个顶层测试通过，包含对仍存缺陷的复现，不代表安全修复验收。未设置 `UUID_REVIEW_NAMES_JSON` 时，样本用例明确跳过，其余 6 个用例可运行。命令依赖本地被忽略的临时 overlay 及前轮 fixture，并非正式 CI 覆盖；历史中文串号、生产索引和名称比较规则仍未核定。
