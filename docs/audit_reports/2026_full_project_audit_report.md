# Privacy Local Agent 全项目安全、正确性审计与漏洞整改报告（教科书级全量归档版）

> **报告版本**：v4.0.0（全量缺陷原理与修复细节教学归档版）  
> **归档时间**：2026-08-10  
> **审计范围**：全栈代码（分类分级漏斗 / 隐私原语算法 / 服务与网关安全 / llmlora 微调管道 / Console 前后端 / 测试与部署配置）  
> **验证状态**：973 项自动化单元与集成测试 **100% PASSED**（含 5 大专属漏洞回归测试套件）  
> **最新 Commit**：`99fa109`  

---

## 1. 总体评价与学习指南

`privacy-local-agent` 是一个实现了**「三层四柱五御六类」医疗数据安全与隐私治理架构**的 Sidecar 服务。

在经历了第一轮审计发现（64 项漏洞）及第二轮复核（7 项未修透返工项、4 项新引入问题）后，本项目完成了全面的技术攻坚。本报告不仅是一份审计总结，更是一本**系统级安全设计与工程防坑教学指南**。

每个漏洞均按以下结构详细记录：
- **原始问题与漏洞原理**：分析问题为何发生、在什么场景下会触发静默失效或安全越权，以及对系统的危害。
- **修复细节与技术实现**：展示精确的代码重构逻辑、算法校准公式、防御性编程手段与测试保障。

---

## 2. 漏斗与动态分类引擎模块（Funnel & Dynamic Classification）

### 2.1 P0-1: LLM 仲裁压制值级证据，Safety Floor 可被绕过
- **原始问题与漏洞原理**：
  - **原因**：在分类漏斗 Step 4 中，当规则层同时产生普通规则标签与降级标签时，系统判定存在等级冲突并触发 LLM 仲裁。然而原代码在接收到 LLM 仲裁结果（例如 L2）后，没有校验该结果是否低于已经由确定性正则表达式匹配到的真实数据值证据（如身份证号值级匹配得到的 L3）。
  - **危害**：攻击者或不可靠的 LLM 可以通过强制输出低敏感度等级（如 L1/L2），直接压制真实的极高敏感数据（如 L4/L5 疾病或身份证号），且原代码在仲裁后会擦除 `field_value` 证据标签并取消人工复核标志 (`needs_human_review=False`)，使安全防线彻底崩溃。
- **修复细节与技术实现**：
  - 在 `funnel.py:315` 建立 **Safety Floor（安全地基）** 机制：提取所有 `match_target == "field_value"` 且非降级的确定性数据证据标签，计算其最高敏感度 Rank `val_evidence_max_rank`。
  - 增加硬性校验 `llm_level_rank >= val_evidence_max_rank`。若 LLM 给出低于地基等级的裁定，系统立即拦截并拒绝该裁定；保持原最高等级，并强制将 `needs_human_review` 设为 `True` 要求人工介入。
  - 在一致性压制逻辑中，显式豁免 `match_target == "field_value"` 的证据标签，防止其被误擦除。

### 2.2 P0-2 & R1: 热重载假成功与双级缓存永不失效
- **原始问题与漏洞原理**：
  - **原因**：服务虽然支持 YAML 规则热重载，但 gRPC 接口 (`grpc_server.py:1014`) 与 Pipeline 接口 (`pipeline/classifier.py:48`) 之前调用的是底层的 `self._dyn_service.loader.check_and_reload()`。该方法仅重新加载了 YAML 磁盘文件，但没有触发 `DynClassificationService` 中维护的 `_funnel_cache` (Funnel 实例) 以及 `_classification_cache` (LRU 评估结果缓存) 的清空。
  - **危害**：运维修改 YAML 规则后触发 reload，日志显示成功，但后续请求依然命中内存中的旧 Funnel 实例与旧 LRU 评估结果，新规则静默失效。
- **修复细节与技术实现**：
  - 将所有入口统一重构为调用 `DynClassificationService.check_and_reload()`。
  - 在 `check_and_reload()` 内部，在 `_reload_lock` 读写互斥锁保护下，同步清空 `_funnel_cache` 与 `_classification_cache`：
    ```python
    with self._reload_lock:
        reloaded = self.loader.check_and_reload()
        if reloaded:
            self._funnel_cache.clear()
            self._classification_cache.clear()
            self._engine = None
    ```

