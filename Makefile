# PrivShield 常用命令
#
# 这个 Makefile 的目标是把“开发、测试、打包、部署、文档”入口统一到一处，
# 方便贡献者快速找到该跑什么命令：
# - 变更主包时优先跑 `test` / `lint` / `typecheck`
# - 只改控制台时优先跑 `test-console` / `lint-console` / `typecheck-console`
# - 构建部署产物时使用 `docker-*`、`helm-*`、`docs-*`

.PHONY: help test test-cov test-unit test-console test-go lint lint-console format format-console typecheck typecheck-console check cover cover-html bench \
        helm-lint helm-template docker-core docker-ml clean docs-serve docs-build docs-clean

VERSION ?= 0.1.0
HELM_DIR = deploy/helm/PrivShield

help:
	@echo "Available targets:"
	@echo ""
	@echo "Testing:"
	@echo "  test           - 运行 pytest 测试套件（核心 Agent）"
	@echo "  test-unit      - 仅运行 Agent 单元测试（排除 integration/slow）"
	@echo "  test-console   - 运行 console/bff-py 单元与冒烟测试"
	@echo "  test-go        - 运行 Go 全量测试（共享库 + 微服务群 + BFF-Go）"
	@echo "  test-services  - 运行三大中台微服务单元测试"
	@echo "  test-cov       - 运行测试 + 覆盖率报告"
	@echo "  cover          - 同 test-cov"
	@echo "  cover-html     - 生成 HTML 覆盖率报告"
	@echo "  bench          - 运行性能基准测试"
	@echo ""
	@echo "Quality:"
	@echo "  lint           - ruff 静态检查（主项目 + 控制台后端）"
	@echo "  lint-console   - ruff 静态检查（仅控制台后端）"
	@echo "  format         - ruff 自动格式化（主项目 + 控制台后端）"
	@echo "  format-console - ruff 自动格式化（仅控制台后端）"
	@echo "  typecheck      - mypy 类型检查（主项目 + 控制台后端）"
	@echo "  typecheck-console - mypy 类型检查（仅控制台后端）"
	@echo "  check          - lint + typecheck 一键检查"
	@echo ""
	@echo "Deployment:"
	@echo "  helm-lint      - helm lint 检查 chart"
	@echo "  helm-template  - helm template 渲染 chart"
	@echo "  docker-core    - 构建 core 镜像"
	@echo "  docker-ml      - 构建 ml 镜像"
	@echo ""
	@echo "Docs:"
	@echo "  docs-serve     - 启动 MkDocs 开发服务器"
	@echo "  docs-build     - 构建文档站点"
	@echo "  docs-clean     - 清理文档构建产物"
	@echo ""
	@echo "Other:"
	@echo "  clean          - 清理构建产物"

# ── Quality ──────────────────────────────────────────────────

lint:
	ruff check PrivShield/ tests/ console/bff-py/

lint-console:
	ruff check console/bff-py/

format:
	ruff format PrivShield/ tests/ console/bff-py/
	ruff check --fix PrivShield/ tests/ console/bff-py/

format-console:
	ruff format console/bff-py/
	ruff check --fix console/bff-py/

typecheck:
	mypy

typecheck-console:
	mypy console/bff-py

check: lint-console typecheck-console

# ── Testing ──────────────────────────────────────────────────

# `test` 是主项目默认测试入口；`test-unit` 则排除更慢或依赖外部条件的用例，
# 便于本地高频反馈。
test:
	pytest tests/ -q --tb=short

test-unit:
	pytest tests/ -q --tb=short -m "not integration and not slow"

# 控制台测试分成 Python 后端和 smoke test 两步，确保 UI 代理链路都能跑通。
test-console:
	cd console/bff-py && pytest tests/ -v
	cd console/bff-py && python smoke_test.py

# Go 微服务与 BFF 测试
test-go:
	go test ./pkg/... ./services/service-hub/... ./services/datasource-mgr/... ./services/audit-log/... ./console/bff-go/...

test-services:
	go test ./services/service-hub/... ./services/datasource-mgr/... ./services/audit-log/...

test-cov:
	pytest tests/ -q --tb=short \
		--cov=PrivShield \
		--cov-report=term-missing \
		-m "not integration and not slow"

cover: test-cov

cover-html:
	pytest tests/ -q --tb=short \
		--cov=PrivShield \
		--cov-report=html \
		-m "not integration and not slow"
	@echo "Open htmlcov/index.html"

bench:
	pytest tests/ -q --benchmark-only --benchmark-columns=mean,stddev,rounds

# ── Deployment ───────────────────────────────────────────────

# Helm 相关目标只负责模板和 lint，不直接安装集群；这样可以在 CI 和本地做预检查。
helm-lint:
	helm lint $(HELM_DIR)

helm-template:
	helm template test $(HELM_DIR)

docker-core:
	# core 镜像仅包含运行主服务所需的基础依赖，体积更小、启动更快。
	docker build --target core -t privshield:$(VERSION) .

docker-ml:
	# ml 镜像额外包含 torch / transformers / onnxruntime 等重依赖，
	# 适合需要 NER / VLM / LLM 功能的环境。
	docker build --target ml -t privshield:$(VERSION)-ml .

# ── Docs ─────────────────────────────────────────────────────

# MkDocs 文档生成分成“开发预览”和“静态构建”两种入口，便于本地校对与 CI 发布复用。
docs-serve:
	@echo "Starting MkDocs dev server..."
	mkdocs serve

docs-build:
	@echo "Building docs site..."
	mkdocs build

docs-clean:
	rm -rf site/

# ── Console Launchers ────────────────────────────────────────

dev-go:
	./console/scripts/dev-start-go.sh

dev-python:
	./console/scripts/dev-start.sh

dev-all:
	./console/scripts/dev-start-all.sh

dev-go-mtls:
	./console/scripts/dev-start-go-mtls.sh

prod-go:
	./console/scripts/prod-start-go.sh

prod-python:
	./console/scripts/prod-start.sh

prod-all:
	./console/scripts/prod-start-all.sh

prod-go-mtls:
	./console/scripts/prod-start-go-mtls.sh

stop:
	./console/scripts/dev-stop.sh

prod-stop:
	./console/scripts/prod-stop.sh

prod-compose:
	./scripts/prod/deploy-docker-compose.sh

prod-compose-stop:
	./scripts/prod/stop-docker-compose.sh

prod-check:
	./scripts/prod/prod_health_check.sh

prod-backup:
	./scripts/prod/backup_privacy_budget.sh

# ── Other ────────────────────────────────────────────────────

proto-gen:
	python -m grpc_tools.protoc -I proto --python_out=PrivShield --grpc_python_out=PrivShield proto/privacy.proto

clean:
	rm -rf .pytest_cache __pycache__ .bin htmlcov .coverage coverage.xml
	find . -type d -name __pycache__ -exec rm -rf {} + 2>/dev/null || true
