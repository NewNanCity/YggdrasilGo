---
type: docs
scope: docs
audience: internal
summary: 只读核验 UUID 和角色表缺少名称索引及 UUID 唯一约束，并记录业务查询暂停边界
breaking: false
demo_ready: false
tests:
  - "go test -race -count=1 -timeout 30s -v ./tmp/sttothome-review-20260906"
  - "go run ./tmp/sttothome-review-20260906 --uuid-schema"
artifacts:
  - TODO.md
  - working-delta/2026-09-07-uuid-live-preflight.md
  - working-delta/2026-09-07-uuid-collision-review.md
  - working-delta/2026-09-06-project-audit.md
---

## What changed

记录当前 RDS 的 uuid/players 元数据：两表均为 InnoDB，名称字段均采用 utf8mb4_unicode_ci，索引仅有各自的 ID 主键。三个名称/UUID 查询的 EXPLAIN 均为 ALL；按本轮预设边界暂停业务记录读取。

## Why it matters

此前未核验的生产索引现在有只读证据。名称及 UUID 的唯一性没有数据库约束保障，但这不等同于已经发现重复记录，更不能定性历史中文串号原因。

## Demo posture / limitations

5 个诊断测试及 race 检查通过，线上只执行元数据 SELECT 和普通 EXPLAIN。未查询两个历史名称的实际记录，未修改业务代码、schema、配置或数据，也未发布。估计行数不是精确计数；诊断代码仍在被忽略的临时目录，含线上访问的命令不是 CI 步骤。继续限时扫描需要用户确认。
