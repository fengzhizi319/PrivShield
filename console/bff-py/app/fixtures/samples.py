"""PrivShield 所有 REST 端点的示例请求载荷。
Sample request payloads for all PrivShield REST endpoints.

示例数据刻意保持**最小化与确定性**：只用于验证连通性、展示合法的
请求形状。用户可以在 UI 中编辑后再发送。
Sample data is intentionally **minimal and deterministic**: used only to verify
connectivity and demonstrate valid request shapes. Users can edit in the UI before sending.

每个示例由 :class:`EndpointSample` 描述，含：请求方法 / 路径 / 展示标签 /
功能分类 / 描述 / 默认请求体等。``backend`` 字段标识该端点在哪个后端
可用（``rest`` 仅 Python REST 后端，``both`` 两个后端都支持）。
Each sample is described by :class:`EndpointSample`, containing: HTTP method / path /
display label / category / description / default body, etc. The ``backend`` field
indicates availability (``rest`` = Python REST backend only, ``both`` = both backends).
"""

from __future__ import annotations

import base64
from typing import Any


def _arrow_ipc_payload() -> str:
    """生成一个小型 Arrow IPC 流并返回其 base64 字符串。
    Generate a small Arrow IPC stream and return its base64 string.

    用于 ``/v1/privacy/dp/arrow_ipc`` 端点的示例：该端点要求二进制
    Arrow 流作为输入，这里构造一个 5 行的小表并编码为 base64，
    由前端经 ``rawPayloadB64`` 字段传递、后端解码后转发。
    Used for the ``/v1/privacy/dp/arrow_ipc`` endpoint sample: this endpoint requires
    a binary Arrow stream as input. Here we construct a 5-row table encoded as base64,
    passed by the frontend via ``rawPayloadB64`` field, decoded and forwarded by the backend.
    """
    # 延迟导入 pyarrow：仅在生成 Arrow 示例时才引入重量级依赖。
    # Lazy-import pyarrow: only pull in the heavyweight dependency when generating Arrow sample.
    import io

    import pyarrow as pa

    # 构造一个 5 行单列（value）的小表。
    # Construct a small 5-row single-column (value) table.
    table = pa.table({"value": [1.0, 2.0, 3.0, 4.0, 5.0]})
    # 创建内存缓冲区作为 Arrow IPC 流的写入目标。
    # Create an in-memory buffer as the write target for the Arrow IPC stream.
    sink = io.BytesIO()
    # 以流式格式写入整张表（with 块结束时自动写入流尾）。
    # Write the entire table in streaming format (stream footer auto-written on with-block exit).
    with pa.ipc.new_stream(sink, table.schema) as writer:
        writer.write_table(table)
    # 把二进制流编码为 base64 字符串，供前端经 rawPayloadB64 传递。
    # Encode the binary stream as a base64 string for frontend delivery via rawPayloadB64.
    return base64.b64encode(sink.getvalue()).decode("ascii")


class EndpointSample:
    """单个 PrivShield 端点的元数据与示例载荷。
    Metadata and sample payload for a single PrivShield endpoint.

    属性说明 / Attribute Descriptions：
        - ``method`` / ``path``：HTTP 方法与端点路径 / HTTP method and endpoint path;
        - ``label``：UI 中显示的简短名称 / short name displayed in UI;
        - ``category``：功能分类（用于侧边栏分组，如 Masking / DP）/ category (for sidebar grouping);
        - ``description``：中文功能描述 / Chinese functional description;
        - ``body``：默认 JSON 请求体（可为空）/ default JSON request body (nullable);
        - ``content_type`` / ``raw_payload_b64``：二进制载荷场景使用 / used for binary payload scenarios;
        - ``backend``：可用性标识（``rest`` / ``both``）/ availability flag.
    """

    def __init__(
        self,
        method: str,
        path: str,
        label: str,
        category: str,
        description: str,
        body: dict[str, Any] | None = None,
        content_type: str | None = None,
        raw_payload_b64: str | None = None,
        backend: str = "rest",
    ):
        # 逐一保存端点的各项元数据与示例载荷。
        # Store each piece of endpoint metadata and sample payload.
        self.method = method                  # HTTP 方法 / HTTP method
        self.path = path                      # 端点路径 / endpoint path
        self.label = label                    # UI 展示名称 / UI display name
        self.category = category              # 功能分类（侧边栏分组）/ category (sidebar grouping)
        self.description = description        # 中文功能描述 / Chinese description
        self.body = body                      # 默认 JSON 请求体 / default JSON request body
        self.content_type = content_type      # 二进制载荷的 Content-Type / binary payload Content-Type
        self.raw_payload_b64 = raw_payload_b64  # 二进制载荷的 base64 / binary payload base64
        self.backend = backend                # 可用性标识（rest / both）/ availability flag

    def to_dict(self) -> dict[str, Any]:
        # 转换为前端可直接消费的字典；注意 contentType / rawPayloadB64
        # 使用驼峰命名（与前端 TypeScript 契约一致）。
        # Convert to a dict directly consumable by the frontend; note contentType / rawPayloadB64
        # use camelCase naming (consistent with frontend TypeScript contract).
        return {
            "method": self.method,
            "path": self.path,
            "label": self.label,
            "category": self.category,
            "description": self.description,
            "body": self.body,
            "contentType": self.content_type,
            "rawPayloadB64": self.raw_payload_b64,
            "backend": self.backend,
        }


