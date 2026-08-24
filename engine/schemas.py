"""REST 请求模型集合（Pydantic）。

按域分组集中定义各端点的请求体模型，供 ``routers/*`` 子路由导入。
这些模型与 ``main.py`` 拆分前的定义保持完全一致，确保接口契约不变。
"""

from typing import Any

from pydantic import BaseModel, Field

# --------------------------------------------------------------------------- #
# 脱敏 / 哈希
# --------------------------------------------------------------------------- #


class MaskRequest(BaseModel):
    """单字段脱敏请求模型。"""

    field_name: str = Field(max_length=200)
    value: str = Field(max_length=100_000)
    context: str = Field(default="", max_length=10_000)


class MaskRecordRequest(BaseModel):
    """整记录脱敏请求模型。"""

    record: dict[str, str] = Field(max_length=200)
    context: str = Field(default="", max_length=10_000)


class MaskBatchRequest(BaseModel):
    """批量字段脱敏请求模型。"""

    field_names: list[str] = Field(max_length=200)
    values: list[str] = Field(max_length=10_000)
    context: str = Field(default="", max_length=10_000)


class MaskDataFrameRequest(BaseModel):
    """DataFrame 脱敏请求模型。

    data 为 records 列表（可来自 pandas/SecretFlow DataFrame 的转换）。
    columns 指定需要脱敏的列；未指定则对所有字符串列脱敏。
    """

    data: list[dict[str, Any]] = Field(max_length=1_000)
    columns: list[str] | None = Field(default=None, max_length=200)
    context: str = Field(default="", max_length=10_000)


class HashRequest(BaseModel):
    """HMAC 哈希请求模型。"""

    value: str = Field(max_length=100_000)
    salt: str = Field(max_length=1_000)


# --------------------------------------------------------------------------- #
# 差分隐私（DP）
# --------------------------------------------------------------------------- #


class DPRequest(BaseModel):
    """差分隐私聚合请求模型。

    values 为输入数据列表；params 为可选参数，用于覆盖默认或 profile 中的配置。
    """

    values: list[float] = Field(max_length=10_000)
    params: dict[str, object] = Field(default={}, max_length=100)


class DPHistogramRequest(BaseModel):
    """差分隐私直方图请求模型。"""

    values: list[str] = Field(max_length=10_000)
    categories: list[str] = Field(max_length=1_000)
    params: dict[str, object] = Field(default={}, max_length=100)


class DPNoisyCountRequest(BaseModel):
    """对已聚合计数进行 DP 加噪的请求模型。"""

    true_count: float = Field(description="真实聚合计数值")
    params: dict[str, object] = Field(default={}, max_length=100)


class DPNoisySumRequest(BaseModel):
    """对已聚合求和进行 DP 加噪的请求模型。

    params 中需提供 sensitivity，或同时提供 clip_lower 与 clip_upper。
    """

    true_sum: float = Field(description="真实聚合求和值")
    params: dict[str, object] = Field(default={}, max_length=100)


class DPNoisyMeanRequest(BaseModel):
    """对已聚合 sum/count 进行 DP 加噪得到均值的请求模型。"""

    true_sum: float = Field(description="真实聚合求和值")
    true_count: float = Field(description="真实聚合计数值")
    params: dict[str, object] = Field(default={}, max_length=100)


class DPNoisyHistogramRequest(BaseModel):
    """对已聚合直方图计数进行 DP 加噪的请求模型。"""

    true_counts: dict[str, float] = Field(max_length=1_000, description="各桶真实计数")
    params: dict[str, object] = Field(default={}, max_length=100)


class DPChunkedCountRequest(BaseModel):
    """分块流式 DP 计数请求模型。"""

    chunks: list[list[float]] = Field(max_length=1_000)
    params: dict[str, object] = Field(default={}, max_length=100)


class DPChunkedSumRequest(BaseModel):
    """分块流式 DP 求和请求模型。"""

    chunks: list[list[float]] = Field(max_length=1_000)
    params: dict[str, object] = Field(default={}, max_length=100)


class DPChunkedMeanRequest(BaseModel):
    """分块流式 DP 均值请求模型。"""

    chunks: list[list[float]] = Field(max_length=1_000)
    params: dict[str, object] = Field(default={}, max_length=100)


class DPAggregateRequest(BaseModel):
    """表格级原位 DP 聚合请求模型。"""

    rows: list[dict[str, Any]] = Field(max_length=1_000)
    specs: dict[str, Any] = Field(default={}, max_length=100)
    params: dict[str, object] = Field(default={}, max_length=100)


class DPVectorSumRequest(BaseModel):
    """高维向量 / 梯度 $L_2$ 范数截断加噪请求模型。"""

    vectors: list[list[float]] = Field(max_length=1_000)
    params: dict[str, object] = Field(default={}, max_length=100)


class DPAdaptiveClipRequest(BaseModel):
    """差分隐私自适应二分搜索估计上下界请求模型。"""

    values: list[float] = Field(max_length=10_000)
    params: dict[str, object] = Field(default={}, max_length=100)