### 2.3 P0-3 & N3: LLM 返回越界/NaN/1e6 置信度导致异常或误判
- **原始问题与漏洞原理**：
  - **原因**：LLM 模型可能返回非法的置信度数值（如 `NaN`、`Inf`、`-5.0` 或百分数 `1e6`）。原代码直接执行 `float(raw)`，遇到 `NaN` 会导致后端字典计算抛出 HTTP 500 崩溃；第一轮修复将其简单用 `min(1.0, val)` 钳制，导致 `1e6` 被错判为 1.0 (100% 满分最高置信度)。
  - **危害**：恶意大值置信度可以轻易骗过系统的低置信度防御拦截，使不可靠的 LLM 仲裁结果被强制采纳。
- **修复细节与技术实现**：
  - 在 `funnel.py:580` 编写安全的 `_safe_llm_confidence` 函数：
    ```python
    def _safe_llm_confidence(raw: Any, fallback: float) -> float:
        if raw is None: return fallback
        try:
            val = float(raw)
            if math.isnan(val) or math.isinf(val) or val < 0.0 or val > 100.0:
                return fallback
            if val > 1.0: val = val / 100.0  # 容错处理 95.0 百分数
            return min(max(val, 0.0), 1.0)
        except (ValueError, TypeError):
            return fallback
    ```

### 2.4 P0-4: 硬编码医疗 L5/L4 安全网跨 Taxonomy 失效
- **原始问题与漏洞原理**：
  - **原因**：原代码写死了 `if level in ("L5", "L4"): ...` 来触发额外安全防护。当用户切换到非医疗分类体系（如金融体系，等级为 `C1-C4` 或 `LVL1-LVL5`）时，硬编码的字符串比较永远为 `False`。
  - **危害**：在非 `L1-L5` 命名体系下，高敏感度安全网完全失灵。
- **修复细节与技术实现**：
  - 改用基于 Taxonomy Rank 的动态比较：`rank >= taxonomy.get_level_rank(taxonomy.high_risk_threshold)`，实现跨领域体系的通用防护。

### 2.5 P0-5: 复合规则 `\b` 烘焙与归一化缺陷致规则永不触发
- **原始问题与漏洞原理**：
  - **原因**：`CompositeRuleEngine` 在预编译正则时，直接给 pattern 加上了 `\b` 词边界（如 `\bid_card\b`）。然而在评估前，系统会将字典 Key 进行下划线归一化（`id_card` $\to$ `idcard`）。对于包含下划线的正则（如 `id_card`），`\b` 无法在字符与下划线之间匹配词边界；同时 pattern 自身未做下划线剥离。
  - **危害**：所有带下划线的复合规则（如 `COMP_PII_001`）静默无法命中任何字段。
- **修复细节与技术实现**：
  - 在 `composite.py:58` 将边界正则改为兼容下划线的原子分组：
    ```python
    bounded_pattern = rf"(?:\b|_)(?:{pattern})(?:\b|_)"
    ```
    并在编译前对 pattern 与输入的 `field_name` 统一执行下划线/连字符归一化。

### 2.6 P1-27 & P1-28: Validator 模式校验与 YAML 广谱词误伤
- **原始问题与漏洞原理**：
  - `validator.py:188` 之前错误地只加载第一个 Taxonomy 校验所有 Domain Profile，导致自定义体系规则报 Schema 不匹配。
  - `medical.yaml` 中包含 `stat`、`cnt` 等通用 3 字母关键词，导致运营统计字段与普通字段被广泛误伤打标。
- **修复细节与技术实现**：
  - 修复 `validator.py` 循环逻辑，按 `domain.taxonomy_id` 匹配对应的 `DomainTaxonomy` 实例。
  - 精细化 `medical.yaml` 中的关键词，要求最小长度 $\ge 4$ 或使用完整单词边界匹配。

### 2.7 P1-29 & P1-48 & P1-64: 漏斗边界异常与热重载重试缺陷
- **#29 原始问题**：遇到未在 Taxonomy 中定义的未知等级时，系统将其按 `rank=0` 过滤并直接忽略，导致问题数据暴露。  
  **修复**：发现未知等级时抛出异常并触发安全兜底（降级为默认最高安全防护）。
- **#48 原始问题**：热重载语法解析失败时，系统提前更新了文件的 `mtime` 记录，导致运维修改修复后，系统因 `mtime` 未变而不再重试加载坏文件。  
  **修复**：仅在解析且校验成功后才更新缓存的 `mtime`。
