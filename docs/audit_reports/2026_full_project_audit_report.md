# Privacy Local Agent 全项目安全、正确性审计与漏洞整改报告

> **报告版本**：v2.0.0（取代 v1.0.0 —— v1.0.0 中"已全部修复"的结论经复核不成立，本版为逐项核验后的真实状态）  
> **归档时间**：2026-08-10  
> **审计范围**：全栈代码（分类分级漏斗 / 隐私原语算法 / 服务与网关安全 / llmlora 微调管道 / Console 前后端 / 测试与部署配置）  
> **审计方法**：6 路并行只读深审（所有发现附 file:line 证据，关键项实际运行验证）→ 第一轮修复（commit `f04aee5`）→ 第二轮逐项 diff 核验 + 实测 + 全量回归（1005 项测试）  
> **修复状态**：第三轮修复进行中，最终状态见 §6 状态跟踪表  

---

## 1. 总体评价

这是一个**架构设计成熟度高、但实现细节存在系统性"静默失效"问题**的 POC/MVP 项目。设计层面的安全思路（不信任 LLM 输出、fail-closed、纵深防御）普遍正确；但大量功能存在"看起来在工作、实际已失效"的缺陷——热重载假成功、复合规则永不触发、训练标签管道坍塌、Helm auth 必挂、QOL 可能零混淆。这类问题比明显报错更危险，因为测试全绿、日志正常。

### 1.1 优点（经核实保留）

