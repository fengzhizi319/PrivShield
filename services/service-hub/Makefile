# ============================================================================
# Service Hub (数据服务调度中枢) Build Automation Makefile
# 数据服务调度中枢自动化构建与测试 Makefile
#
# 常用指令：
#   make build      - 整理依赖并编译二进制文件至 bin/service-hub
#   make run / dev  - 编译并直接运行本地服务
#   make test       - 运行全量单元测试与集成测试
#   make test-unit  - 运行轻量单元测试（跳过耗时集成测试 -short）
#   make lint       - 执行 Go 代码格式化与静态检查
#   make clean      - 清理编译产物目录 bin/
# ============================================================================

.PHONY: build run dev test test-unit lint clean

# 整理依赖并编译二进制文件
build:
	go mod tidy
	go build -o bin/service-hub ./cmd/server

# 本地编译并直接拉起服务
run:
	go mod tidy
	go build -o bin/service-hub ./cmd/server
	./bin/service-hub

# 开发模式快捷别名
dev: run

# 运行全量测试套件
test:
	go test ./...

# 运行短时单元测试
test-unit:
	go test -short ./...

# 代码格式化与 vet 静态语法分析
lint:
	go fmt ./...
	go vet ./...

# 清理构建临时产物
clean:
	rm -rf bin