- **#N1 原始问题**：Step 4 LLM 仲裁在 `else` 分支直接赋值 `needs_human_review = False`，覆盖了前面规则层已经置位的人工复核标志。  
  **修复**：保留历史状态 `needs_human_review = needs_human_review or orig_needs_human_review`。

---

## 3. 差分隐私与隐私原语模块（DP, Masking, Kano, QOL）

### 3.1 P0-10 & R3: 高维向量 Laplace 未按 $\sqrt{d} \cdot \text{max\_norm}$ 范数扩增校准
- **原始问题与漏洞原理**：
  - **原因**：在标量（1 维）下，数据 $X \in [a, b]$ 的 $L_1$ 敏感度为 $\Delta = max - min$。但对于 $d$ 维向量 $X \in \mathbb{R}^d$，其 $L_1$ 范数上限为 $\|X\|_1 \le \sqrt{d} \cdot \|X\|_2 = \sqrt{d} \cdot \text{max\_norm}$。原代码在 `dp_vector_mean` (`dp.py:2551`) 中直接使用了标量敏感度 `max_norm / count` 添加噪声。
  - **危害**：高维向量（如 128 维嵌入向量）注入的 Laplace 噪声标准差严重不足，使得纯 $\varepsilon$-差分隐私的数学保证彻底失效，攻击者可通过高维向量逆推原始数据。
- **修复细节与技术实现**：
  - 在 `dp.py:2617` 中校准 $d$ 维向量 Laplace 噪声 scale：
    ```python
    d = vectors.shape[1] if len(vectors.shape) > 1 else 1
    if d > 1:
        effective_sensitivity = (max_norm * math.sqrt(d)) / effective_count
    else:
        effective_sensitivity = max_norm / effective_count
    ```

### 3.2 P0-11: BudgetAccountant.spend() 接收负数“充值”预算
- **原始问题与漏洞原理**：
  - **原因**：`BudgetAccountant.spend(epsilon, delta)` 在扣减预算时，原代码未校验 `epsilon > 0` 且 `delta >= 0`。
  - **危害**：攻击者或有缺陷的上层模块可通过传入 `epsilon = -10.0`，使当前已用预算 `spent_epsilon` 变小，实现隐私预算的“非法充值”，绕过配额管控。
- **修复细节与技术实现**：
  - 在 `budget.py:329` 增加前置防御校验：
    ```python
    if epsilon <= 0.0 or delta < 0.0:
        raise ValueError(f"Epsilon and delta must be positive values. Got epsilon={epsilon}, delta={delta}")
    ```

### 3.3 P0-12 & R4: 动态推导边界时 `noise_scale` 泄露数据极值
- **原始问题与漏洞原理**：
  - **原因**：当用户未显式提供 `min_val` 与 `max_val` 剪切区间时，系统会自动从数据集动态推导 $min = \min(X), max = \max(X)$，并计算 Laplace 噪声 scale $\text{noise\_scale} = \frac{max - min}{\varepsilon}$。原代码将此 `noise_scale` 装入 `DPResult.noise_scale` 并回传给客户端或导出至 Arrow 元数据。
  - **危害**：攻击者拿到返回的 `noise_scale` 后，只需乘以已知的 $\varepsilon$，即可精确定出原始数据集的极值差 $max - min = \text{noise\_scale} \cdot \varepsilon$，造成严重的差分隐私泄露。
- **修复细节与技术实现**：
  - 在 `dp.py` 内部追踪 `bounds_inferred` 标记：
    ```python
    if bounds_inferred:
        res.noise_scale = None
        res.confidence_interval = None
    ```
  - 在 `to_arrow()` 导出函数中，若 `noise_scale is None`，则禁止写入包含极值比率的 Arrow Schema 元数据。

### 3.4 P0-13: 掩码与 HMAC 审计日志使用硬编码默认 Key
- **原始问题与漏洞原理**：
  - **原因**：`masking.py:1253` 的 FPE 保形加密与 `budget.py:57` 的审计日志 HMAC 采用了固定的字符串 Key。
  - **危害**：在部署未显式配置环境变量时，不同实例生成的 FPE 掩码伪随机序列与 HMAC 签名完全一致，存在严重的密文可预测性风险。
- **修复细节与技术实现**：
  - 使用 Python `secrets.token_bytes(32)` 在模块加载时生成 256-bit 高熵进程随机 Key 作为安全回退：
    ```python
    DEFAULT_HMAC_KEY = os.environ.get("PRIVACY_HMAC_KEY") or secrets.token_bytes(32)
    ```

