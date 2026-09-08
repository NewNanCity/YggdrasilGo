---
type: docs
scope: docs
audience: developer
summary: 重写根 README，使其反映当前 Yggdrasil 与 BlessingSkin 的职责边界和 shared MySQL 运行方式
breaking: false
demo_ready: false
tests:
  - "go test ./cmd/... ./internal/... ./src/... ./test/..."
  - "go vet ./cmd/... ./internal/... ./src/... ./test/..."
  - "go build ./cmd/... ./internal/... ./src/... ./test/..."
  - "git diff --check"
  - "README local-link validation"
artifacts:
  - README.md
  - docs/releases/unreleased/2026-09-08-docs-readme-current-architecture.md
  - TODO.md
---

## What changed

根 README 移除过时的 Docker Compose、性能承诺、历史实现细节和已失真的兼容性描述，改为说明当前 API、legacy/shared_mysql 模式、BlessingSkin 职责边界、UUID 规则、最小开发命令及生产配置限制。

## Why it matters

维护者可以从仓库入口快速判断 Go 服务与 BlessingSkin 的数据所有权，避免将网页用户管理、材质上传或旧 UUID 名称映射误接入 shared_mysql 路径。

## Demo posture / limitations

本文档不代表新建生产迁移、改变 RDS、重新签发令牌，或完成真实启动器和游戏服验收。生产部署仍应使用不可变镜像 digest、受控 Secret 和既有维护窗口。根级 `go test ./...` 目前会发现 Git 忽略的历史 `tmp/*review*` 审查包；本次只验证受版本控制的项目包。
