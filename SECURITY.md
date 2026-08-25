# Security Policy

The **数联天下 · 数盾 (`PrivShield`)** team takes the security of our data privacy computing, dynamic classification, and security governance platform seriously. This document describes our supported versions, vulnerability reporting guidelines, security scope, and built-in security defenses.

---

## 1. Supported Versions

We provide security updates and bug fixes for the following versions:

| Version | Status | Supported | Notes |
|---|---|---|---|
| **1.8.x** | Current Release | ✅ Yes | Active development and full security support |
| **1.x** | Major Version | ✅ Yes | Security patches and critical vulnerability fixes |
| **< 1.0.0** | Legacy / Beta | ❌ No | Please upgrade to the latest stable 1.8.x release |

---

## 2. Reporting a Vulnerability

If you believe you have discovered a security vulnerability in PrivShield, please report it responsibly. **Please do NOT open public GitHub issues or discussions for security vulnerabilities.**

### How to Report

1. **GitHub Security Advisory (Recommended)**:
   - Navigate to the repository: **[Security → Advisories → Report a vulnerability](https://github.com/fengzhizi319/PrivShield/security/advisories/new)**
   - Fill out the private vulnerability report form.
2. **Email Disclosure**:
   - Send full details and reproduction steps to the repository maintainer via GitHub Security Advisories or maintainer contact.

### What to Include in Your Report

To help us triage and resolve the issue quickly, please provide:
- **Component**: Python Engine, Go Microservices (`service-hub`, `datasource-mgr`, `audit-log`), Go BFF, or Web UI.
- **Vulnerability Description**: Detailed explanation of the attack vector and security impact.
- **Steps to Reproduce**: Minimal reproducible example, curl commands, or script payload.
- **Environment**: OS version, Python/Go runtime versions, active configuration flags, deployment mode (native / Docker / K8s).
- **Suggested Fix / Remediation** (if available).

### Response SLA & Timeline

- **Initial Acknowledgment**: Within **48 hours**
- **Triage & Severity Assessment**: Within **7 days**
- **Fix & Advisory Release Timeline**:
  - **Critical Severity** (e.g., Unauthenticated RCE, Full Auth Bypass, Privacy Budget Arbitrary Drain): **7 days**
  - **High Severity** (e.g., Sensitive Data Leakage, DP Epsilon Compromise, Audit Tampering): **14 days**
  - **Medium / Low Severity** (e.g., DoS via resource exhaustion, Scope Escalation): **30 days**

---

## 3. Vulnerability Scope

### In Scope

- **Authentication & Authorization**:
  - API Key authentication bypass or forgery
  - mTLS client certificate verification bypass or CN spoofing
  - RBAC Scope privilege escalation between internal and external tenants
  - Constant-time comparison flaws and timing side-channel attacks
- **Privacy Primitives & Data Protection**:
  - Differential Privacy (DP/LDP) budget manipulation, race conditions, or noise suppression flaws
  - K-Anonymity / Mondrian quasi-identifier generalization leakage
  - PII masking algorithm bypass or reversible unmasking vulnerabilities
  - Query Obfuscation (QOL) semantic leakage
- **Audit & Cryptographic Integrity**:
  - 8-Factor SHA-256 audit hash collision, forgery, or verification bypass
  - Append-Only SQLite storage tampering or record deletion bypass
- **Microservices & Orchestration Pipeline**:
  - Service Hub 6-stage pipeline security bypass
  - Datasource Manager CSV path traversal, symlink escape, or LFI vulnerabilities
  - Image redaction file path traversal or arbitrary directory reading
- **Infrastructure & Execution Safety**:
  - Insecure deserialization (YAML `SafeLoader` bypass, `pickle` injection, unverified PyTorch weights)
  - Command / Code injection via API inputs
  - Application-level Denial of Service (Slowloris bypass, large payload OOM, uncontrolled goroutine/thread spawn)

### Out of Scope

- Self-XSS with no impact on other administrative users
- Missing informational HTTP security headers with no demonstrated exploitability
- Vulnerabilities requiring root/physical compromise of the host system
- Third-party dependency vulnerabilities with no reachable exploit path in PrivShield
- Social engineering or phishing targeting repository maintainers

---

## 4. Built-in Security Architecture & Defenses

PrivShield implements defense-in-depth security across the Python calculation engine and Go enterprise microservices:

### 4.1 Transport Layer Security (TLS 1.3 & Zero-Trust mTLS)
- **REST & gRPC Encryption**: Enforces TLS 1.3 for external REST (`:8079`) and high-performance gRPC (`:50051`).
- **Inter-service mTLS & Public Key Pinning**: Microservices (`service-hub`, `datasource-mgr`, `audit-log`, `console/bff-go`) utilize [`pkg/tlsutil`](pkg/tlsutil) for mutual TLS authentication with SPKI Public Key Pinning.
- **Client CN Whitelist & Hot Reloading**: gRPC mTLS enforces client certificate CN whitelisting (`PRIVACY_AUTH_MTLS_WHITELIST_FILE`) with per-CN scope mapping and dynamic runtime hot-reloading.

### 4.2 Authentication & Timing Attack Prevention
- **Constant-Time Verification**: All API Keys, Bearer Tokens, and HMAC digests are compared using constant-time algorithms (`hmac.compare_digest` in Python, `crypto/subtle.ConstantTimeCompare` in Go) to prevent timing side-channel attacks.
- **Dual-Tier Identity Isolation**: Distinct internal service keys (`scopes: ["*"]`) vs external tenant keys (least privilege scopes like `privacy:mask`, `classification:read`).

### 4.3 Multi-Layered Anti-DDoS & Traffic Shield
- **Slowloris Mitigation**: Server-level hard timeouts (`ReadHeaderTimeout: 5s`, `ReadTimeout: 30s`, `MaxHeaderBytes: 1MB`).
- **Large Payload DoS Interception**: Strict body size limits (`MaxBodySize: 32MB/64MB`) using `http.MaxBytesReader` returning `413 Payload Too Large`.
- **IP Token-Bucket & Sliding Window Rate Limiting**: Built-in `IPRateLimiter` with automatic GC for idle IP buckets, plus endpoint-level rate limit overrides.
- **Concurrency Circuit Breaking**: Semaphore-based concurrency capping (`MaxConcurrent`) to prevent thread/goroutine exhaustion, fast-failing with `503 Service Unavailable`.

### 4.4 Sandbox Isolation & Safe Deserialization
- **Path Traversal & LFI Prevention**:
  - Image redaction verifies `PRIVACY_IMAGE_ALLOWED_DIRS`, canonicalizes paths via `Path.resolve()`, and rejects symlink escapes.
  - Datasource manager strictly enforces `.csv` whitelist, base name sanitization (`filepath.Base`), and a 50,000-row loading ceiling.
- **Safe Serialization**:
  - YAML deserialization strictly uses `yaml.safe_load()`.
  - Machine learning models enforce `torch.load(..., weights_only=True)`.

### 4.5 8-Factor Immutable Audit Trail
- **Cryptographic Hash Chaining**: Every desensitization event is signed with an 8-factor SHA-256 integrity hash:
  $$\text{IntegrityHash} = \text{SHA256}(\text{logID} \parallel \text{timestamp} \parallel \text{algorithm} \parallel \text{inputHash} \parallel \text{outputHash} \parallel \text{user} \parallel \text{securityLevel} \parallel \text{paramsJSON})$$
- **Append-Only Store & Database Integrity**: Storage APIs expose no update/delete methods; SQLite databases run `PRAGMA integrity_check` on boot.

---

## 5. Production Security Configuration Guide

When deploying PrivShield to production, configure the following security environment variables:

### 5.1 Python Engine Security Settings

```bash
# --- TLS / HTTPS / gRPCs ---
PRIVACY_TLS_ENABLED=true
PRIVACY_TLS_CERT_FILE=/etc/privshield/certs/server.crt
PRIVACY_TLS_KEY_FILE=/etc/privshield/certs/server.key
PRIVACY_TLS_CA_FILE=/etc/privshield/certs/ca.crt
PRIVACY_TLS_CLIENT_AUTH=require

# --- Authentication & Zero-Trust mTLS ---
PRIVACY_AUTH_ENABLED=true
PRIVACY_AUTH_INTERNAL_MTLS_ENABLED=true
PRIVACY_AUTH_MTLS_WHITELIST_FILE=/etc/privshield/config/mtls-whitelist.yaml
PRIVACY_AUTH_INTERNAL_KEYS_JSON='{
  "sk-internal-svc": {"name": "service-hub", "scopes": ["*"]}
}'
PRIVACY_AUTH_EXTERNAL_KEYS_JSON='{
  "sk-external-app": {"name": "client-app", "scopes": ["privacy:mask", "classification:read"]}
}'

# --- Rate Limiting ---
PRIVACY_RATE_LIMIT_ENABLED=true
PRIVACY_RATE_LIMIT_DEFAULT_RPS=100
PRIVACY_RATE_LIMIT_DEFAULT_BURST=200
PRIVACY_RATE_LIMIT_PER_ENDPOINT_JSON='{
  "/v1/privacy/dp/count": {"rps": 10, "burst": 20},
  "/v1/privacy/mask_record": {"rps": 50, "burst": 100}
}'

# --- Sandbox & File Access ---
PRIVACY_IMAGE_ALLOWED_DIRS=/data/medical_images:/tmp/privshield

# --- Privacy Budget Persistence ---
PRIVACY_BUDGET_DB=/data/budget.db
```

### 5.2 Go Microservices Security Settings

```bash
# --- Service Hub (:8082, gRPC :50052) ---
SERVICE_HUB_TLS_ENABLED=true
SERVICE_HUB_TLS_CERT_FILE=/etc/privshield/certs/service-hub.crt
SERVICE_HUB_TLS_KEY_FILE=/etc/privshield/certs/service-hub.key
SERVICE_HUB_TLS_CA_FILE=/etc/privshield/certs/ca.crt
SERVICE_HUB_API_KEY=sk-internal-servicehub-secret

# --- Datasource Manager (:8083, gRPC :50053) ---
DATASOURCE_MGR_TLS_ENABLED=true
DATASOURCE_MGR_TLS_CERT_FILE=/etc/privshield/certs/datasource-mgr.crt
DATASOURCE_MGR_TLS_KEY_FILE=/etc/privshield/certs/datasource-mgr.key
DATASOURCE_MGR_TLS_CA_FILE=/etc/privshield/certs/ca.crt
DATASOURCE_MGR_API_KEY=sk-internal-datasourcemgr-secret

# --- Audit Log (:8084, gRPC :50054) ---
AUDIT_LOG_TLS_ENABLED=true
AUDIT_LOG_TLS_CERT_FILE=/etc/privshield/certs/audit-log.crt
AUDIT_LOG_TLS_KEY_FILE=/etc/privshield/certs/audit-log.key
AUDIT_LOG_TLS_CA_FILE=/etc/privshield/certs/ca.crt
AUDIT_LOG_API_KEY=sk-internal-auditlog-secret
```

---

## 6. Security Documentation Index

For in-depth architecture designs, threat models, and operational checklists:

- 📘 [Production Security Architecture & Design (生产安全设计)](docs/production_security/design.md)
- 📋 [Security Requirements & Common Vulnerability Mitigations (安全规范与漏洞防范)](docs/production_security/security_requirements.md)
- 🛠️ [Production Security Operations & Cert Guide (生产安全运维手册)](docs/production_security/ops.md)
- 🔒 [Service Hub Reliability & Security (调度中枢可靠性与安全)](services/service-hub/docs/reliability.md)
- 🛡️ [Datasource Manager Security & Reliability (数据源安全与可靠性)](services/datasource-mgr/docs/reliability.md)
- 📜 [Audit Log Cryptographic Integrity & Reliability (审计存证可靠性与哈希完整性)](services/audit-log/docs/reliability.md)
- 🔍 [2026 Full Project Audit Report (全项目安全审计报告)](docs/audit_reports/2026_full_project_audit_report.md)