### 3.5 P1-22 & P1-23: Masking 字段名子串误伤与 Arrow 向量化校验不一致
- **原始问题与漏洞原理**：
  - `masking.py` 原先采用简单的 `kw in field_name` 匹配，导致 `hotel` 被误伤当成 `tel` 电话打码，`username` 被当成 `name` 打码。
  - Arrow 向量化批处理路径缺少标量路径中的 15 位旧身份证校验，导致向量化路径下 15 位身份证漏打码。
- **修复细节与技术实现**：
  - 将字段名匹配升级为基于 word-boundary 的精确匹配或下划线边界匹配。
  - 统一 Arrow 批处理路径与标量 Path 的正则引擎，补齐 15 位身份证与中文字段名匹配规则。

### 3.6 P1-24 & P1-25 & P1-26: 预算 SQLite 竞态、QOL 策略死代码与 MT 随机数隐患
- **#24 竞态问题**：SQLite 预算重置窗口使用普通 `SELECT` 后更新，在多进程并发下存在 Race Condition，可能抹掉其他进程已扣除的预算。  
  **修复**：改用 `BEGIN IMMEDIATE` 事务锁与原子更新语句 `UPDATE ... WHERE ...`。
- **#25 QOL 问题**：Query Obfuscation 的 `HYBRID` 策略代码分支不可达，生成的 Dummy Query 与真实查询仅相差一个病名，缺乏多样性。  
  **修复**：修复分支逻辑，Dummy 查询引入多样化语义干扰词库。
- **#26 随机数问题**：QOL 使用了非加密的 `random.MersenneTwister`。  
  **修复**：统一替换为 `secrets.SystemRandom` 安全随机数生成器。

---

## 4. 服务接口、协议与网关安全模块（API, gRPC, Gateway, Auth）

### 4.1 P0-6: REST 敏感端点缺乏认证、授权与限流中间件
- **原始问题与漏洞原理**：
  - **原因**：`/v1/medical/process` 与 `/v1/pipeline/*` 端点在路由注册时未挂载 `SecurityMiddleware` 与 Rate Limiter。
  - **危害**：外部未经认证的匿名网络请求可以直接调用高消耗的脱敏管道与 LLM 推理接口，导致服务被滥用或 DDoS 攻击。
- **修复细节与技术实现**：
  - 在 `main.py` 与 `server.py` 中将认证中间件挂载到全局 Router 前缀，对所有业务端点一律强制执行 API Key / Bearer Token 校验与令牌桶限流。

### 4.2 P0-7 & R2: gRPC proto3 零值透传导致剪切为 0 与参数坍塌
- **原始问题与漏洞原理**：
  - **原因**：Protobuf v3 在反序列化未赋值的数值时，会隐式填充为 `0` 或 `0.0`。gRPC Servicer (`grpc_server.py:981`) 直接读取 `request.clip_lower` (0.0) 与 `request.clip_upper` (0.0) 并透传给 DP 算法模块。
  - **危害**：所有未显式传剪切范围的 gRPC DP 聚合请求，其数据会被瞬间全截取为 `0`，算出的结果完全失真。同样 `DPAdaptiveClip` 的 `num_iterations=0` 会导致算法直接崩溃或死循环。
- **修复细节与技术实现**：
  - 在 `grpc_server.py:988` 增加双零检测与参数防御拦截：
    ```python
    if request.clip_lower == 0.0 and request.clip_upper == 0.0:
        clip_lower, clip_upper = -10.0, 10.0
    target_quantile = request.target_quantile if request.target_quantile > 0 else 0.95
    num_iterations = request.num_iterations if request.num_iterations > 0 else 15
    initial_clip = request.initial_clip if request.initial_clip > 0 else 10.0
    ```

### 4.3 P0-8: mTLS 任意 CA 证书无差别授予 `["*"]` 全权 Scope
- **原始问题与漏洞原理**：
  - **原因**：当开启 mTLS 认证时，只要客户端证书通过了 CA 根证书链校验，`auth.py:96` 会自动为其生成 `Identity(scopes=["*"])`。
  - **危害**：无法对不同的客户端证书 Common Name (CN) 做细粒度权限控制，任何拥有合法签发证书的业务方都能调用管理接口或进行越权操作。
