# Privacy Local Agent — Agent Guide

> AI coding agent guide for the `privacy-local-agent` project. Read this before modifying code.

`privacy-local-agent` is a Python sidecar that exposes privacy primitives (masking, differential privacy, K-anonymity, query obfuscation) and a 3-layer data classification funnel over REST and gRPC. It is designed for local/Sidecar deployment and is currently at POC/MVP maturity.

---

## 1. Project Overview

| Capability | Status | Notes |
|---|---|---|
| Masking | ✅ Ready | Field-name-aware masking for common PII |
| Differential Privacy | ✅ Ready | Laplace count/sum/mean with budget accounting |
| K-anonymity | ✅ Ready | Per-record heuristic & dataset-level generalization |
| Query Obfuscation | ✅ Ready | Dummy query injection |
| Classification | ✅ Ready | Rule engine → Small-NER → local LLM |
| Gateway / Load Balancer | ✅ Ready | REST + gRPC reverse proxy with health checks |
| TLS / Auth / Rate Limit | ✅ Ready | Opt-in via environment variables |
| Observability | ✅ Ready | Structured logs + Prometheus `/metrics` + optional tracing |
| K8s / Helm Deployment | ✅ Ready | `deploy/helm/` + `deploy/k8s/` + `deploy/docker-compose/` |
| Dataset-level K-anonymity | ✅ Ready | Implemented via Mondrian algorithm |
| DP Gaussian / clipping | ✅ Ready | Gaussian mechanism & clipping bounds supported |
| ML dependency split | ✅ Ready | Single Dockerfile with `--target core|ml` |

## 2. Technology Stack

- **Python 3.10+**
- **FastAPI** + **Uvicorn** for REST
- **gRPC** (`grpcio`) for RPC
- **Pydantic v2** for models
- **PyYAML** for profile configuration
- **ONNX Runtime / ModelScope** for Small-NER (optional, lazy-loaded)
- **PyTorch + Transformers + Qwen3.5** for LLM layer (optional, lazy-loaded)

Core dependencies are pinned in `pyproject.toml`. Heavy ML dependencies are **not** pinned as runtime deps; they are lazy-loaded and degraded gracefully if absent.

## 3. Repository Layout

```text
privacy-local-agent/
├── privacy_local_agent/           # Main package
│   ├── main.py                    # FastAPI REST entrypoint
│   ├── grpc_server.py             # gRPC servicer
│   ├── server.py                  # REST + gRPC combined launcher
│   ├── service.py                 # PrivacyService orchestrator
│   ├── schemas.py                 # REST request models (Pydantic)
│   ├── routers/                   # REST sub-routers (mask/dp/kano/qol/dynclassification/...)
│   ├── security/                  # TLS / auth / rate-limit
│   ├── observability/             # Logging / metrics / tracing
│   ├── privacy/                   # Privacy primitives
│   │   ├── masking.py
│   │   ├── dp.py
│   │   ├── kano.py
│   │   ├── qol.py
│   │   ├── budget.py
│   │   ├── profile.py
│   │   ├── download_model.py
│   │   └── download_ner_model.py
│   ├── dynclassification/         # Dynamic classification (3-layer funnel: Rule → NER → LLM)
│   │   ├── funnel.py              # ClassificationFunnel orchestrator + Safety Floor
│   │   ├── engine.py              # ConfigurableRuleEngine (YAML rules)
│   │   ├── models.py              # SecurityTag / ConfidencePolicy / DomainTaxonomy
│   │   ├── rule_schema.py         # RuleDef / RuleProfile schema
│   │   ├── composite.py           # Composite rules
│   │   ├── service.py             # DynClassificationService
│   │   ├── ner_adapter.py / ner_engines.py       # Small-NER (lazy-load)
│   │   ├── llm_adapter.py / llm_engines.py       # Local LLM/VLM (lazy-load)
│   │   ├── mlx_ner_engine.py / mlx_llm_engine.py # MLX backends
│   │   └── image_redaction.py     # Image redaction
│   └── gateway/                   # Optional gateway/load balancer
│       ├── server.py
│       ├── balancer.py
│       ├── http_proxy.py
│       └── grpc_proxy.py
├── proto/privacy.proto            # gRPC service definition
├── tests/                         # pytest suite
├── mkdocs.yml                       # MkDocs + Material configuration

├── config/                        # Profile & runtime configs
├── rules/                         # Preset classification rules & standards
├── data/                          # Sample datasets & test data
├── scripts/                       # Utility scripts
│   ├── dev/                       # Services, health check & test runners
│   ├── data/                      # Data generators & rule exporters
│   ├── env/                       # Environment installers & acceleration
│   └── models/                    # Model downloaders & converters
├── console/                       # 测试控制台（React + FastAPI / Go 代理）
│   ├── backend/                   # FastAPI 代理，转发请求到 agent REST
│   ├── backend-go/                # Go gRPC 代理，可直接提供 Console UI
│   └── web/                       # React 单页测试控制台
├── Makefile
├── pyproject.toml
├── requirements.txt               # Local dev/test deps
├── requirements-core.txt          # Core image runtime deps
├── requirements-ml.txt            # ML image extra deps
└── Dockerfile
```

