---
type: docs
scope: docs
audience: internal
summary: 完成两个历史名称的授权只读核验，记录当前无共用 UUID 证据及旧名遗留映射
breaking: false
demo_ready: false
tests:
  - "go test -count=1 -timeout 30s -v ./tmp/sttothome-review-20260906"
  - "go test -race -count=1 -timeout 30s ./tmp/sttothome-review-20260906"
  - "go vet ./tmp/sttothome-review-20260906"
  - "go run ./tmp/sttothome-review-20260906 --uuid-mappings"
artifacts:
  - TODO.md
  - working-delta/2026-09-07-uuid-targeted-mapping.md
  - working-delta/2026-09-07-uuid-live-preflight.md
  - working-delta/2026-09-07-uuid-collision-review.md
  - working-delta/2026-09-06-project-audit.md
---

## What changed

用户批准后，在一个只读事务内完成最多 8 条、单条设 2 秒期限的定向 SELECT。两个旧名各有一条符合完整名称 v3 计算值的映射，未查到对应 UUID 关联其他名称。样本 A 有一个同名角色；样本 B 的旧名无角色行，但映射还在。新增脱敏结果记录并同步审查与 TODO 状态。

## Why it matters

当前记录没有提供这两个中文名称串号的直接证据，也没有证明历史事故已修复。旧名缺少角色行与账号删除、角色改名是不同结论；保留原始映射，避免未经归属确认改变玩家身份。

## Demo posture / limitations

8 个诊断测试、race 与 vet 通过，获批线上查询成功。未修改业务实现、schema、配置或数据，未提交或发布。本次不代表全库无重复、四地缓存一致或历史根因已确定；现有代码风险仍待整改。诊断位于已忽略的临时目录，实际名称仅通过临时进程环境输入，线上命令不是 CI 步骤。