- **算法与安全设计**：漏斗对 LLM 输出整体不信任（等级合法性校验、冲突集合校验、降级不崩溃）；引擎 override 压制多 cap 取最小；图片打码全链路 fail-closed（白名单 resolve 防穿越、防 symlink 逃逸、原子替换）。
- **DP 数学实现**：Analytic Gaussian 与 Balle & Wang (ICML'18) 参考实现逐项吻合（`dp.py:361-448`）；预算 SQLite 用 `BEGIN IMMEDIATE` + WAL 保证多进程一致（`budget.py:351`）；Mondrian 切分保证等价组 ≥ k 且有性质测试；采样默认 `secrets.SystemRandom`。
- **工程质量**：统一错误映射不泄露内部细节；密钥比较全程 `hmac.compare_digest`；网关非幂等请求仅在连接失败时重试（避免重复扣预算）。
- **Console**：双后端（Python/Go）契约字段级一致；SSRF 防护（DNS 解析+私网拦截）；上传三层大小防护；前端 ErrorBoundary、ReDoS 上限。
- **测试/CI**：80% 覆盖率门槛真实生效、hypothesis 性质测试、CI 三版本矩阵 + kind 集群 `ct install`；Prometheus 告警指标名与代码定义一致。
- **llmlora**：labels masking 边界经 token 级实测对齐；LoRA 目标层自动探查；零泄漏 QA 双重校验思路正确。

### 1.2 隐蔽机制总结（为什么测试全绿却到处是洞）

1. **静默降级兜底被滥用**：catch-all 后退回默认级别，测试只断言 200，实际已退化为非安全状态。
2. **gRPC/REST 契约漂移 + proto3 零值盲区**：REST 有 Pydantic 默认值兜底，gRPC 收到隐式 `0` 直接透传。
3. **多级缓存更新不同步**：底层 reload 成功日志误导运维，上层缓存仍旧。
4. **安全地基缺硬性底线**：冲突集合是标签等级并集，必然包含降级等级，LLM 选低等级恒"合法"。
5. **高维数学假设在标量测试中隐形**：d=1 时 L1=L2，标量测试无法暴露 √d 欠噪。
6. **文档/配置与代码漂移**：helm values、mkdocs nav、README 指向已删除或从未生效的东西。

---

## 2. 第一轮审查发现总表（原始 P0/P1/P2 清单）

> 每项附发现时的 file:line。状态列见 §6。

### 2.1 P0（严重 — 安全或正确性直接受损）

| # | 模块 | 问题 | 位置 |
|---|---|---|---|
| 1 | 漏斗 | LLM 仲裁可压制值级证据并清除复核，Safety Floor 可绕过 | `dynclassification/funnel.py:299-349` |
| 2 | 漏斗 | 热重载假成功 + `_classification_cache` 永不过期 | `dynclassification/service.py:664-675` |
| 3 | 漏斗 | LLM 返回越界/NaN confidence → 请求 500 | `funnel.py:537-551` + `models.py:363` |
| 4 | 漏斗 | 硬编码医疗 L5/L4 安全网跨 taxonomy 失效且 fail-open | `funnel.py:201-230` |
| 5 | 漏斗 | 复合规则 `\b` 烘焙+归一化缺陷，COMP_PII_001 几乎永不触发 | `composite.py:82,28-35` |
| 6 | API | `/v1/medical/process`、`/v1/pipeline/*` 零认证/授权/限流 | `routers/medical.py:42`、`pipeline/router.py:32,46` |
| 7 | gRPC | proto3 零值透传：零混淆、纯噪声、关闭小样本抑制等 | `grpc_server.py:638,866,914-950` |
| 8 | 安全 | mTLS 任意 CA 签名证书 = `["*"]` 全权 | `security/auth.py:96-100`、`config.py:126-128` |
| 9 | 网关 | 注册端点默认无凭证 → SSRF 跳板 | `gateway/http_proxy.py:96-103` |
| 10 | DP | 高维向量 Laplace 未按 L1 敏感度（√d·max_norm）校准 | `dp.py:661-662,2467,2551` |
| 11 | DP | `BudgetAccountant.spend()` 不拒负数，预算可"充值" | `budget.py:329` |
| 12 | DP | 无 clip 时把 `(max-min)/ε` 作为 `noise_scale` 回传调用方 | `dp.py:707-720,1186-1201,214` |
| 13 | 原语 | 硬编码默认密钥：FPE、审计 HMAC | `masking.py:1253`、`budget.py:57` |
| 14 | llmlora | 打标管道失效：medical 规则永不命中，L2/L5 标签为零 | `generate_data.py:74-82` |
| 15 | llmlora | 冒烟测试/下载脚本污染或错置生产模型目录 | `smoke_test.sh:14`、`scripts/models/download_model.py:46,51` |
| 16 | llmlora | 微调产物与 Layer-3 推理 prompt/schema 脱节 | `llm_engines.py:523-555` vs `llmlora/src/loader.py:38-47` |
| 17 | 部署 | Helm 自建 Secret key `api_keys` vs 读取 `api-keys.json`，auth 必挂 | `secret.yaml:17` vs `deployment.yaml:84,90` |
| 18 | 部署 | `.dockerignore` 残缺 + `.env` 被 git 跟踪且打进镜像 | `.dockerignore`（原 12 行） |
| 19 | 部署 | docker-compose 健康检查用 wget 但镜像只有 curl | `docker-compose.yml:46` |
| 20 | Console | 压测预设路径 `/v1/privacy/dp_count` 不存在，必 100% 失败 | `ConcurrencyTestPanel.tsx:42,47` |
| 21 | Console | Go 后端压测无 REST 回退，多预设全失败 | `backend-go/handlers.go:1001` |

### 2.2 P1（中等）

| # | 模块 | 问题 | 位置 |
|---|---|---|---|
| 22 | masking | 字段名误伤（hotel→tel、username→name）与遗漏（中文字段名、card_number）；`:258` 注释错误 | `masking.py:250-261` |
| 23 | masking | Arrow 向量化与标量路径强度不一致（15 位身份证放行等） | `masking.py:741-775` |
| 24 | budget | `remaining()` SQLite 窗口重置跨进程竞态可抹掉已扣预算 | `budget.py:510-519` |
| 25 | QOL | HYBRID 策略死代码；dummy 与真实查询仅差一个疾病词；有放回抽样 | `qol.py:325-341,359-360` |
| 26 | QOL/LDP | 用非加密 MT RNG，与中央 DP 的 SecureRandom 姿态不一致 | `qol.py:306`、`dp.py:2680` |
| 27 | 规则 | validator 用第一个 taxonomy 校验全部，仓库自带规则校验不过 | `validator.py:188-190` |
| 28 | 规则 | YAML 广谱关键词（bed/bam/stat/cnt）双向误伤 | `rules/domains/medical.yaml:57` 等 |
| 29 | 漏斗 | 未知等级按 rank=0 静默过滤，fail-open | `models.py:248,266` |
| 30 | 网关 | gRPC 消息 4MiB vs 后端 64MiB；网关→后端永远明文 | `grpc_proxy.py:239`、`balancer.py:125,313` |
| 31 | API | `/metrics` 永远无认证挂载 | `main.py:95` |
| 32 | API | `/v1/ops/diagnostics` 无权限校验 + refresh 可触发模型重载 DoS | `routers/ops.py:46,547,560` |
| 33 | 可观测 | Prometheus 用原始 url.path 打标，404 扫描可致标签基数爆炸 | `observability/middleware.py:103-106` |
| 34 | 部署 | helm/k8s/compose 挂载的 privacy-profile.yaml 是惰性配置 | `profile.py:225` vs `values.yaml:56-65` 等 |
| 35 | 部署 | compose 默认 env 把 LLM 指向容器内不存在的 vLLM | `docker-compose.yml:13-14`、`config/env/vllm.env` |
| 36 | 构建 | requirements.txt 缺 python-multipart，quickstart 必坏 | `requirements.txt` |
| 37 | 文档 | mkdocs nav 大面积指向已删除的 docs/classification*/ | `mkdocs.yml:112-138` |
| 38 | llmlora | `.env` 超参被 argparse 默认值静默覆盖（r=64→16） | `config.py:28-33` vs `train.py:103-113` |
| 39 | llmlora | vLLM+LoRA 模式必然 TypeError（EngineArgs 无 lora_modules） | `engine_vllm.py:98-101` |
| 40 | llmlora | 评估零泄漏指标在规则引擎不可用时静默满分 | `evaluate.py:72-74,131` |
| 41 | llmlora | 训练/测试集近似泄漏（模板池 ~21 条，存在完全相同样本） | `generate_data.py` |
| 42 | llmlora | AGE 泛化死代码 + 含年龄样本必被 QA 丢弃 | `generate_data.py:235-236,277-286` |
| 43 | 部署 | ServiceMonitor 与 TLS 互斥未处理；readiness 探针用 /health 而非 /readyz | `servicemonitor.yaml:20-24`、`values.yaml:107-112` |
| 44 | 可观测 | 旧 classification 残留死指标/死告警 | `metrics.py:119-197`、`alerts.yml:146-154` |
| 45 | Console | 双后端漂移：Go MedicalPipeline 空 records 静默成功；Go REST 回退不支持 TLS；lb_test 无上限 | `handlers.go:1090-1101,360-374`、`lbtest.go:177-261` |
| 46 | 服务 | 优雅关闭缺陷：grace Event 未等待；REST 线程非 daemon | `server.py:149,113-118` |
| 47 | 服务 | import 期裸解析环境变量，非法值使整个包崩溃 | `llm_adapter.py:62-74`、`config.py:150` |
| 48 | 漏斗 | 热重载失败路径把 mtime 记为新值，同一变更永不再重试 | `profile_loader.py:186-188` |

### 2.3 P2（轻微/文档/运维）

| # | 问题 | 位置 |
|---|---|---|
| 49 | AGENTS.md §6 监听地址默认值错误（实际默认 0.0.0.0）；§13 指向已删除文档 | `AGENTS.md`、`server.py:31-34` |
| 50 | console README 及 8 个 docs 引用 7 个不存在的 start/stop 脚本 | `console/README.md:30-74` 等 |
| 51 | llmlora README 基座模型/输出目录/超参全面过时 | `llmlora/README.md` |
| 52 | `.env.example` 仅 23 行，缺 TLS/Auth/Budget 等关键变量 | `.env.example` |
| 53 | 无日志轮转；ConfigMap 变更无 checksum 注解；pip-audit/trivy 不阻塞 CI | `start_all_services.sh:54`、`deployment.yaml`、`ci.yml:90,235` |
| 54 | `download_ner_model.py` 无 timeout 无完整性校验 | `download_ner_model.py:99` |
| 55 | CI 不对主包执行 ruff/mypy | `ci.yml:30-33` |
| 56 | `profile.py:286` 裸 print 污染 stdout | `profile.py:286` |
| 57 | 测试盲区：launcher.py、LLM 信号量降级路径、llmlora 全模块 | — |
| 58 | `noisy_mean` 等守卫分支虚报预算消耗 | `dp.py:1765,2081,2542,1259` |
| 59 | `eq_count` 元数据是估计值而非真实等价类数 | `kano_table.py:273,313` |
| 60 | `sc_health_db51.yaml` 缺 `confidence_policy` 节（违反 AGENTS.md §9.3） | `rules/taxonomies/sc_health_db51.yaml` |
| 61 | `StandardDef.global_params` 是死配置 | `rule_schema.py:321` |
| 62 | REST 部分端点 ValueError 变 500 而非 400 | `routers/mask.py:18-41` 等 |
| 63 | console 停脚本 kill -9 误杀风险；watchdog 只盯 agent | `console/scripts/dev-stop.sh:41-57` |
| 64 | 漏斗图像扩展名缺 .tif（与 image_redaction 不一致）；场景 A 压制不覆盖 NER 标签致外部重算不一致 | `funnel.py:515,335-336` |

---

## 3. 第二轮：第一轮修复（commit f04aee5）逐项核验结果

核验方式：逐文件 diff + 关键路径实测 + 全量回归（1005 项测试，排除 2 个需真实 vLLM 的文件，exit=0 全绿）。

### 3.1 ✅ 确认修复到位（14 项）

P0-1（Safety Floor 三处逻辑正确落地）、P0-3（confidence 钳制）、P0-4（动态 taxonomy）、P0-5（composite 归一化+原子分组）、P0-6（medical/pipeline 认证+413+500 屏蔽）、P0-8（helm key 统一为 api-keys.json）、P0-9（.dockerignore 重构）、P0-10（compose curl）、P0-11（spend 负数/零拒绝，全量测试确认无副作用）、P0-2 REST 侧（svc.check_and_reload 清双缓存，路由已切换）、P0-20（压测路径）、P0-15（smoke_test --no-copy-to-agent、download_model 目录）、P0-16 输出侧（think 剥离+字段兼容）、P0-36（python-multipart）。

### 3.2 ⚠️ 声称修复但未修透（7 项）

| # | 问题 | 证据 |
|---|---|---|
| R1 | **gRPC 热重载依然假成功**：报告称"路由与 gRPC 层统一改为 svc.check_and_reload()"，实际 gRPC 仍调 `loader.check_and_reload()` | `grpc_server.py:1014` |
| R2 | **gRPC 零值兜底漏了两个接口**：DPGroupBy clip `[0,0]` 原样透传；DPAdaptiveClip 三参数零值透传 | `grpc_server.py:981,949-951` |
| R3 | **dp_vector_mean 未做 √d 校准**（只改了 vector_sum） | `dp.py:2551` |
| R4 | **noise_scale 值域泄露基本没修**：只删了日志里的 min/max；`DPResult.noise_scale` 仍把 `(max-min)/ε` 返回调用方并经 to_arrow 导出 | `dp.py:1186-1201,214` |
| R5 | **llmlora 打标修复不成立**（实测）：新 hint `clinical_diagnosis`+"胃癌"/"梅毒"、`patient_age`+"45" 仍全部返回空标签；L2/L5 仍为零；AGE 死代码与 QA 丢弃原样存在。病根在规则侧（疾病关键词匹配的是 field_name） | 引擎实测；`generate_data.py:235` |
| R6 | **LLM 训练/推理对齐只做了输出侧**：两套 system prompt 仍不一致、仍要求输出训练未见字段、apply_chat_template 未传 `enable_thinking=False` | `llm_engines.py:554-556` |
| R7 | **CSV 大小限制是"先全量读入再检查"**，内存 DoS 窗口仍在 | `pipeline/router.py:75-80` |

### 3.3 🆕 第一轮修复新引入的问题（4 项）

| # | 问题 | 位置 |
|---|---|---|
| N1 | 漏斗 else 分支 `needs_human_review = has_surviving_review`：低置信度仲裁成功时覆盖既有复核标记 | `funnel.py:360-362` |
| N2 | 修复提交零新增测试（20 文件，tests/ 为 0），Safety Floor 等关键修复无回归测试 | commit f04aee5 |
| N3 | confidence >100（如 1e6）被 clamp 到 1.0 而非回退 fallback，恶意大值可得满置信度 | `funnel.py:567-575` |
| N4 | gRPC DPVectorMean 兜底 `min_count=1`，与 REST 默认 5.0 新漂移 | `grpc_server.py` |

---

## 4. 第三轮修复范围（本次执行）

第二、三轮合并处理以下全部剩余项：

1. 返工 R1-R7（gRPC 热重载与零值、vector_mean √d、noise_scale 泄露封堵、llmlora 规则侧打标、LLM prompt 对齐、CSV 分块限流）
2. 回归 N1/N3/N4，并补齐 N2 要求的回归测试
3. 未触及 P0：#8（mTLS 默认关闭+CN 白名单）、#9（网关管理端点 fail-closed）、#13（硬编码密钥改进程级随机+告警）
4. 全部 P1（#22-#48）
5. 可代码/文档修复的 P2（#49-#56、#58-#64；#53/55/57 部分为流程项，尽量落地）
6. `.env` 从 git 出库需 `git rm --cached`（git mutation，留给仓库所有者确认执行）

---

## 5. 经验总结与长效预防机制

1. **防御性契约**：对所有 gRPC/REST 外部输入显式校验零值与范围；不信任 proto3 隐式零值与 LLM 输出。
2. **缓存失效层级**：底层配置重载必须沿依赖树清空所有上层衍生缓存；REST 与 gRPC 两条入口都要走同一封装。
3. **不可压制的安全地基**：`match_target="field_value"` 的确定性证据拥有绝对优先级，AI 仲裁只允许同级或升级。
4. **测试维度扩充**：断言内容而非仅 200；gRPC 零值用例；高维 DP 噪声统计检验；修复必须带回归测试（AGENTS.md 硬性要求）。
5. **文档即契约**：helm values / mkdocs nav / README 的每次代码漂移都要同步，CI 加 mkdocs build 与 validator 门禁。

---

## 6. 状态跟踪表

| 批次 | 内容 | 状态 |
|---|---|---|
| 第一轮（f04aee5） | P0 主干 21 项中的 14 项 | ✅ 已验证 |
| 第三轮 | 返工 R1-R7、回归 N1-N4、剩余 P0/P1/P2 | 🔄 进行中，完成后更新本节 |

> **注意**：v1.0.0 的"已全部修复并经全量测试验证"结论不准确——其验证仅覆盖 3 个既有测试文件（64 项），且 7 项修复经核验未修透、4 项为新引入问题。以本版（v2.0.0）为准。