## 4. Build & Test Commands

```bash
cd /home/charles/code/sfwork/privacy-local-agent

# Install in editable mode
pip install -e .

# Or install dev extras
pip install -e ".[dev]"

# Run tests
PYTHONPATH=. pytest tests -q

# Run a specific test file
PYTHONPATH=. pytest tests/api/test_rest.py -v

# Benchmark privacy primitives
PYTHONPATH=. python tests/benchmark_primitives.py

# Download models (optional, required for LLM/NER layers)
python -m privacy_local_agent.privacy.download_model
python -m privacy_local_agent.privacy.download_ner_model
```

## 5. Running Locally

### REST + gRPC in one process

```bash
python -m privacy_local_agent.server
```

Defaults:
- REST: `http://127.0.0.1:8079`
- gRPC: `127.0.0.1:50051`

### REST only

```bash
python -m privacy_local_agent.main
```

### gRPC only

```bash
python -m privacy_local_agent.grpc_server
```

### Gateway + worker pool

```bash
python -m privacy_local_agent.gateway.server
```

## 6. Configuration

Key environment variables:

| Variable | Default | Purpose |
|---|---|---|
| `PRIVACY_ENV_PROFILE` | `vllm` | Active LLM profile (`vllm`/`qwen3`/`mlx`/`openai`, loads `config/env/<profile>.env`) |
| `PRIVACY_PROFILE` | — | Path to YAML parameter profile |
| `PRIVACY_NAMESPACE` | `default` | Budget namespace |
| `PRIVACY_REST_HOST` | `127.0.0.1` | REST host |
| `PRIVACY_REST_PORT` | `8079` | REST port |
| `PRIVACY_GRPC_HOST` | `127.0.0.1` | gRPC host |
| `PRIVACY_GRPC_PORT` | `50051` | gRPC port |
| `PRIVACY_BUDGET_DB` | — | SQLite DB path for distributed budget |
| `PRIVACY_BUDGET_WINDOW_SECONDS` | — | Time window for automatic privacy budget reset |
| `PRIVACY_LOG_LEVEL` | `INFO` | Logging level |
| `PRIVACY_LOG_FORMAT` | `text` | `text` or `json` |
| `PRIVACY_SERVICE_NAME` | `privacy-local-agent` | Service name in logs/traces |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | — | Optional OpenTelemetry OTLP endpoint |
| `PRIVACY_TLS_ENABLED` | `false` | Enable TLS on REST/gRPC |
| `PRIVACY_AUTH_ENABLED` | `false` | Enable API key auth |
| `PRIVACY_RATE_LIMIT_ENABLED` | `false` | Enable rate limiting |
| `PRIVACY_WARMUP_LLM` | `false` | Async warmup local LLM on REST startup |
| `PRIVACY_LLM_MAX_CONCURRENCY` | `1` | Process-wide LLM inference concurrency cap (semaphore, prevents OOM) |
| `PRIVACY_LLM_SEMAPHORE_WAIT_SECONDS` | `30` | Max seconds a request waits for the LLM inference slot before degrading |
| `PRIVACY_LLM_MIN_FREE_MEM_MB` | `512` | Skip LLM layer when available memory falls below this threshold (MB) |
| `PRIVACY_LLM_CONFIDENCE_THRESHOLD` | `0.75` | Minimum confidence threshold for Layer-3 arbitration |
| `PRIVACY_LLM_ENABLE_ARBITRATION` | `true` | Enable Layer-3 LLM arbitration on low confidence or uncertainty |
| `PRIVACY_IMAGE_ALLOWED_DIRS` | cwd + 系统临时目录 | 图片打码允许读取的目录白名单（os.pathsep 分隔）；路径 resolve 后必须位于白名单内，拒绝目录穿越与 symlink 逃逸 |

