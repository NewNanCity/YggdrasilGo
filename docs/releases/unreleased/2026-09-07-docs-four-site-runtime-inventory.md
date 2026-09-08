---
type: docs
scope: docs
audience: internal
summary: 记录四地五个目标进程的版本与挂载差异，并同步已确认的身份管理方向
breaking: false
demo_ready: false
tests:
  - "git diff --check"
  - "Bounded SSH metadata checks: Docker ps/inspect/image inspect on three hosts; scoped K3s Pod fields on SttotHome"
artifacts:
  - TODO.md
  - working-delta/2026-09-07-four-site-runtime-inventory.md
  - working-delta/2026-09-06-remediation-brief.md
  - working-delta/2026-09-06-project-audit.md
---

## What changed

经用户明确批准，通过四个现有 root SSH 入口完成限定只读核验。SttotHome 运行两个同节点 K3s 副本，其余三处各一个 Docker 容器；NewNanCity 使用 v0.0.12，其余 v0.0.13 摘要一致。记录配置挂载来源及 Docker 重启策略，未读取配置正文。用户随后确认 NewNanCity 由 MCSM 管理，并保留实例更新操作；Go 侧后续只准备/更新约定配置，不直接操作该实例生命周期。

同步用户已确认的目标：BlessingSkin 管理用户、角色和皮肤，Go 提供游戏 API；保留旧 UUID、新身份用 v4、UUID 关联 pid、改名保持身份、改密撤销旧令牌。具体契约、schema、回填及发布仍待确认。

## Why it matters

后续切换要覆盖至少这 5 个进程，并区分 Kubernetes、MCSM 管理的 Docker 与其他 Docker 入口。镜像版本及配置挂载差异已有运行证据，但不能将相同镜像摘要当作相同配置，不能把 Docker 的 restart=no 单独视为缺少守护，也不能假定旧名称关联逻辑可与新身份方案长期混用。

## Demo posture / limitations

仅更新本地审查、方案状态及任务记录，没有业务代码改动、数据库访问、容器 exec、配置读取、线上修改、提交或发布。该核验不代表认证功能、运行配置一致性或跨节点性能验收；NewNanCity 未升级，重启策略和可写配置挂载均未调整。运行元数据查询不是 CI 步骤。
