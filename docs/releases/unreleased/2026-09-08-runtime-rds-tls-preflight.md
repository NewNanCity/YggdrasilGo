---
type: test
scope: runtime
audience: internal
summary: 完成 Aliyun RDS 验证 TLS 探针和新 UUID 策略生产只读预检
breaking: false
demo_ready: false
tests:
  - "openssl s_client -starttls mysql -connect <rds-endpoint>:3306 -servername <rds-endpoint> -CAfile .local/shared-auth/ApsaraDB-CA-Chain.pem -verify_return_error -brief"
  - "go run .local/shared-auth/tls_probe.go"
  - "go run ./cmd/shared-auth-migrate dry-run -config - -plan .local/shared-auth/production-20260908-rds-tls-v3.json -timeout 2m -max-rows 100000"
artifacts:
  - docs/shared-auth-deployment.md
  - TODO.md
---

## What changed

检查控制台 CA 压缩包的 3 个安全相对路径，仅将 PEM 提取到 Git 忽略目录。PEM 含 81 张 CA，检查时全部在有效期内。OpenSSL 和项目正式 MySQL TLS 配置均验证实际 RDS endpoint 的证书链与主机名，数据库会话协商 TLS 1.2，`Ssl_cipher` 与 `Ssl_version` 均非空。

通过实时 Secret 的内存管道补齐 CA、主机名和连接/读写超时，再执行只读迁移预检。快照包含 3311 players、2738 mappings：3305 active、6 blocked、187 reserved、0 invalid。756 个无旧映射玩家使用确定性 OfflinePlayer v3；blocked 仅包含 4 个排序规则等价名称和 2 个重复旧名称。规范计划摘要为 `bca43c5ddfae25621f9e488f8fa80261d264ab303f0f4ae1198e50f727969a10`。

## Why it matters

生产读路径已经证明 CA 信任、endpoint 身份和迁移分类同时成立，不再依赖跳过验证的 TLS 或旧的 762 条阻塞计划。账号、密码、DSN、玩家名和 UUID 均未写入仓库或终端证据。

## Demo posture / limitations

这次不代表已修改 RDS schema/数据、已冻结 BlessingSkin 角色写入、已激活 shared_mysql、已发布镜像或已更新四地配置。CA 和计划仍只在本地受忽略目录；生产 Secret、CA 挂载、可信回源身份、RDS 迁移权限与触发器 definer 仍需完成。当前计划是可复核预检，不得替代维护窗口冻结后的最终快照。