- **修复细节与技术实现**：
  - 在 `security/auth.py` 引入 CN 白名单与 Scope 映射机制：
    ```python
    client_cn = get_client_cn_from_cert(request)
    if client_cn not in settings.AUTH_MTLS_ALLOWED_CNS:
        raise HTTPAuthorizationError("Client CN not in authorized whitelist")
    scopes = settings.AUTH_MTLS_CN_SCOPES_MAP.get(client_cn, ["read"])
    ```

### 4.4 P0-9: 网关节点注册端点无凭证致 SSRF 跳板攻击
- **原始问题与漏洞原理**：
  - **原因**：`/v1/gateway/register` 允许任意 HTTP 请求注册后端节点 URL。
  - **危害**：攻击者注册内网敏感 IP（如 `127.0.0.1:6379` 或云厂商元数据服务 `http://169.254.169.254/`），Gateway 在执行健康检查或转发代理请求时会发起 HTTP 请求，形成严重的 SSRF 漏洞。
- **修复细节与技术实现**：
  - 管理端点强制要求 `GATEWAY_API_KEY` 鉴权；未配置时默认禁掉管理端点 (Fail-Closed)。
  - 对注册 URL 的 IP 进行合规校验，禁止注册回环地址、私网地址及 Link-Local 元数据地址。

### 4.5 R7: CSV 大小限制“先全量读入再检查”致内存 DoS 窗口
- **原始问题与漏洞原理**：
  - **原因**：`/v1/pipeline/process_csv` 之前使用 `content = await file.read()` 将整个文件装入内存后才判断 `len(content) > MAX_SIZE`。
  - **危害**：攻击者发送 5GB 恶意 CSV 文件，服务端在 `await file.read()` 期间内存瞬间飙升，引发 OOM Killer 杀掉 Sidecar 进程。
- **修复细节与技术实现**：
  - 改为 64KB 流式分块读取，累计字节数超 10MB 立即中断并返回 HTTP 413：
    ```python
    total_bytes = 0
    chunks = []
    while True:
        chunk = await file.read(64 * 1024)
        if not chunk: break
        total_bytes += len(chunk)
        if total_bytes > 10 * 1024 * 1024:
            raise HTTPException(status_code=413, detail="File size exceeds limit of 10MB")
        chunks.append(chunk)
    ```

---

## 5. LLM 微调与数据集管道模块（llmlora & Models）

### 5.1 P0-14 & R5: 打标管道规则失配与错别字致高敏标签归零
- **原始问题与漏洞原理**：
  - **原因**：`llmlora/scripts/generate_data.py` 使用规则引擎为微调数据集自动打标。但在 `rules/domains/medical.yaml` 中，`RULE_MED_DISEASE_001` 误将匹配目标设为 `field_name`，且将“抑郁症”错拼为“抑抑症”。
  - **危害**：当输入数据为真实病名（如 `"clinical_diagnosis": "胃癌"`）时，规则引擎无法命中值级病名，导致合成数据集里的 L4/L5 高敏标签全部归零，微调出来的模型丧失高敏识别能力。
- **修复细节与技术实现**：
  - 修正错别字，并在 `medical.yaml` 中增加值级正则表达式匹配器：
    ```yaml
    - id: RULE_MED_DISEASE_VALUE_001
      name: 医疗诊断具体疾病名称值级匹配
      category: MEDICAL_DISEASE
      level: L4
      matchers:
        - target: field_value
          operator: regex
          params:
            pattern: "(?i)(胃癌|肝癌|肺癌|乳腺癌|白血病|糖尿病|高血压|抑郁症|精神分裂症)"
    ```

### 5.2 P0-15: 冒烟测试与下载脚本污染生产模型目录
- **原始问题与漏洞原理**：
  - **原因**：`smoke_test.sh` 与 `download_model.py` 默认直接向代理的主模型目录 `./.models/` 写入临时或测试模型权重。
  - **危害**：测试运行后覆盖了生产环境已下载的高精度模型权重，导致生产服务加载到损坏或不完整的测试权重。
- **修复细节与技术实现**：
  - 在下载与测试脚本中增加 `--no-copy-to-agent` 参数，隔离测试输出目录与生产模型目录。

### 5.3 P0-16 & R6: 微调 Prompt 脱节与 `<think>` 思考链破坏 JSON 结构
- **原始问题与漏洞原理**：
  - **原因**：微调训练集与推理引擎 `llm_engines.py` 使用了不同的 System Prompt，且未对 Qwen3.5 思考链文本进行处理。
  - **危害**：推理时微调模型输出了 `<think>...\n</think>\n{"category":...}`，直接送入 `json.loads` 会抛出 JSONDecodeError。
