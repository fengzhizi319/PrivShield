# 金融级双向 mTLS、加密防篡改与安全治理技术指南 / Security, mTLS, Cryptography & Tamper-Evident Audit Technical Guide

## 1. 技术简介 / Introduction

在数据流通、跨网络边车通信与隐私计算架构中，传输层安全与审计存证是企业级落地的关键保障：
1. **双向身份认证（Mutual TLS, mTLS）**：不仅服务端向客户端证明身份，客户端也必须向服务端提供由受信任 CA 签发的 X.509 证书；
2. **时序攻击免疫（Constant-Time Verification）**：利用 `hmac.compare_digest` 规避基于执行时间差的密钥前缀推测；
3. **不可篡改存证（Tamper-Evident Audit Chaining）**：基于 SHA-256 前序哈希链（Chained Hash）实现任何审计记录篡改或删减的毫秒级发现；
4. **路径穿越与符号链接逃逸防御（Path Traversal & Symlink Escape Guard）**：通过绝对路径规范化校验与目录白名单机制拦截恶意文件读取。

```text
                  客户端 / Client (BFF / Service Hub)
                                  │
                                  ▼
      ┌────────────────────────────────────────────────────────┐
      │  ★ 双向 mTLS 握手阶段 (Mutual TLS Handshake)             │
      │  - 服务端验证客户端 X.509 证书由私有 CA 签发              │
      │  - 客户端验证服务端证书及公钥固定 (Public Key Pinning)   │
      └───────────────────────────┬────────────────────────────┘
                                  │ [TLS 建立]
                                  ▼
      ┌────────────────────────────────────────────────────────┐
      │  ★ gRPC Auth Interceptor & WhitelistManager (auth.py)  │
      │  - 提取客户端证书 Subject Common Name (CN)              │
      │  - 匹配 YAML 白名单及 Scope 权限 (Hot-Reload 支持)      │
      │  - Fail-Closed 原则：白名单为空或未匹配一律拒绝 (401/403)│
      └───────────────────────────┬────────────────────────────┘
                                  │ [鉴权通过]
                                  ▼
      ┌────────────────────────────────────────────────────────┐
      │  ★ 隐私治理业务处理 (Privacy Processing)                 │
      └───────────────────────────┬────────────────────────────┘
                                  │
                                  ▼
      ┌────────────────────────────────────────────────────────┐
      │  ★ 不可篡改哈希链审计存证 (services/audit-log)           │
      │  - Current_Hash = SHA256(Prev_Hash + Record_Data)      │
      │  - HMAC 签名校验与防篡改验证 API                        │
      └────────────────────────────────────────────────────────┘
```

---

## 2. 在本项目中的用法 / Usage in This Project

### 2.1 gRPC mTLS 客户端证书身份提取与白名单热加载 / gRPC mTLS & Whitelist Hot-Reload

