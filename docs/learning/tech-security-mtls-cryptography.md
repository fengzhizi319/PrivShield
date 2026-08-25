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

文件 / File：[`engine/security/auth.py`](file:///home/charles/code/PrivShield/engine/security/auth.py#L80-L130) & [`engine/security/whitelist.py`](file:///home/charles/code/PrivShield/engine/security/whitelist.py)

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

文件 / File：[`engine/security/auth.py`](file:///home/charles/code/PrivShield/engine/security/auth.py#L50-L80)

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

文件 / File：[`services/audit-log/internal/storage/`](file:///home/charles/code/PrivShield/services/audit-log/internal/storage/)

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

文件 / File：[`engine/dynclassification/image_redaction.py`](file:///home/charles/code/PrivShield/engine/dynclassification/image_redaction.py)

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