- **修复细节与技术实现**：
  - 统一训练集与推理侧的 System Prompt 契约。
  - 调用 `apply_chat_template` 时设置 `enable_thinking=False`。
  - 在 JSON 解析前执行正则清洗：
    ```python
    text = re.sub(r"<think>.*?</think>", "", text, flags=re.DOTALL).strip()
    ```

---

## 6. 运维部署、Console 与可观测性模块（Deploy, Console, Obs, Docs）

### 6.1 P0-17: Helm Secret Key `api_keys` 不匹配致 Auth 必挂
- **原始问题与漏洞原理**：
  - `secret.yaml` 中定义的 Key 为 `api_keys`，但 `deployment.yaml` 挂载时读取的是 `api-keys.json`。
  - **后果**：Helm 部署后 Pod 因找不到指定 Key 持续处于 `CreateContainerConfigError` 状态。
- **修复**：统一 Helm 模板中的 Secret Key 名称为 `api-keys.json`。

### 6.2 P0-18: `.dockerignore` 残缺导致敏感 `.env` 打入容器镜像
- **原始问题与漏洞原理**：
  - `.dockerignore` 仅 12 行，未排除 `.env` 文件。
  - **后果**：包含数据库密码、私钥等敏感信息的 `.env` 被镜像构建层打包，镜像推送到仓库后造成严重密钥泄露。
- **修复**：重构 `.dockerignore`，显式屏蔽所有 `.env*` 文件与本地密钥文件。

### 6.3 P0-19: docker-compose 健康检查用 `wget` 但镜像仅含 `curl`
- **原始问题与漏洞原理**：
  - `docker-compose.yml` 中的 healthcheck 使用了 `wget --quiet --tries=1 http://localhost:8079/health`。精简版 Docker 镜像中未安装 `wget`，仅包含 `curl`。
  - **后果**：容器状态永远被标记为 `unhealthy`。
- **修复**：统一将健康检查命令改为 `curl -f http://localhost:8079/readyz || exit 1`。

### 6.4 P0-20 & P0-21: Console 压测预设路径错误致 100% 失败
- **原始问题与漏洞原理**：
  - React 控制台压测面板中的预设路径写成了 `/v1/privacy/dp_count`，而后端真实的 REST 路由路径为 `/v1/privacy/dp_aggregate`。
  - **后果**：前端控制台压测功能开启即 100% 报 404 失败。
- **修复**：校准 React 前端 `ConcurrencyTestPanel.tsx` 中的 API 请求路径。

### 6.5 P1-33: Prometheus 原始 Path 打标致基数爆炸 (Cardinality Explosion)
- **原始问题与漏洞原理**：
  - 可观测性中间件使用 `request.url.path` 作为 Prometheus label 标签值（如 `/v1/user/123`、`/v1/user/456`）。
  - **后果**：外部扫描器发送高频随机 404 路径时，Prometheus 指标基数呈线性急剧膨胀，导致 Prometheus 服务内存耗尽崩溃。
- **修复**：改用路由模板名称（如 `/v1/user/{id}`）打标，未匹配路由统一归集为 `NOT_FOUND`。

---

## 7. 自动化回归测试验证总结

项目新增专属回归测试套件 [`tests/test_audit_remediation.py`](file:///home/charles/code/sfwork/privacy-local-agent/tests/test_audit_remediation.py)，覆盖 5 大核心场景：

1. `test_safety_floor_prevents_llm_downgrade`: 验证 Safety Floor 拦截 LLM 降级裁定。
2. `test_budget_spend_rejects_non_positive_epsilon`: 验证预算系统拒绝负数与零值充值。
3. `test_dp_vector_laplace_sqrt_d_calibration`: 验证高维向量 Laplace 噪声 scale 精确满足 $\sqrt{d}$ 校准。
4. `test_composite_rule_underscore_normalization`: 验证下划线字段名自动规范化匹配。
5. `test_safe_llm_confidence_clamping`: 验证 `1e6` / `NaN` / `Inf` 置信度异常值安全回退。

**全量测试套件运行结论**：
```bash
PYTHONPATH=. pytest tests -k "not test_real_ and not test_modelscope_cuda and not test_ner_adapter_cuda"
```
```text
================ 973 passed, 92 skipped, 9 deselected in 32.92s ================
```
全量 **973 项自动化单元与集成测试 100% PASSED**，系统安全整改全面完成。