## 7. Code Conventions

- Follow **PEP 8**.
- Keep acronyms in PascalCase uppercase for domain terms (e.g. `DP`, `LDP`, `QOL`, `KAnonymity`, `NER`, `LLM`).
- Exception classes must end with `Error` or `Exception` (e.g. `PrivacyBudgetExhaustedError`).
- Use **type hints** on public functions.
- Use **Pydantic v2** models for request/response schemas.
- Keep primitives stateless; state lives in `PrivacyService` / `BudgetAccountant`.
- Lazy-load heavy ML models; never import `torch`/`transformers` at module top level unless unavoidable.
- Add tests for new primitives and classification rules.
- Prefer `pathlib.Path` over string paths.


## 8. Adding a New Privacy Primitive

1. Implement the algorithm in `privacy_local_agent/privacy/<primitive>.py`.
2. Add a Pydantic request/response model in `privacy_local_agent/schemas.py` or a new models file.
3. Expose it in:
   - `privacy_local_agent/service.py` (business logic)
   - `privacy_local_agent/routers/<primitive>.py` (REST sub-router, mounted by `main.py`)
   - `privacy_local_agent/grpc_server.py` (gRPC method)
4. Add tests in `tests/api/test_rest.py` and/or `tests/test_<primitive>.py`.
5. Update `proto/privacy.proto` and regenerate stubs if adding gRPC:
   ```bash
   python -m grpc_tools.protoc -I proto --python_out=privacy_local_agent --grpc_python_out=privacy_local_agent proto/privacy.proto
   ```

## 9. Adding a Classification Rule / Composite Rule / Taxonomy

分类规则已迁移至 `dynclassification` 模块并全面 YAML 化：领域规则在 `rules/domains/*.yaml`，
分类体系在 `rules/taxonomies/*.yaml`；引擎为 `ConfigurableRuleEngine`
（`dynclassification/engine.py`），规则 schema 见 `dynclassification/rule_schema.py`。
旧 `privacy/classification/` 子包（含 vectorized/async/review/template 机制）已删除，勿再引用。

### 9.1 Adding a Layer-1 Rule

1. 在对应的 `rules/domains/*.yaml` 中新增 `RuleDef`（`id`/`level`/`category`/`matchers`），
   降级规则加入 `downgrade_rules` 节（`DowngradeRuleDef`）。
2. 匹配算子定义在 `dynclassification/operators.py`；新算子经 `operator_registry.py` 注册。
3. 在 `tests/dynclassification/` 添加测试（参考 `test_funnel.py`、`test_downgrade_override.py`）。

### 9.2 Adding a Composite Rule

1. 在 `dynclassification/composite.py` 中添加规则。
2. 在 `tests/dynclassification/` 添加测试。

### 9.3 Adding a Taxonomy / Standard

1. 新增 `rules/taxonomies/<domain>.yaml`：`levels`（按 rank 升序）、`default_level`，
   并**显式补齐 `confidence_policy` 节**（字段与默认值见
   `docs/dynclassification/three_layer_funnel_design.md` §2.3）。
2. 新增对应领域规则 `rules/domains/<domain>.yaml`。
3. 在 `tests/dynclassification/test_standards_switching.py` 扩展体系切换用例。

## 10. Testing Guidelines

- All changes must include tests.
- Mock heavy ML models in unit tests (see `tests/dynclassification/test_ner_adapter.py` and `tests/dynclassification/test_llm_adapter.py`).
- Gateway tests use `httpx` / `grpc.aio` channels; run them with the gateway server fixture.
- Budget tests cover both in-memory and SQLite backends.

## 11. Deployment Notes

### Docker

```bash
# core 镜像（默认推荐）
docker build --target core -t privacy-local-agent:0.1.0 .

# ml 镜像（含 torch/transformers/onnxruntime）
docker build --target ml -t privacy-local-agent:0.1.0-ml .

docker run -p 8079:8079 -p 50051:50051 privacy-local-agent:0.1.0
```

### Helm

```bash
helm install pla ./deploy/helm/privacy-local-agent

# 生产模式（需自管 TLS/API Key Secret）
helm install pla ./deploy/helm/privacy-local-agent \
  -f ./deploy/helm/privacy-local-agent/values-production.yaml \
  --set security.tls.existingSecret=your-tls-secret \
  --set security.auth.apiKeysSecret=your-apikeys-secret
```

### 原生 K8s

