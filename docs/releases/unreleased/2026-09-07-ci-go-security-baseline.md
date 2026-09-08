---
type: fix
scope: ci
audience: developer
summary: 将构建基线升级到 Go 1.26.6 并修复全部可达 Go 漏洞
breaking: false
demo_ready: false
tests:
  - "govulncheck ./..."
  - "go test -race -count=1 -timeout 180s ./..."
  - "go vet ./..."
  - "go build ./..."
artifacts:
  - go.mod
  - go.sum
  - Dockerfile
  - .github/workflows/build-test.yml
  - .github/workflows/release.yml
---

## What changed

将本地模块、Docker 构建和 GitHub Actions 统一到 Go 1.26.6。按 Go 漏洞数据库给出的最低修复版本升级 `x/text`、`x/net` 和 `quic-go`，并接受同一依赖族要求的 `x/crypto`、`x/sys` 兼容升级。

## Why it matters

升级前扫描发现 10 条可达漏洞，涉及标准库的 URL、TLS、HTTP、ASN.1、X.509，以及 Unicode 规范化和 HTTP/3。升级后 `govulncheck ./...` 报告 0 条可达漏洞，生产镜像不会继续使用已知受影响的 Go 1.26.3/1.26.0 基线。

## Demo posture / limitations

这次只说明当前调用图没有已知可达 Go 漏洞，不等于容器基础层、运行环境或未来新增路径永久无漏洞。模块图仍可能包含当前代码未调用的受影响包，需在后续发布继续扫描。尚未构建或推送生产镜像。
