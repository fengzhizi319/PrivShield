# 数盾 PrivShield

Welcome to the **数盾 PrivShield** documentation.

`PrivShield` is a high-performance Go cloud-native sidecar and governance platform implementing the **「三层四柱五御六类」医疗数据安全与隐私治理架构** (3-Funnel, 4-Pillar, 5-Protection, 6-Category Architecture), exposing 44 privacy primitives (masking, differential privacy, K-anonymity, query obfuscation) and a 3-layer data classification funnel over REST and gRPC.

---


## Capabilities

| Capability | Status | Description |
|---|---|---|
| Masking | ✅ Ready | Field-name-aware masking for common PII |
| Differential Privacy | ✅ Ready | Laplace/Gaussian count/sum/mean with budget accounting |
| K-anonymity | ✅ Ready | Per-record heuristic & dataset-level (Mondrian) |
| Query Obfuscation | ✅ Ready | Dummy query injection |
| Classification | ✅ Ready | Rule engine → Small-NER → local VLM/LLM |
| Service Hub | ✅ Ready | 6-stage data pipeline orchestration (:8082) |
| Datasource Mgr | ✅ Ready | Asset metadata & sensitive feature discovery (:8083) |
| Audit Log | ✅ Ready | Tamper-proof SHA-256 blockchain-style log (:8084) |
| Console & BFF | ✅ Ready | React SPA + Go gRPC BFF (:5173/:8081) |
| Gateway / Load Balancer | ✅ Ready | REST + gRPC reverse proxy with health checks |
| TLS / Auth / Rate Limit | ✅ Ready | Opt-in via environment variables |
| Observability | ✅ Ready | Structured logs + Prometheus `/metrics` + tracing |
| K8s / Helm Deployment | ✅ Ready | Helm chart + Kustomize + Docker Compose |

## Quick Navigation

- **Privacy Primitives** — Masking, DP, K-Anonymity, Query Obfuscation
- **Data Classification** — 3-layer funnel: Rule Engine → NER → LLM
- **Enterprise Microservices** — Service Hub, Datasource Manager, Audit Log
- **Console & BFF Gateway** — React Web UI, Go gRPC BFF
- **Infrastructure** — Gateway, load balancer, health checks
- **Production** — Security, observability, deployment
- **Audit & Security** — [Full Project Audit & Remediation Report (2026)](audit_reports/2026_full_project_audit_report.md)
- **Appendix** — Personalized profiles, improvement suggestions

---

!!! tip "Getting Started"
    Head over to the [Quick Start](quickstart.md) guide to install and run the agent locally.