class DPGroupByRequest(BaseModel):
    """Tau-Thresholding 差分隐私 SQL Group-By 请求模型。"""

    rows: list[dict[str, Any]] = Field(max_length=1_000)
    group_col: str = Field(max_length=200)
    target_col: str = Field(max_length=200)
    agg: str = Field(max_length=50)
    params: dict[str, object] = Field(default={}, max_length=100)


class DPChunkedHistogramRequest(BaseModel):
    """分块流式 DP 直方图请求模型。"""

    chunks: list[list[str]] = Field(max_length=1_000)
    categories: list[str] = Field(max_length=1_000)
    params: dict[str, object] = Field(default={}, max_length=100)


# --------------------------------------------------------------------------- #
# K-匿名
# --------------------------------------------------------------------------- #


class KAnonRequest(BaseModel):
    """K-匿名单条记录请求模型。"""

    record: dict[str, object] = Field(max_length=200)
    qi_cols: list[str] = Field(max_length=50)
    k: int = Field(default=5, ge=2, le=1000)


class KAnonTableRequest(BaseModel):
    """K-匿名整张表请求模型。"""

    rows: list[dict[str, object]] = Field(max_length=1_000)
    qi_cols: list[str] = Field(max_length=50)
    k: int = Field(default=5, ge=2, le=1000)
    max_depth: int = Field(default=10, ge=1, le=50)


class KAnonDataFrameRequest(BaseModel):
    """K-匿名 DataFrame 请求模型。

    data 为 records 列表（可来自 pandas/SecretFlow DataFrame）。
    """

    data: list[dict[str, Any]] = Field(max_length=1_000)
    qi_cols: list[str] = Field(max_length=50)
    k: int = Field(default=5, ge=2, le=1000)
    max_depth: int = Field(default=10, ge=1, le=50)


# --------------------------------------------------------------------------- #
# 查询混淆（QoL）
# --------------------------------------------------------------------------- #


class QolRequest(BaseModel):
    """查询混淆请求模型。"""

    query: str = Field(max_length=10_000)
    num_dummies: int = Field(default=3, ge=1, le=100)
    domain: str = Field(default="medical", max_length=100)
    medical_pool: list[str] | None = Field(default=None, max_length=1_000)
    generic_pool: list[str] | None = Field(default=None, max_length=1_000)
    seed: int | None = None


class QolBatchRequest(BaseModel):
    """批量查询混淆请求模型。"""

    queries: list[str] = Field(max_length=1_000)
    num_dummies: int = Field(default=3, ge=1, le=100)
    domain: str = Field(default="medical", max_length=100)
    medical_pool: list[str] | None = Field(default=None, max_length=1_000)
    generic_pool: list[str] | None = Field(default=None, max_length=1_000)
    seed: int | None = None


# --------------------------------------------------------------------------- #
# 本地差分隐私（LDP）
# --------------------------------------------------------------------------- #


class LdpPerturbBinaryRequest(BaseModel):
    """二值本地 DP 扰动请求模型。"""

    values: list[int] = Field(max_length=10_000)
    epsilon: float = Field(gt=0)


class LdpPerturbCategoricalRequest(BaseModel):
    """类别型本地 DP 扰动请求模型。"""

    values: list[str] = Field(max_length=10_000)
    categories: list[str] = Field(max_length=1_000)
    epsilon: float = Field(gt=0)


class LdpEstimateBinaryRequest(BaseModel):
    """二值本地 DP 估计请求模型。"""

    reported_values: list[int] = Field(max_length=10_000)
    epsilon: float = Field(gt=0)


class LdpEstimateCategoricalRequest(BaseModel):
    """类别型本地 DP 估计请求模型。"""

    reported_values: list[str] = Field(max_length=10_000)
    categories: list[str] = Field(max_length=1_000)
    epsilon: float = Field(gt=0)


# --------------------------------------------------------------------------- #
# 隐私参数推荐
# --------------------------------------------------------------------------- #


class RecommendRequest(BaseModel):
    """隐私参数推荐请求模型。"""

    namespace: str = Field(max_length=200)
    values: list[float] | None = Field(default=None, max_length=10_000)
    rows: list[dict[str, object]] | None = Field(default=None, max_length=1_000)
    qi_cols: list[str] | None = Field(default=None, max_length=50)


# --------------------------------------------------------------------------- #
# 缩写规范与语义增强别名（保证 PEP 8 专有名词全大写规范与向后兼容）
# --------------------------------------------------------------------------- #

LDPPerturbBinaryRequest = LdpPerturbBinaryRequest
LDPPerturbCategoricalRequest = LdpPerturbCategoricalRequest
LDPEstimateBinaryRequest = LdpEstimateBinaryRequest
LDPEstimateCategoricalRequest = LdpEstimateCategoricalRequest

QOLRequest = QolRequest
QOLBatchRequest = QolBatchRequest
QueryObfuscationRequest = QolRequest
QueryObfuscationBatchRequest = QolBatchRequest

KAnonymityRequest = KAnonRequest
KAnonymityTableRequest = KAnonTableRequest
KAnonymityDataFrameRequest = KAnonDataFrameRequest

