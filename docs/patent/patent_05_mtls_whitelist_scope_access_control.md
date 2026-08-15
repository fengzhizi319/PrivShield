# 专利草案 05：面向隐私计算服务的 mTLS 白名单与 per-CN 细粒度 scope 访问控制方法

## 一、创新点提炼（与已有专利的差异定位）

> 已有中文专利 4 已提及网关支持 TLS、API Key 鉴权、按证书 CN 白名单授予权限范围与速率限制，但未深入展开 mTLS 认证架构、per-CN 独立 scope 控制、YAML 白名单热重载、统一身份模型、fail-closed 默认策略与认证拒绝可观测机制。本专利草案聚焦于**隐私计算服务中 mTLS 传输层与应用层两层校验**、**细粒度 per-CN scope 访问控制**、**请求驱动的白名单热重载**与**统一 REST/gRPC 身份模型**，与已有专利形成互补。

1. **传输层 + 应用层两层校验模型**
   - 传统 mTLS 仅校验客户端证书是否由受信 CA 签发，本方法在此基础上增加 CN（Common Name）白名单校验，只有白名单中的 CN 才被授予授权身份。
   - 解决证书泄露或内部证书横向越权问题：即使某客户端证书被第三方获取，只要其 CN 不在白名单中，服务端依然拒绝访问。

2. **per-CN 独立 scope 控制**
   - 每个白名单条目可配置独立的权限 scope 列表（如 `privacy:dp`、`classification:read`、`health:read` 等），实现最小权限原则。
   - 向后兼容静态 CN 列表模式，未配置 YAML 文件时所有 CN 默认获得 `["*"]` 全权限。

3. **YAML 白名单配置热重载**
   - 白名单通过 YAML 文件配置，支持请求驱动的 mtime 检查与两阶段提交热重载，修改后无需重启服务即可生效。
   - 加载失败时保留旧配置，确保服务可用性。

4. **REST 与 gRPC 统一身份模型**
   - 设计 `Identity` 数据类（service_type、name、scopes），统一表示内部/外部服务身份。
   - REST 通过 FastAPI dependency 提取 Bearer Token 或 mTLS 信息；gRPC 通过 unary interceptor 从 auth_context 提取 CN 或 metadata token。
   - REST 路径与 gRPC 方法映射到同一套权限标识（如 `privacy:mask`、`privacy:dp`、`classification:read`）。

5. **Fail-Closed 安全设计**
   - `auth_internal_mtls_enabled` 默认 false；`auth_mtls_allowed_cns` 默认空列表；`tls_client_auth` 默认 none。
   - 未显式配置时，即使证书通过 CA 校验也不授予身份，拒绝所有 mTLS 证书，确保安全默认行为。

6. **认证拒绝可观测**
   - 认证拒绝事件记录到结构化日志，并更新 `privacy_auth_denials_total` Prometheus Counter 与 `privacy_auth_duration_seconds` Histogram，支持运维监控与告警。

---

## 二、专利原文

### 发明名称

一种面向隐私计算服务的 mTLS 白名单与 per-CN 细粒度 scope 访问控制方法及系统

### 技术领域

本发明涉及网络安全与访问控制技术领域，尤其涉及隐私计算服务中的双向 TLS 认证、客户端证书白名单、基于 scope 的接口级鉴权与速率限制。

### 背景技术

隐私计算服务通常处理高敏感医疗、金融数据，需要严格的身份认证与访问控制。现有技术存在以下问题：
1. 仅依赖 CA 签名校验的 mTLS 无法区分不同客户端证书的授权范围，一旦证书泄露即可横向越权。
2. 传统 API Key 鉴权依赖 Bearer Token，存在泄露后难以快速撤销、难以细粒度控制权限的问题。
3. REST 与 gRPC 两套协议通常使用独立的认证与鉴权实现，增加了配置复杂度和安全漏洞风险。
4. 证书白名单修改后通常需要重启服务，影响可用性。
5. 缺乏对认证拒绝事件的统一监控与审计。

### 发明内容

本发明提供一种面向隐私计算服务的 mTLS 白名单与 per-CN 细粒度 scope 访问控制方法及系统，通过传输层 CA 校验与应用层 CN 白名单两层校验，结合 per-CN scope 控制、热重载、统一身份模型与 fail-closed 默认策略，实现对隐私计算服务的精细化访问控制。

