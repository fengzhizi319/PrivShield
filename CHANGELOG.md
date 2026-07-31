# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- 动态分类分级引擎基于 `OrderedDict` 的真实 LRU 评估缓存 (`ConfigurableRuleEngine._eval_cache`)，支持并发线程锁 (`_cache_lock`)、`PRIVACY_ENGINE_CACHE_MAX_SIZE` 环境变量配置及微秒级重复评估匹配
- 独立 Markdown 文档提取与规则 YAML 生成脚本 (`scripts/gen_yaml_from_doc.py`)，支持从 Markdown 规范直接抽取规则
- 大模型 YAML 关键词自动扩展脚本 (`scripts/expand_keywords_with_llm.py`)，支持在线 LLM API 与离线降级同义词表扩展
- `rule_schema.py` 核心架构与系统流转图 Docstring 文档
- 全面扩充 `general-pii` / `finance` / `medical` / `sc_health_db51` 领域规则包关键词词库
- Apache-2.0 LICENSE
- GitHub Actions CI: lint (ruff + mypy) / test (Python 3.10/3.11/3.12) / security (pip-audit) / docker build
- Ruff lint + format 配置 (pyproject.toml)
- mypy 类型检查配置
- pytest 覆盖率配置 (pytest-cov, fail_under=60)
- py.typed PEP 561 类型标记
- pre-commit hooks (ruff + ruff-format + trailing-whitespace)
- CHANGELOG.md (本文件)
- CONTRIBUTING.md 贡献指南
- SECURITY.md 安全漏洞报告流程
- Makefile 增强: lint/format/typecheck/cover/bench 目标
- pytest markers: `@pytest.mark.integration` / `@pytest.mark.slow`
- pytest-benchmark 性能基准测试

## [0.1.0] - 2024-06-01

### Added
- REST API (FastAPI, port 8079): masking / DP / K-anonymity / QoL / LocalDP / classification
- gRPC API (port 50051): 双协议统一 PrivacyService
- 差分隐私: count/sum/mean/histogram/vector_sum/vector_mean/adaptive_clip/groupby
- 本地差分隐私: binary/categorical 扰动与估计
- 数据脱敏: mobile/id_card/name/bank_card/email/address + HMAC hash
- K-匿名: 单记录/整表 Mondrian/DataFrame
- 查询混淆: 语义槽位替换 + 批量混淆
- 数据分类: Rule Engine → Small-NER → LLM 三层级联
- 隐私预算管理: 命名空间隔离 + SQLite 持久化
- 可观测性: Prometheus metrics + OTel tracing + 结构化日志
- 生产安全: API Key/mTLS 认证 + RBAC + Rate Limit + TLS
- 网关/负载均衡: REST + gRPC 反向代理
- 部署: Dockerfile 多阶段 + Helm Chart + Kustomize + docker-compose
- Arrow IPC 高效二进制端点
- 个性化隐私画像 (config/personalized-profiles.yaml)
