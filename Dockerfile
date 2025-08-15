# 构建阶段
FROM --platform=$BUILDPLATFORM golang:1.24.5-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app
RUN apk add --no-cache git ca-certificates tzdata

# 下载依赖
COPY go.mod go.sum ./
RUN go mod download

# 编译应用
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -a -installsuffix cgo \
    -ldflags="-w -s" \
    -o yggdrasil-api-server main.go

# 运行阶段
FROM scratch
WORKDIR /app

# 添加 OCI 标签来连接仓库
LABEL org.opencontainers.image.source=https://github.com/NewNanCity/YaggdrasilGo
LABEL org.opencontainers.image.description="A high-performance Yggdrasil API server implementation in Go"
LABEL org.opencontainers.image.licenses=MIT
LABEL maintainer="NewNanCity Team"

# 复制二进制文件
COPY --from=builder /app/yggdrasil-api-server .

# 暴露端口
EXPOSE 8080

# 挂载点
VOLUME ["/app/conf", "/app/storage", "/app/data", "/app/logs"]

# 启动命令
ENTRYPOINT ["./yggdrasil-api-server"]
CMD ["-config", "conf/config.yml"]