一种面向隐私计算服务的 mTLS 白名单与 per-CN 细粒度 scope 访问控制方法，包括：
- 在服务端启用 TLS 与可选 mTLS，要求客户端出示 X.509 证书；
- 在 TLS 握手阶段校验客户端证书是否由受信 CA 签发；
- 在应用层从 TLS 认证上下文提取客户端证书的 Common Name（CN）；
- 查询 CN 白名单，仅当 CN 存在于白名单中且条目处于启用状态时，授予对应身份与 scope 列表；
- 根据请求的目标 REST 路径或 gRPC 方法，校验该身份是否具有对应的权限 scope；
- 若通过权限校验，执行业务逻辑；否则拒绝访问并记录审计日志与指标；
- 其中，白名单通过 YAML 文件配置，支持 per-CN 独立 scope 控制与请求驱动的热重载。

进一步地，所述身份模型包括：服务类型（内部/外部）、名称（CN 或 API Key 名称）、scope 列表；scope 包含 `*` 表示全权限，或包含 `privacy:mask`、`privacy:dp`、`privacy:kano`、`privacy:qol`、`classification:read`、`health:read` 等接口级权限。

进一步地，所述权限校验包括：为每个 REST 路径与 gRPC 方法预先配置权限标识；调用 `identity.has_permission(permission)` 判断身份的 scope 列表是否包含该权限或通配符。

进一步地，所述 CN 白名单包括：
- YAML 配置文件，包含版本、默认 scope、条目列表；每个条目包含 CN、scope 列表、描述、启用状态；
- 静态 CN 列表，作为向后兼容模式，所有 CN 默认获得 `*` 全权限；
- 优先级：YAML 配置文件 > 静态 CN 列表；两者均未配置时白名单为空，拒绝所有证书。

进一步地，所述热重载包括：每次请求到达时检查配置文件 mtime；若 mtime 大于上次加载时间，解析 YAML 到临时缓存，校验成功后原子替换当前配置；加载失败时保留旧配置并记录错误日志。

进一步地，还包括速率限制：基于身份名称与目标路径/方法构造限流键，采用移动窗口策略，单副本使用内存存储，多副本使用 Redis 存储；超速时返回 HTTP 429 或 gRPC RESOURCE_EXHAUSTED。

进一步地，还包括健康检查端点豁免：`/health` 路径与 `Health` 方法默认无需认证、不限速，避免 K8s 探针误判。

进一步地，还包括 fail-closed 默认配置：mTLS 认证默认关闭；CA 校验通过默认不授予身份；CN 白名单默认空列表；TLS 客户端认证默认不请求客户端证书。

进一步地，还包括认证拒绝可观测：认证/鉴权/限速拒绝事件记录结构化日志，并更新 `privacy_auth_denials_total` Counter 与 `privacy_auth_duration_seconds` Histogram。

### 具体实施方式

**证书链结构**
- Root CA：`PrivShield-test-ca`。
- Server Certificate：CN=localhost，SAN 覆盖 localhost/127.0.0.1，EKU 含 serverAuth。
- Client Certificate：CN=privacy-console-go-client，EKU 含 clientAuth。

**gRPC 认证流程**
1. 客户端发起 gRPC 连接，服务端出示服务端证书，客户端校验服务端证书。
2. 客户端出示客户端证书，服务端校验客户端证书 CA 信任链。
3. TLS 握手完成，建立加密通道。
4. 客户端发送 RPC 请求，gRPC 拦截器读取 `auth_context`。
5. 提取 `x509_common_name`，确认 `transport_security_type` 为 ssl。
6. 查询白名单管理器，若 CN 存在且启用，返回 `Identity("internal", cn, entry.scopes)`。
7. 校验目标 gRPC 方法的权限 scope，通过则执行，否则返回 `PERMISSION_DENIED`。
8. 若 mTLS 认证失败，回退到 API Key 认证；若均失败，返回 `UNAUTHENTICATED`。

**REST 认证流程**
1. 客户端发起 HTTPS 请求，服务端根据 `tls_client_auth` 模式决定是否请求客户端证书。
2. TLS 握手完成，请求到达 FastAPI 路由。
3. `get_current_identity` 依赖提取 `Authorization: Bearer <token>`。
4. 若启用 mTLS 客户端证书认证，可进一步从 ASGI scope 读取客户端证书 CN 并优先使用。
5. 校验目标 REST 路径的权限 scope，通过则执行，否则返回 403 Forbidden。

**YAML 白名单示例**
```yaml
version: "1.0"
default_scopes: []
entries:
  - cn: "privacy-console-go-client"
    scopes: ["*"]
    description: "Go console backend - full access"
    enabled: true
  - cn: "prometheus-monitor"
    scopes: ["health:read"]
    description: "Prometheus scraper - health check only"
    enabled: true
  - cn: "data-analytics"
    scopes: ["privacy:dp", "classification:read"]
    description: "Analytics pipeline - DP and classification"
    enabled: true
```