文件 / File：[`engine/security/auth.py`](engine/security/auth.py#L80-L130) & [`engine/security/whitelist.py`](engine/security/whitelist.py)（Python 端）；[`pkg/tlsutil/whitelist.go`](pkg/tlsutil/whitelist.go)（Go 端，mtime 轮询热重载）

#### (1) Common Name 提取与作用域授权

```python
def _authenticate_mtls(
    settings: SecuritySettings, auth_context: dict[str, Any]
) -> Identity | None:
    """从已验证的 gRPC mTLS 客户端证书中提取 Identity。
    
    安全原则：
      仅通过 CA 校验只能证明客户端持有某张证书，不能证明其有权访问本服务。
      因此必须结合 Common Name 白名单与作用域（Scopes）进行严格匹配。
    """
    if not settings.internal_mtls_enabled:
        return None

    # 从 gRPC SSL 认证上下文中提取 X.509 证书的主题通用名 (CN)
    cn_props = auth_context.get("x509_common_name", [])
    if not cn_props:
        return None
    cn = cn_props[0].decode("utf-8") if isinstance(cn_props[0], bytes) else str(cn_props[0])

    # 委托 WhitelistManager 校验白名单并获取该 CN 分配的权限
    mgr = get_whitelist_manager()
    scopes = mgr.get_scopes_for_cn(cn)
    if scopes is None:
        logger.warning(f"mTLS client cert CN '{cn}' not in allowed whitelist. Denied.")
        return None  # Fail-Closed

    return Identity(identity_type="internal_mtls", name=cn, scopes=scopes)
```

#### (2) 白名单配置热加载 (Hot-Reloading)

`WhitelistManager` 会监听 YAML 文件的 `mtime` 修改时间。当运维更新配置时，服务无需重启即可自动感知最新的 CN 权限表。

---

### 2.2 防范时序攻击的常量时间比对 / Constant-Time API Key Verification

文件 / File：[`engine/security/auth.py`](engine/security/auth.py#L50-L80)

传统的 `dict.get()` 或普通字符串 `==` 运算会在遇到首个不匹配字符时立即短路退出，攻击者可通过高精度测量网络 RTT 逐字节推断出有效的 API Key。`PrivShield` 强制采用全量常量时间迭代：

```python
def _constant_time_lookup(keys: dict[str, Any], token: str) -> Any | None:
    """常量时间查找 token，杜绝短路比较造成的时序侧信道泄露。"""
    token_bytes = token.encode("utf-8")
    matched = None
    # 强制遍历所有已配置的 Key，绝不提前 break
    for key, value in keys.items():
        if hmac.compare_digest(key.encode("utf-8"), token_bytes):
            matched = value
    return matched
```

---

### 2.3 基于 SHA-256 前序哈希链的不可篡改审计日志 / Tamper-Evident Audit Chaining

文件 / File：[`services/audit-log/internal/storage/`](services/audit-log/internal/storage/)

在中台审计服务中，为了防止特权运维人员或攻击者直接修改数据库抹除操作痕迹，所有审计日志采用区块链式的前序哈希指针链接：

$$H_i = \text{SHA-256}(H_{i-1} \parallel \text{Timestamp}_i \parallel \text{Operator}_i \parallel \text{Action}_i \parallel \text{PayloadHash}_i)$$

```go
type AuditLogEntry struct {
    ID          string    `json:"id"`
    Timestamp   int64     `json:"timestamp"`
    PrevHash    string    `json:"prev_hash"`    // 前序记录哈希指针
    CurrentHash string    `json:"current_hash"` // 当前记录整体 SHA-256 哈希
    Operator    string    `json:"operator"`
    Action      string    `json:"action"`
    DetailJSON  string    `json:"detail_json"`
}

func (s *Storage) AppendAuditLog(entry *AuditLogEntry) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    // 1. 获取上一条记录的哈希作为前序锚点
    entry.PrevHash = s.latestHash
    
    // 2. 计算当前哈希
    h := sha256.New()
    h.Write([]byte(fmt.Sprintf("%s|%d|%s|%s|%s", 
        entry.PrevHash, entry.Timestamp, entry.Operator, entry.Action, entry.DetailJSON)))
    entry.CurrentHash = hex.EncodeToString(h.Sum(nil))

    // 3. 写入存储并更新最新哈希锚点
    s.latestHash = entry.CurrentHash
    return s.db.Save(entry).Error
}
```

任何对历史记录中某一行字段的篡改，都会导致后续整条链条的哈希校验断裂，中台提供 `/v1/audit/verify` 接口可一键验证整库数据完整性。

---

### 2.4 路径穿越与符号链接逃逸安全防护 / Path Traversal & Symlink Escape Guard

文件 / File：[`engine/dynclassification/image_redaction.py`](engine/dynclassification/image_redaction.py)

在处理医学图像与 DICOM 文件时，攻击者可能传入 `../../../../etc/shadow` 或构造恶意软链接（Symlink）。`PrivShield` 实现了严密的路径归一化与白名单隔离：

```python
def validate_safe_file_path(path: str | Path) -> Path:
    """严格路径校验：规范化解析后必须位于允许的白名单目录之内。"""
    resolved_path = Path(path).resolve()  # 展开所有 .. 并解析符号链接

    allowed_dirs = [
        Path(p).resolve() for p in os.environ.get(
            "PRIVACY_IMAGE_ALLOWED_DIRS", 
            f"{os.getcwd()}{os.pathsep}{tempfile.gettempdir()}"
        ).split(os.pathsep) if p.strip()
    ]

    # 检查目标路径是否完全属于白名单目录树内
    is_safe = any(
        resolved_path == d or d in resolved_path.parents 
        for d in allowed_dirs
    )
    if not is_safe:
        raise PermissionError(f"Access to file path '{path}' is forbidden by path security policy")
    return resolved_path
```

**四重安全防护层详解**：

```text
攻击者输入: "../../etc/passwd" 或 "/tmp/symlink -> /etc/shadow"
    │
    ▼
┌─────────────────────────────────────────────────────┐
│  Layer 1: Path.resolve() 规范化解析                │
│  - 展开所有 ".." 父目录引用                         │
│  - 解析所有符号链接为真实物理路径                │
│  - 结果: /etc/passwd 或 /etc/shadow                   │
└───────────────────────┬─────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────┐
│  Layer 2: 白名单目录比对                             │
│  - PRIVACY_IMAGE_ALLOWED_DIRS 环境变量配置           │
│  - 默认: cwd + 系统临时目录                             │
│  - 每个目录也经过 resolve() 规范化                    │
└───────────────────────┬─────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────┐
│  Layer 3: 严格父路径检查                             │
│  - resolved_path == allowed_dir 完全匹配             │
│  - allowed_dir in resolved_path.parents 父链匹配   │
│  - 不允许任何目录外的路径                              │
└───────────────────────┬─────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────┐
│  Layer 4: Fail-Closed 异常回退                       │
│  - 不匹配 → PermissionError 立即抛出                │
│  - 损坏图像 → [IMAGE-REDACTION-FAILED] 标记       │
│  - 绝不泄露原始敏感文件                              │
└─────────────────────────────────────────────────────┘
```

---

## 3. mTLS CN 白名单热加载系统深度解析 / Whitelist Hot-Reload System

文件 / File：[`engine/security/whitelist.py`](engine/security/whitelist.py)

### 3.1 YAML 配置结构与 Pydantic 校验

PrivShield 的 mTLS 白名单采用 YAML 配置文件管理，支持每个 CN 独立的作用域控制（最小权限原则）：

```yaml
# config/mtls-whitelist.yaml
version: "1.0"

# 未列在名单中但通过 CA 验证的 CN 的默认权限
# 空列表 = fail-closed（默认拒绝所有未列名 CN）
default_scopes: []

entries:
  - cn: "service-hub-client"
    scopes: ["mask:*", "dp:query", "classify:*"]
    description: "数联服务调度中枢 - 脱敏与分类权限"
    enabled: true

  - cn: "audit-log-service"
    scopes: ["audit:write", "audit:verify"]
    description: "审计存证服务 - 仅写入与验证"
    enabled: true

  - cn: "bff-go-admin"
    scopes: ["*"]  # 全权限（管理员控制台）
    description: "Go BFF 运维控制台 - 全权限"
    enabled: true

  - cn: "deprecated-service"
    scopes: ["*"]
    description: "已废弃服务 - 禁用"
    enabled: false  # 禁用但保留记录，便于审计
```

### 3.2 两阶段提交与线程安全重载

`WhitelistManager` 采用**两阶段提交**（Two-Phase Commit）模式实现安全重载：先在临时缓冲区中解析新配置，成功后再原子交换缓存，失败则保留旧配置不中断服务。

```python
class WhitelistManager:
    def _load(self) -> None:
        """加载或重载白名单配置（两阶段提交）。"""
        # 阶段 1：解析到临时缓冲区
        content = path.read_text(encoding="utf-8")
        raw = yaml.safe_load(content)
        config = WhitelistConfig.model_validate(raw)  # Pydantic 校验

        new_cache: dict[str, CNEntry] = {}
        for entry in config.entries:
            if entry.enabled:
                new_cache[entry.cn] = entry  # 仅缓存启用的条目

        # 阶段 2：原子交换缓存（在锁内完成）
        with self._lock:
            self._cache = new_cache
            self._default_scopes = config.default_scopes
            self._last_mtime = path.stat().st_mtime
            self._load_error = None  # 清除旧错误
```

**为什么用两阶段提交？** 如果 YAML 文件被运维编辑到一半（语法错误、字段缺失），直接解析并覆盖缓存会导致服务崩溃或所有认证失败。两阶段提交确保「要么全成功，要么保持现状」。

### 3.3 基于 mtime 的请求驱动热重载

PrivShield 不使用后台线程监听文件变化，而是采用**请求驱动的被动检查**：每次白名单查询时先检查文件 mtime 是否变化，变化则触发重载。

```python
def _check_reload(self) -> None:
    """每次查询前轻量检查文件是否变化。"""
    current_mtime = path.stat().st_mtime
    if current_mtime > self._last_mtime:
        self._load()  # 触发重载

def get_scopes(self, cn: str) -> list[str] | None:
    self._check_reload()  # 每次查询前检查
    with self._lock:
        entry = self._cache.get(cn)
        return entry.scopes if entry else None
```

> **设计取舍**：后台线程监听（如 `watchdog`）虽然更实时，但引入了额外依赖和线程管理复杂性。mtime 检查的开销极低（一次 `stat()` 系统调用，纳秒级），且完全避免了线程同步问题。

---

## 4. API Key 认证与作用域授权 / API Key Authentication & Scope Authorization

文件 / File：[`engine/security/auth.py`](engine/security/auth.py)

### 4.1 API Key 配置格式

API Key 通过环境变量配置，格式为逗号分隔的 `key=scope1,scope2` 对：

```bash
export PRIVACY_AUTH_API_KEYS="sk-hub-abc123=mask:*,dp:query,sk-admin-xyz789=*"
```

### 4.2 认证流程与 Fail-Closed 策略

```text
客户端请求 ──► 携带 Authorization: Bearer sk-hub-abc123
    │
    ▼
┌────────────────────────────────────────────────┐
│  1. 检查 auth_enabled 环境变量                 │
│     未启用 → 跳过认证，直接放行                  │
└────────────────────┬───────────────────────────┘
                     │
                     ▼
┌────────────────────────────────────────────────┐
│  2. 提取 Bearer Token                          │
│     缺失/格式错误 → 401 Unauthorized             │
└────────────────────┬───────────────────────────┘
                     │
                     ▼
┌────────────────────────────────────────────────┐
│  3. 常量时间比对（遍历所有 Key）              │
│     不匹配 → 401 Unauthorized                    │
└────────────────────┬───────────────────────────┘
                     │
                     ▼
┌────────────────────────────────────────────────┐
│  4. Scope 权限检查                             │
│     请求的 API 所需 scope 不在分配列表中       │
│     → 403 Forbidden                              │
└────────────────────┬───────────────────────────┘
                     │
                     ▼
              放行请求到业务层
```

### 4.3 Scope 通配符匹配

Scope 支持简单的通配符匹配：`mask:*` 匹配所有以 `mask:` 开头的权限请求。

```python
def _check_scope(granted_scopes: list[str], required_scope: str) -> bool:
    """检查已授权的 scope 列表是否包含所需的 scope。"""
    for scope in granted_scopes:
        if scope == "*":
            return True  # 全权限通配
        if scope == required_scope:
            return True  # 精确匹配
        if scope.endswith("*") and required_scope.startswith(scope[:-1]):
            return True  # 前缀通配匹配
    return False
```

---

## 5. 速率限制器实现 / Rate Limiter Implementation

文件 / File：[`engine/security/ratelimit.py`](engine/security/ratelimit.py)

PrivShield 实现了基于**滑动窗口**的速率限制器，保护服务免受突发流量冲击：

```bash
# 启用速率限制
export PRIVACY_RATE_LIMIT_ENABLED=true
# 每个 IP 每分钟最多 100 次请求
export PRIVACY_RATE_LIMIT_REQUESTS_PER_MINUTE=100
```

速率限制器使用内存中的令牌桶算法，按客户端 IP 或认证身份进行粒度控制。超过限制时返回 `429 Too Many Requests`，并在响应头中携带重试信息：

```http
HTTP/1.1 429 Too Many Requests
Retry-After: 30
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1693000000
```

---

## 6. TLS 证书生成与管理 / TLS Certificate Generation

### 6.1 使用 OpenSSL 生成完整的 PKI 体系

```bash
# 1. 生成 CA 私钥和自签名证书
openssl genrsa -out ca.key 4096
openssl req -new -x509 -key ca.key -sha256 \
  -subj "/C=CN/ST=Shanghai/O=PrivShield/CN=PrivShield-CA" \
  -days 3650 -out ca.crt

# 2. 生成服务端证书
openssl genrsa -out server.key 2048
openssl req -new -key server.key \
  -subj "/C=CN/ST=Shanghai/O=PrivShield/CN=privshield-agent" \
  -out server.csr
# 用 CA 签发
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key \
  -CAcreateserial -out server.crt -days 365 -sha256

# 3. 生成客户端证书（mTLS）
openssl genrsa -out client.key 2048
openssl req -new -key client.key \
  -subj "/C=CN/ST=Shanghai/O=PrivShield/CN=service-hub-client" \
  -out client.csr
openssl x509 -req -in client.csr -CA ca.crt -CAkey ca.key \
  -CAcreateserial -out client.crt -days 365 -sha256
```

> **学习要点**：CN（Common Name）字段是 PrivShield 白名单系统的核心。服务端通过提取客户端证书的 CN 值（如 `service-hub-client`）来匹配白名单中的作用域配置。

### 6.2 环境变量配置一览

| 变量 | 用途 | 示例 |
|---|---|---|
| `PRIVACY_TLS_ENABLED` | 启用 TLS | `true` |
| `PRIVACY_TLS_CERT_FILE` | 服务端证书路径 | `/etc/ssl/server.crt` |
| `PRIVACY_TLS_KEY_FILE` | 服务端私钥路径 | `/etc/ssl/server.key` |
| `PRIVACY_AUTH_ENABLED` | 启用 API Key 认证 | `true` |
| `PRIVACY_AUTH_API_KEYS` | API Key 列表 | `sk-abc=mask:*,sk-xyz=*` |
| `PRIVACY_AUTH_INTERNAL_MTLS_ENABLED` | 启用 gRPC mTLS | `true` |
| `PRIVACY_AUTH_MTLS_WHITELIST_FILE` | CN 白名单 YAML | `/config/mtls-whitelist.yaml` |
| `PRIVACY_AUTH_MTLS_ALLOWED_CNS` | 静态 CN 列表（回退） | `client-a,client-b` |
| `PRIVACY_RATE_LIMIT_ENABLED` | 启用速率限制 | `true` |

---

## 7. 审计日志不可篡改性验证 / Audit Log Integrity Verification

### 7.1 哈希链验证原理

每条审计记录的 `current_hash` 依赖前一条记录的 `prev_hash`，形成链式依赖：

```text
Record 1: prev_hash="0000...", current_hash=SHA256("0000...|ts1|op1|action1|data1")
Record 2: prev_hash=Record1.current_hash, current_hash=SHA256(prev_hash|ts2|op2|action2|data2")
Record 3: prev_hash=Record2.current_hash, current_hash=SHA256(prev_hash|ts3|op3|action3|data3")
```

如果攻击者修改了 Record 2 的任意字段，Record 2 的 current_hash 会变化，导致 Record 3 的 prev_hash 不匹配，整条链断裂。

### 7.2 验证 API 使用

```bash
# 验证整库完整性
curl -X POST http://audit-log:8084/v1/audit/verify \
  -H "Content-Type: application/json"

# 响应示例
{
  "valid": true,
  "total_entries": 15234,
  "verified_chain": true,
  "first_hash": "a1b2c3...",
  "latest_hash": "f8e7d6..."
}
```

### 7.3 HMAC 签名校验

除了哈希链，PrivShield 还支持基于 HMAC-SHA256 的审计记录签名，用于检测特权运维直接修改数据库的场景：

```go
// HMAC 签名计算
func computeHMAC(entry *AuditLogEntry, secret []byte) string {
    mac := hmac.New(sha256.New, secret)
    mac.Write([]byte(fmt.Sprintf("%s|%d|%s|%s",
        entry.PrevHash, entry.Timestamp, entry.Operator, entry.DetailJSON)))
    return hex.EncodeToString(mac.Sum(nil))
}
```

---

## 8. 安全加固检查清单 / Security Hardening Checklist

| 检查项 | 风险等级 | 配置方式 |
|---|---|---|
| 启用 TLS | 🔴 高 | `PRIVACY_TLS_ENABLED=true` |
| 启用 API Key 认证 | 🔴 高 | `PRIVACY_AUTH_ENABLED=true` |
| 启用 gRPC mTLS | 🔴 高 | `PRIVACY_AUTH_INTERNAL_MTLS_ENABLED=true` |
| 配置 CN 白名单 | 🟡 中 | `PRIVACY_AUTH_MTLS_WHITELIST_FILE` |
| 启用速率限制 | 🟡 中 | `PRIVACY_RATE_LIMIT_ENABLED=true` |
| 配置图片目录白名单 | 🟡 中 | `PRIVACY_IMAGE_ALLOWED_DIRS` |
| 审计日志完整性校验 | 🟢 低 | `/v1/audit/verify` 定期调用 |
| 最小权限 Scope 分配 | 🟡 中 | 每个 CN 仅分配必需 scope |

---

## 9. 运维实战命令 / Operations Commands

```bash
# 生成自签名 CA + 服务端 + 客户端证书
mkdir -p config/certs
openssl genrsa -out config/certs/ca.key 4096
openssl req -new -x509 -key config/certs/ca.key -sha256 \
  -subj "/CN=PrivShield-CA" -days 3650 -out config/certs/ca.crt

# 启动 Agent（TLS + mTLS + 速率限制）
export PRIVACY_TLS_ENABLED=true
export PRIVACY_TLS_CERT_FILE=config/certs/server.crt
export PRIVACY_TLS_KEY_FILE=config/certs/server.key
export PRIVACY_AUTH_ENABLED=true
export PRIVACY_AUTH_INTERNAL_MTLS_ENABLED=true
export PRIVACY_AUTH_MTLS_WHITELIST_FILE=config/mtls-whitelist.yaml
export PRIVACY_RATE_LIMIT_ENABLED=true
python -m engine.server

# 验证审计日志完整性
curl -X POST http://localhost:8084/v1/audit/verify

# 查看当前白名单状态（通过管理 API）
curl http://localhost:8079/v1/ops/diagnostics \
  -H "Authorization: Bearer $ADMIN_KEY"
```

---

## 10. 常见安全攻击与 PrivShield 防御矩阵 / Attack Vectors & Defense Matrix

| 攻击类型 | 攻击描述 | PrivShield 防御机制 | 代码位置 |
|---|---|---|---|
| **时序侧信道** | 通过精确测量 RTT 逐字节推断 API Key | `hmac.compare_digest` 常量时间比对 | `auth.py` |
| **路径穿越** | `../../etc/shadow` 读取敏感文件 | `Path.resolve()` + 白名单目录比对 | `image_redaction.py` |
| **符号链接逃逸** | 创建 symlink 指向白名单外的敏感文件 | `resolve()` 展开 symlink 后重新校验 | `image_redaction.py` |
| **解压炸弹** | 超大图片反序列化耗尽内存 | `Image.MAX_IMAGE_PIXELS = 25M` | `image_redaction.py` |
| **中间人攻击** | 截获/篡改客户端与服务端通信 | TLS 1.2+ 加密 + CA 证书校验 | `server.py` |
| **身份伪冒** | 未授权服务假冒合法客户端身份 | mTLS 双向证书验证 + CN 白名单 | `whitelist.py` |
| **重放攻击** | 截获合法请求后重复发送 | 时间戳校验 + 审计哈希链不可篡改 | `audit-log` |
| **配置篡改** | 特权运维修改白名单 YAML 添加后门 CN | 两阶段提交 + 文件完整性校验 | `whitelist.py` |
| **Slowloris** | 大量慢连接耗尽服务器资源 | Uvicorn `limit_concurrency` + `timeout_keep_alive` | `server.py` |
| **大包 DDoS** | 超大 payload 耗尽网关内存 | 64 MiB 请求体上限 | `http_proxy.py` |

---

## 11. 扩展阅读 / Further Reading

1. **HIPAA Safe Harbor**：https://www.hhs.gov/hipaa/for-professionals/security/laws-regulations
2. **NIST SP 800-52**：TLS 实施指南
3. **OWASP Top 10**：https://owasp.org/www-project-top-ten/
4. **时序攻击**：Paul Kocher, "Timing Attacks on Implementations of Diffie-Hellman, RSA, DSS, and Other Systems" (1996)
5. **区块链哈希链**：Bitcoin Whitepaper Section 2 - "Timestamp Server"
