---
type: docs
scope: docs
audience: internal
summary: 记录 UUID 算法配置为 v3 的只读证据并修正改名方案前提
breaking: false
demo_ready: false
tests:
  - go test -count=1 -timeout 30s ./tmp/sttothome-review-20260906
  - go run ./tmp/sttothome-review-20260906 --uuid-algorithm
artifacts:
  - TODO.md
  - working-delta/2026-09-06-project-audit.md
  - working-delta/2026-09-06-remediation-brief.md
---

## What changed

记录限定只读查询结果：`ygg_uuid_algorithm` 匹配一行，配置值为 v3；按 id 的前 32 条 UUID 样本中，31 条为 v3、1 条为 v4。方案中的 pid 身份表降为历史候选，不默认推进全站改名保 UUID 的触发器。

## Why it matters

实际配置与历史值不能由默认值或单条 UUID 推断。v3 的名称生成约定不能被当成 v4 改名缺陷处理，修复仍必须保留已有 UUID，并避免静默改变身份语义。

## Demo posture / limitations

没有修改生产数据、schema、配置或插件，没有发布。样本不是全库分布，也没有识别那一条 v4 的来源或读取各进程的配置缓存。

诊断程序与测试位于现有 Git 忽略目录 `tmp/sttothome-review-20260906/`，不是干净 checkout 自带的产品功能。真实查询只在显式 `--uuid-algorithm` 模式运行，使用只读事务和有界 SELECT；不输出或保存凭据、玩家名和具体 UUID。以上命令是本次证据记录，不是要求 CI 自动连接生产数据库。
