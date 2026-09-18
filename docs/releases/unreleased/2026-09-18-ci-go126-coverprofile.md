---
type: fix
scope: ci
audience: developer
summary: 修复 Go 1.26.6 覆盖率参数被误解析为包路径的问题
breaking: false
demo_ready: false
tests:
  - "go test -coverprofile coverage.out ./cmd/shared-auth-migrate"
artifacts:
  - .github/workflows/build-test.yml
---

## What changed

GitHub Actions 的覆盖率命令改用 `-coverprofile coverage.out` 参数形式。

## Why it matters

Go 1.26.6 会把原来的 `-coverprofile=coverage.out` 误解析为额外的 `.out` 包路径，导致 race 已通过后覆盖率步骤失败。空格形式会正常生成覆盖率文件。

## Demo posture / limitations

这只修复 CI 命令解析，不改变服务二进制、覆盖率内容或生产行为。
