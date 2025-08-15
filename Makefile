# Yggdrasil API Server Makefile

# 变量定义
BINARY_NAME=yggdrasil-api-server
VERSION?=v1.0.0
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT=$(shell git rev-parse --short HEAD)
LDFLAGS=-ldflags "-w -s -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -X main.gitCommit=$(GIT_COMMIT)"

# Go 相关变量
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Docker 相关变量
DOCKER_IMAGE=yggdrasil-api-server
DOCKER_TAG?=latest

# 默认目标
.PHONY: all
all: clean deps test build

# 安装依赖
.PHONY: deps
deps:
	@echo "📦 下载依赖..."
	$(GOMOD) download
	$(GOMOD) tidy

# 运行测试
.PHONY: test
test:
	@echo "🧪 运行测试..."
	$(GOTEST) -v -race -coverprofile=coverage.out ./...

# 生成测试覆盖率报告
.PHONY: coverage
coverage: test
	@echo "📊 生成覆盖率报告..."
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "覆盖率报告已生成: coverage.html"

# 代码检查
.PHONY: lint
lint:
	@echo "🔍 运行代码检查..."
	$(GOCMD) vet ./...
	$(GOCMD) fmt ./...

# 构建二进制文件
.PHONY: build
build:
	@echo "🔨 构建二进制文件..."
	CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) main.go

# 构建多平台二进制文件
.PHONY: build-all
build-all:
	@echo "🔨 构建多平台二进制文件..."
	@mkdir -p build
	
	# Windows
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o build/$(BINARY_NAME)-windows-amd64.exe main.go
	GOOS=windows GOARCH=arm64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o build/$(BINARY_NAME)-windows-arm64.exe main.go
	
	# Linux
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o build/$(BINARY_NAME)-linux-amd64 main.go
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o build/$(BINARY_NAME)-linux-arm64 main.go
	
	# macOS
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o build/$(BINARY_NAME)-darwin-amd64 main.go
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o build/$(BINARY_NAME)-darwin-arm64 main.go
	
	# FreeBSD
	GOOS=freebsd GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o build/$(BINARY_NAME)-freebsd-amd64 main.go
	
	@echo "✅ 多平台构建完成，文件位于 build/ 目录"

# 清理构建文件
.PHONY: clean
clean:
	@echo "🧹 清理构建文件..."
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -rf build/
	rm -f coverage.out coverage.html

# 运行服务器
.PHONY: run
run: build
	@echo "🚀 启动服务器..."
	./$(BINARY_NAME) -config conf/config.yml

# 运行开发服务器
.PHONY: dev
dev:
	@echo "🔧 启动开发服务器..."
	$(GOCMD) run main.go -config conf/example.yml

# 生成密钥对
.PHONY: keys
keys:
	@echo "🔑 生成RSA密钥对..."
	@mkdir -p keys
	openssl genrsa -out keys/private.pem 2048
	openssl rsa -in keys/private.pem -pubout -out keys/public.pem
	@echo "✅ 密钥对已生成在 keys/ 目录"

# Docker 相关命令
.PHONY: docker-build
docker-build:
	@echo "🐳 构建Docker镜像..."
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

.PHONY: docker-run
docker-run:
	@echo "🐳 运行Docker容器..."
	docker run -p 8080:8080 -v $(PWD)/conf:/app/conf:ro -v $(PWD)/keys:/app/keys:ro $(DOCKER_IMAGE):$(DOCKER_TAG)

.PHONY: docker-compose-up
docker-compose-up:
	@echo "🐳 启动Docker Compose..."
	docker-compose up -d

.PHONY: docker-compose-down
docker-compose-down:
	@echo "🐳 停止Docker Compose..."
	docker-compose down

.PHONY: docker-compose-logs
docker-compose-logs:
	@echo "📋 查看Docker Compose日志..."
	docker-compose logs -f

# 部署相关命令
.PHONY: deploy-prepare
deploy-prepare:
	@echo "📋 准备部署环境..."
	@if [ ! -f .env ]; then cp .env.example .env; echo "请编辑 .env 文件配置环境变量"; fi
	@if [ ! -f conf/config.yml ]; then cp conf/example.yml conf/config.yml; echo "请编辑 conf/config.yml 文件"; fi
	@if [ ! -f keys/private.pem ]; then $(MAKE) keys; fi

.PHONY: deploy
deploy: deploy-prepare docker-build
	@echo "🚀 部署应用..."
	docker-compose up -d

# 健康检查
.PHONY: health
health:
	@echo "🏥 检查服务健康状态..."
	@curl -f http://localhost:8080/ > /dev/null 2>&1 && echo "✅ 服务正常运行" || echo "❌ 服务不可用"

# 查看日志
.PHONY: logs
logs:
	@echo "📋 查看应用日志..."
	docker-compose logs -f yggdrasil-api

# 备份数据
.PHONY: backup
backup:
	@echo "💾 备份数据..."
	@mkdir -p backups
	docker-compose exec mysql mysqldump -u root -p$(MYSQL_ROOT_PASSWORD) $(MYSQL_DATABASE) > backups/mysql-$(shell date +%Y%m%d_%H%M%S).sql
	@echo "✅ 数据库备份完成"

# 帮助信息
.PHONY: help
help:
	@echo "Yggdrasil API Server Makefile"
	@echo ""
	@echo "可用命令:"
	@echo "  deps              - 下载依赖"
	@echo "  test              - 运行测试"
	@echo "  coverage          - 生成测试覆盖率报告"
	@echo "  lint              - 代码检查和格式化"
	@echo "  build             - 构建二进制文件"
	@echo "  build-all         - 构建多平台二进制文件"
	@echo "  clean             - 清理构建文件"
	@echo "  run               - 运行服务器"
	@echo "  dev               - 运行开发服务器"
	@echo "  keys              - 生成RSA密钥对"
	@echo "  docker-build      - 构建Docker镜像"
	@echo "  docker-run        - 运行Docker容器"
	@echo "  docker-compose-up - 启动Docker Compose"
	@echo "  deploy-prepare    - 准备部署环境"
	@echo "  deploy            - 部署应用"
	@echo "  health            - 检查服务健康状态"
	@echo "  logs              - 查看应用日志"
	@echo "  backup            - 备份数据"
	@echo "  help              - 显示此帮助信息"
