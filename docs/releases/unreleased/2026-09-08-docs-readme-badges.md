---
type: docs
scope: docs
audience: developer
summary: 恢复 README 的仓库状态和社区链接徽标
breaking: false
demo_ready: false
tests:
  - "README local-link validation"
  - "git diff --check"
artifacts:
  - README.md
  - docs/releases/unreleased/2026-09-08-docs-readme-badges.md
---

## What changed

恢复根 README 顶部的 Go 版本、许可证、v0.0.14 Release、main CI、Star、Fork 和 Issue badge，并链接到各自的仓库页面。

## Why it matters

读者可在不离开项目入口的情况下查看版本、构建状态、许可证与社区入口；正文仍只描述当前架构，不重新引入过时的功能承诺。

## Demo posture / limitations

badge 由 GitHub 和 Shields 实时生成，显示状态取决于外部服务；本次不改变发布、构建工作流或部署。