**热重载实施例**
```python
def get_entry(self, cn: str) -> CNEntry | None:
    self._check_reload()
    return self._entries.get(cn)

def _check_reload(self):
    if self._config_path is None:
        return
    current_mtime = self._config_path.stat().st_mtime
    if current_mtime > self._last_mtime:
        self._load()  # 两阶段提交
```

**Fail-Closed 默认配置**
- `auth_internal_mtls_enabled=false`：即使 CA 校验通过也不授予身份。
- `auth_mtls_allowed_cns=[]`：启用 mTLS 认证时拒绝所有证书。
- `tls_client_auth="none"`：不请求客户端证书。

### 权利要求书

1. 一种面向隐私计算服务的 mTLS 白名单与 per-CN 细粒度 scope 访问控制方法，其特征在于，包括：
   服务端启用 TLS 与可选的 mTLS，在 TLS 握手阶段校验客户端证书是否由受信 CA 签发；
   在应用层从 TLS 认证上下文中提取客户端证书的 Common Name（CN）；
   查询 CN 白名单，仅当 CN 存在于白名单且条目启用时，授予与该 CN 对应的身份及 scope 列表；
   根据目标 REST 路径或 gRPC 方法对应的权限标识，校验该身份是否具有对应 scope；
   校验通过则执行业务逻辑，否则拒绝访问并记录审计日志；
   其中，白名单通过 YAML 文件配置，支持 per-CN 独立 scope 控制与请求驱动的热重载。

2. 根据权利要求 1 所述的方法，其特征在于，所述身份模型包括服务类型、名称与 scope 列表；scope 列表包含通配符或接口级权限标识，如 `privacy:mask`、`privacy:dp`、`classification:read`、`health:read`。

3. 根据权利要求 1 所述的方法，其特征在于，所述权限校验为每个 REST 路径与 gRPC 方法预先配置统一的权限标识，并通过判断身份 scope 列表是否包含该标识或通配符完成鉴权。

4. 根据权利要求 1 所述的方法，其特征在于，所述 CN 白名单包括 YAML 配置文件与静态 CN 列表两种模式；YAML 模式优先级高于静态模式；两者均未配置时白名单为空，拒绝所有证书。

5. 根据权利要求 1 所述的方法，其特征在于，所述热重载包括：每次请求到达时检查配置文件 mtime；mtime 更新时解析 YAML 到临时缓存，校验成功后原子替换当前配置；加载失败时保留旧配置并记录错误。

6. 根据权利要求 1 所述的方法，其特征在于，还包括基于身份名称与目标路径/方法的速率限制，采用移动窗口策略，单副本使用内存存储，多副本使用 Redis 存储。

7. 根据权利要求 1 所述的方法，其特征在于，还包括健康检查端点豁免：`/health` 路径与 `Health` 方法默认无需认证且不限速。

8. 根据权利要求 1 所述的方法，其特征在于，还包括 fail-closed 默认配置：mTLS 认证默认关闭、CA 校验通过默认不授予身份、CN 白名单默认空列表、TLS 客户端认证默认不请求证书。

9. 根据权利要求 1 所述的方法，其特征在于，还包括认证拒绝可观测：认证/鉴权/限速拒绝事件记录结构化日志，并更新 `privacy_auth_denials_total` Counter 与 `privacy_auth_duration_seconds` Histogram。

10. 一种面向隐私计算服务的 mTLS 白名单与 per-CN 细粒度 scope 访问控制系统，其特征在于，包括处理器、存储器及存储在所述存储器上的计算机程序，所述处理器执行所述计算机程序时实现如权利要求 1-9 任一项所述的方法。

### 摘要

本发明公开一种面向隐私计算服务的 mTLS 白名单与 per-CN 细粒度 scope 访问控制方法及系统。该方法在 mTLS 传输层 CA 校验之上增加应用层 CN 白名单校验，并为每个 CN 配置独立的权限 scope；通过 YAML 白名单配置文件支持 per-CN 最小权限控制与请求驱动热重载；通过统一身份模型同时覆盖 REST 与 gRPC 协议；并采用 fail-closed 默认策略与认证拒绝可观测机制。本发明有效防止证书泄露导致的横向越权，适用于隐私计算服务、医疗数据平台、金融数据共享等需要严格访问控制的场景。