```bash
kubectl apply -k ./deploy/k8s/
```

### Docker Compose

```bash
cd deploy/docker-compose && docker-compose up -d
```

### Production Gaps

- KMS integration and automated key rotation are not yet implemented.
- Load/chaos/memory-leak test suites are not yet implemented.

Address these before any hardened production deployment.

## 12. Security Considerations

- Never commit model weights or large `.models/` files to git.
- Do not expose the gRPC/REST ports to untrusted networks without TLS.
- HMAC salt should be provided by the caller; consider KMS integration for production.
- Privacy budget in memory mode is not consistent across multiple instances; use `PRIVACY_BUDGET_DB` for multi-instance deployments.
- Validate and sanitize all inputs; Pydantic models are the first line of defense.

## 13. Key Documentation

| Document | Path | Purpose |
|---|---|---|
| README | `README.md` | Quick start and examples |
| Classification design | `docs/classification/design.md` | 3-layer funnel architecture |
| Classification ops | `docs/classification/ops.md` | Deployment and YAML profile |
| Classification PRD | `docs/classification/prd.md` | Requirements |
| LLM PRD | `docs/classification_llm/prd.md` | Multimodal LLM gateway requirements |
| Gateway design | `docs/gateway_balancer/design.md` | Gateway and load balancer |
| Production security PRD | `docs/production_security/prd.md` | TLS/auth/rate-limit requirements |
| Production security design | `docs/production_security/design.md` | TLS/auth/rate-limit architecture |
| Production security ops | `docs/production_security/ops.md` | Deployment and cert quick reference |
| Observability PRD | `docs/production_observability/prd.md` | Logging/metrics/tracing requirements |
| Observability design | `docs/production_observability/design.md` | Architecture and metric design |
| Observability ops | `docs/production_observability/ops.md` | Configuration and Grafana examples |
| Masking design | `docs/masking/design.md` | Field-name-aware masking architecture |
| Masking ops | `docs/masking/ops.md` | Masking deployment and tuning |
| Masking testing | `docs/masking/testing.md` | Masking test checklist |
| Query obfuscation design | `docs/qol/design.md` | Query obfuscation architecture |
| Query obfuscation ops | `docs/qol/ops.md` | Query obfuscation monitoring |
| Query obfuscation testing | `docs/qol/testing.md` | Query obfuscation test checklist |
| Deployment PRD | `docs/deployment/prd.md` | K8s/Helm/Docker Compose requirements |
| Deployment design | `docs/deployment/design.md` | Chart structure and parameters |
| Deployment ops | `docs/deployment/ops.md` | Install, upgrade and troubleshooting |

## 14. Quick Reference

| Goal | Command |
|---|---|
| Install | `pip install -e .` |
| Test | `PYTHONPATH=. pytest tests -q` |
| Helm lint | `make helm-lint` |
| Helm template | `make helm-template` |
| Build core image | `make docker-core` |
| Build ml image | `make docker-ml` |
| Run Dev Console (Go + Vite HMR) | `./console/scripts/dev-start-go.sh` |
| Run Dev Console (Python + Vite HMR) | `./console/scripts/dev-start.sh` |
| Run Dev Console (Dual Backend + Vite) | `./console/scripts/dev-start-all.sh` |
| Run Dev Console (Go mTLS + Vite) | `./console/scripts/dev-start-go-mtls.sh` |
| Run Prod Console (Go + Static) | `./console/scripts/prod-start-go.sh` |
| Run Prod Console (Python + Static) | `./console/scripts/prod-start.sh` |
| Run Prod Console (Dual Backend + Static) | `./console/scripts/prod-start-all.sh` |
| Stop Dev Console | `./console/scripts/dev-stop.sh` |
| Stop Prod Console | `./console/scripts/prod-stop.sh` |
| Run REST + gRPC | `python -m privacy_local_agent.server` |
| Run test console backend | `cd console/backend && ./run.sh` |
| Build test console frontend | `cd console/web && corepack pnpm install && corepack pnpm build` |
| Run gateway | `python -m privacy_local_agent.gateway.server` |
| Regenerate gRPC stubs | `python -m grpc_tools.protoc -I proto --python_out=privacy_local_agent --grpc_python_out=privacy_local_agent proto/privacy.proto` |
| Build docs | `make docs-build` |
| Serve docs | `make docs-serve` |
| Download LLM | `python -m privacy_local_agent.privacy.download_model` |
| Download NER | `python -m privacy_local_agent.privacy.download_ner_model` |