# fmt: off
SAMPLES: list[EndpointSample] = [
    # Health
    EndpointSample("GET", "/health", "Health", "Health", "服务健康检查", backend="rest"),
    EndpointSample("GET", "/livez", "Livez", "Health", "存活探针", backend="rest"),
    EndpointSample("GET", "/readyz", "Readyz", "Health", "就绪探针", backend="rest"),
    EndpointSample("GET", "/readyz/llm", "LLM Ready", "Health", "LLM 分类器就绪探针", backend="rest"),

    # Masking
    EndpointSample(
        "POST", "/v1/privacy/mask", "Mask", "Masking",
        "单字段脱敏",
        body={"field_name": "email", "value": "alice@example.com", "context": ""},
        backend="both",
    ),
    EndpointSample(
        "POST", "/v1/privacy/mask_record", "Mask Record", "Masking",
        "整条记录脱敏",
        body={
            "record": {
                "email": "alice@example.com",
                "phone": "13800138000",
                "name": "Alice",
                "id_card": "11010119900101XXXX",
            },
            "context": "",
        },
        backend="rest",
    ),
    EndpointSample(
        "POST", "/v1/privacy/mask/batch", "Mask Batch", "Masking",
        "批量字段脱敏",
        body={
            "field_names": ["email", "phone", "name"],
            "values": ["bob@example.com", "13900139000", "Bob"],
            "context": "",
        },
        backend="rest",
    ),
    EndpointSample(
        "POST", "/v1/privacy/mask/dataframe", "Mask DataFrame", "Masking",
        "DataFrame 脱敏",
        body={
            "data": [
                {"email": "alice@example.com", "phone": "13800138000"},
                {"email": "bob@example.com", "phone": "13900139000"},
            ],
            "columns": ["email", "phone"],
            "context": "",
        },
        backend="rest",
    ),
    EndpointSample(
        "POST", "/v1/privacy/hash", "Hash", "Hash",
        "HMAC 哈希",
        body={"value": "sensitive-value", "salt": "demo-salt"},
        backend="both",
    ),

    # DP
    EndpointSample(
        "POST", "/v1/privacy/dp/count", "DP Count", "DP",
        "差分隐私计数",
        body={"values": [1.0, 2.0, 3.0, 4.0, 5.0], "params": {"epsilon": 0.1, "mechanism": "laplace"}},
        backend="both",
    ),
    EndpointSample(
        "POST", "/v1/privacy/dp/sum", "DP Sum", "DP",
        "差分隐私求和",
        body={
            "values": [1000.0, 2000.0, 3000.0, 4000.0, 5000.0],
            "params": {"epsilon": 0.1, "mechanism": "laplace", "clip_lower": 0.0, "clip_upper": 10000.0},
        },
        backend="rest",
    ),
    EndpointSample(
        "POST", "/v1/privacy/dp/mean", "DP Mean", "DP",
        "差分隐私均值",
        body={
            "values": [20.0, 30.0, 40.0, 50.0, 60.0],
            "params": {"epsilon": 0.1, "mechanism": "laplace", "clip_lower": 0.0, "clip_upper": 100.0},
        },
        backend="rest",
    ),
    EndpointSample(
        "POST", "/v1/privacy/dp/histogram", "DP Histogram", "DP",
        "差分隐私直方图",
        body={
            "values": ["eng", "hr", "eng", "sales", "eng"],
            "categories": ["eng", "hr", "sales", "marketing"],
            "params": {"epsilon": 0.1, "mechanism": "laplace"},
        },
        backend="both",
    ),
    EndpointSample(
        "POST", "/v1/privacy/dp/noisy_count", "Noisy Count", "DP",
        "对已聚合计数加噪",
        body={"true_count": 100.0, "params": {"epsilon": 0.1, "mechanism": "laplace"}},
        backend="both",
    ),
    EndpointSample(
        "POST", "/v1/privacy/dp/noisy_sum", "Noisy Sum", "DP",
        "对已聚合求和加噪",
        body={
            "true_sum": 10000.0,
            "params": {"epsilon": 0.1, "mechanism": "laplace", "clip_lower": 0.0, "clip_upper": 10000.0},
        },
        backend="rest",
    ),
    EndpointSample(
        "POST", "/v1/privacy/dp/noisy_mean", "Noisy Mean", "DP",
        "对已聚合均值加噪",
        body={
            "true_sum": 10000.0,
            "true_count": 100.0,
            "params": {"epsilon": 0.1, "mechanism": "laplace", "clip_lower": 0.0, "clip_upper": 10000.0},
        },
        backend="rest",
    ),
    EndpointSample(
        "POST", "/v1/privacy/dp/noisy_histogram", "Noisy Histogram", "DP",
        "对已聚合直方图加噪",
        body={
            "true_counts": {"eng": 50.0, "hr": 20.0, "sales": 30.0},
            "params": {"epsilon": 0.1, "mechanism": "laplace"},
        },
        backend="both",
    ),
    EndpointSample(
        "POST", "/v1/privacy/dp/aggregate", "DP Aggregate", "DP",
        "表格级原位 DP 聚合",
        body={
            "rows": [
                {"age": 20, "salary": 1000.0, "dept": "eng"},
                {"age": 30, "salary": 2000.0, "dept": "hr"},
                {"age": 40, "salary": 3000.0, "dept": "eng"},
                {"age": 50, "salary": 4000.0, "dept": "sales"},
            ],
            "specs": {
                "age": ["mean", {"clip_lower": 0, "clip_upper": 100}],
                "salary": ["sum", {"clip_lower": 0, "clip_upper": 10000}],
                "dept": ["histogram", {"categories": ["eng", "hr", "sales"]}],
            },
            "params": {"epsilon": 0.5, "mechanism": "laplace"},
        },
        backend="both",
    ),
    EndpointSample(
        "POST", "/v1/privacy/dp/vector_sum", "DP Vector Sum", "DP",
        "高维向量 DP 求和",
        body={
            "vectors": [[1.0, 2.0], [3.0, 4.0], [5.0, 6.0]],
            "params": {"epsilon": 0.1, "delta": 1e-5, "mechanism": "gaussian", "max_norm": 10.0},
        },
        backend="rest",
    ),
    EndpointSample(
        "POST", "/v1/privacy/dp/vector_mean", "DP Vector Mean", "DP",
        "高维向量 DP 均值",
        body={
            "vectors": [[1.0, 2.0], [3.0, 4.0], [5.0, 6.0]],
            "params": {"epsilon": 0.1, "delta": 1e-5, "mechanism": "gaussian", "max_norm": 10.0},
        },
        backend="rest",
    ),
    EndpointSample(
        "POST", "/v1/privacy/dp/adaptive_clip", "Adaptive Clip", "DP",
        "自适应二分搜索估计截断上下界",
        body={
            "values": [1.0, 5.0, 10.0, 15.0, 20.0],
            "params": {"epsilon": 0.1, "target_quantile": 0.95, "num_iterations": 15, "initial_clip": 10.0},
        },
        backend="rest",
    ),
    EndpointSample(
        "POST", "/v1/privacy/dp/groupby", "DP GroupBy", "DP",
        "Tau-Thresholding 差分隐私 Group-By",
        body={
            "rows": [
                {"dept": "eng", "salary": 1000.0},
                {"dept": "hr", "salary": 2000.0},
                {"dept": "eng", "salary": 3000.0},
                {"dept": "sales", "salary": 4000.0},
            ],
            "group_col": "dept",
            "target_col": "salary",
            "agg": "sum",
            "params": {"epsilon": 0.1, "mechanism": "laplace", "clip_lower": 0.0, "clip_upper": 10000.0},
        },
        backend="both",
    ),
    EndpointSample(
        "POST", "/v1/privacy/dp/chunked_count", "Chunked Count", "DP",
        "分块流式 DP 计数",
        body={
            "chunks": [[1.0, 2.0], [3.0, 4.0], [5.0, 6.0]],
            "params": {"epsilon": 0.1, "mechanism": "laplace"},
        },
        backend="rest",
    ),
    EndpointSample(
        "POST", "/v1/privacy/dp/chunked_sum", "Chunked Sum", "DP",
        "分块流式 DP 求和",
        body={
            "chunks": [[1.0, 2.0], [3.0, 4.0], [5.0, 6.0]],
            "params": {"epsilon": 0.1, "mechanism": "laplace", "clip_lower": 0.0, "clip_upper": 10.0},
        },
        backend="rest",
    ),
    EndpointSample(
        "POST", "/v1/privacy/dp/chunked_mean", "Chunked Mean", "DP",
        "分块流式 DP 均值",
        body={
            "chunks": [[1.0, 2.0], [3.0, 4.0], [5.0, 6.0]],
            "params": {"epsilon": 0.1, "mechanism": "laplace", "clip_lower": 0.0, "clip_upper": 10.0},
        },
        backend="rest",
    ),
    EndpointSample(
        "POST", "/v1/privacy/dp/chunked_histogram", "Chunked Histogram", "DP",
        "分块流式 DP 直方图",
        body={
            "chunks": [["eng", "hr"], ["eng", "sales"], ["eng", "marketing"]],
            "categories": ["eng", "hr", "sales", "marketing"],
            "params": {"epsilon": 0.1, "mechanism": "laplace"},
        },
        backend="rest",
    ),
    EndpointSample(
        "POST", "/v1/privacy/dp/arrow_ipc", "Arrow IPC", "DP",
        "Arrow IPC 二进制 DP 聚合",
        body={"column": "value", "aggregation": "count", "epsilon": 0.1, "delta": 0.0, "mechanism": "laplace"},
        content_type="application/vnd.apache.arrow.stream",
        raw_payload_b64=_arrow_ipc_payload(),
        backend="rest",
    ),

    # LDP
    EndpointSample(
        "POST", "/v1/privacy/ldp/perturb/binary", "Perturb Binary", "LDP",
        "二值本地 DP 扰动",
        body={"values": [0, 1, 1, 0, 1], "epsilon": 1.0},
        backend="both",
    ),
    EndpointSample(
        "POST", "/v1/privacy/ldp/perturb/categorical", "Perturb Categorical", "LDP",
        "类别型本地 DP 扰动",
        body={"values": ["eng", "hr", "eng", "sales"], "categories": ["eng", "hr", "sales"], "epsilon": 1.0},
        backend="both",
    ),
    EndpointSample(
        "POST", "/v1/privacy/ldp/estimate/binary", "Estimate Binary", "LDP",
        "二值本地 DP 估计",
        body={"reported_values": [0, 1, 1, 0, 1], "epsilon": 1.0},
        backend="both",
    ),
    EndpointSample(
        "POST", "/v1/privacy/ldp/estimate/categorical", "Estimate Categorical", "LDP",
        "类别型本地 DP 估计",
        body={
            "reported_values": ["eng", "hr", "eng", "sales"],
            "categories": ["eng", "hr", "sales"],
            "epsilon": 1.0,
        },
        backend="both",
    ),

    # K-Anonymity
    EndpointSample(
        "POST", "/v1/privacy/k_anonymize/record", "K-Anonymize Record", "K-Anonymity",
        "单条记录 K-匿名泛化",
        body={
            "record": {"age": "30", "zip": "100000", "gender": "F"},
            "qi_cols": ["age", "zip", "gender"],
            "k": 2,
        },
        backend="both",
    ),
    EndpointSample(
        "POST", "/v1/privacy/k_anonymize/table", "K-Anonymize Table", "K-Anonymity",
        "整张表 K-匿名泛化",
        body={
            "rows": [
                {"age": "30", "zip": "100000", "gender": "F"},
                {"age": "31", "zip": "100001", "gender": "F"},
                {"age": "32", "zip": "100002", "gender": "M"},
                {"age": "33", "zip": "100003", "gender": "M"},
            ],
            "qi_cols": ["age", "zip", "gender"],
            "k": 2,
            "max_depth": 10,
        },
        backend="both",
    ),
    EndpointSample(
        "POST", "/v1/privacy/k_anonymize/dataframe", "K-Anonymize DataFrame", "K-Anonymity",
        "DataFrame K-匿名泛化",
        body={
            "data": [
                {"age": "30", "zip": "100000", "gender": "F"},
                {"age": "31", "zip": "100001", "gender": "F"},
                {"age": "32", "zip": "100002", "gender": "M"},
                {"age": "33", "zip": "100003", "gender": "M"},
            ],
            "qi_cols": ["age", "zip", "gender"],
            "k": 2,
            "max_depth": 10,
        },
        backend="both",
    ),

    # Query Obfuscation
    EndpointSample(
        "POST", "/v1/privacy/qol/obfuscate", "Obfuscate Query", "Query Obfuscation",
        "查询混淆",
        body={"query": "糖尿病患者用药推荐", "num_dummies": 3, "domain": "medical"},
        backend="both",
    ),
    EndpointSample(
        "POST", "/v1/privacy/qol/obfuscate/batch", "Obfuscate Batch", "Query Obfuscation",
        "批量查询混淆",
        body={
            "queries": ["糖尿病患者用药推荐", "高血压患者饮食建议"],
            "num_dummies": 3,
            "domain": "medical",
        },
        backend="both",
    ),

    # Profile（隐私参数推荐）
    # 与 Go 后端 samples 对齐：agent REST 契约的 rows 为扁平 dict（非 gRPC 的 fields 包装）。
    EndpointSample(
        "POST", "/v1/privacy/profile/recommend", "Recommend Params", "Profile",
        "自动推荐隐私参数",
        body={
            "namespace": "demo-recommend",
            "values": [1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0],
            "rows": [
                {"age": "30", "zip": "100000", "gender": "F"},
                {"age": "31", "zip": "100001", "gender": "M"},
            ],
            "qi_cols": ["age", "zip", "gender"],
        },
        backend="both",
    ),

    # Dynamic Classification
    EndpointSample(
        "POST", "/v1/dynclassification/eval", "Dynamic Classification Eval", "DynamicClassification",
        "声明式动态分类分级评估",
        body={"fieldName": "mobile_phone", "value": "13800138000", "domain": "general-pii"},
        backend="both",
    ),
    EndpointSample(
        "POST", "/v1/dynclassification/eval_record", "Dynamic Classification Eval Record", "DynamicClassification",
        "记录级动态分类分级（逐字段）",
        body={"record": {"name": "张三", "id_card": "110101199001011237", "phone": "13800138000"}},
        backend="both",
    ),
    EndpointSample(
        "POST", "/v1/dynclassification/generate_profile", "Auto Generate Profiles", "DynamicClassification",
        "从标准 Markdown 文档生成 YAML 配置",
        body={"docPath": "docs/standard/四川省健康医疗大数据应用指南.md"},
        backend="both",
    ),
    EndpointSample("GET", "/v1/dynclassification/standards", "List Standards", "DynamicClassification", "列出所有分类分级标准", backend="both"),
    EndpointSample("GET", "/v1/dynclassification/domains", "List Domains", "DynamicClassification", "列出所有领域匹配包", backend="both"),
    EndpointSample("GET", "/v1/dynclassification/operators", "List Operators", "DynamicClassification", "列出所有已注册匹配算子", backend="both"),
    EndpointSample("POST", "/v1/dynclassification/validate", "Validate Rule Profiles", "DynamicClassification", "校验 YAML 配置合法性", backend="both"),
]

# fmt: on


def get_samples() -> list[dict[str, Any]]:
    """返回所有端点示例（纯字典列表），供 ``/api/samples`` 接口序列化。
    Return all endpoint samples (list of dicts) for ``/api/samples`` API serialization.
    """
    # 把每个 EndpointSample 对象转换为字典。
    # Convert each EndpointSample object to a dict.
    return [s.to_dict() for s in SAMPLES]


def find_sample(path: str) -> dict[str, Any] | None:
    """按端点路径查找示例，未找到时返回 ``None``。
    Look up a sample by endpoint path; return ``None`` if not found.
    """
    # 线性遍历 SAMPLES，按 path 精确匹配。
    # Linear scan over SAMPLES, exact match by path.
    for s in SAMPLES:
        if s.path == path:
            return s.to_dict()
    # 未找到匹配的示例。
    # No matching sample found.
    return None
