"""隐私原语性能基准测试 / Performance Benchmarks for Privacy Primitives.

中文说明：
使用 pytest-benchmark 插件测量核心隐私算子的吞吐量与延迟，覆盖：

1. 差分隐私 (DP) / Differential Privacy:
   - count: 10K 数据点计数加噪 / Count with 10K data points
   - sum: 带 clipping 的求和加噪 / Sum with clipping bounds
   - mean: 组合定理均值计算 / Mean via composition theorem
   - histogram: 5 类别直方图加噪 / 5-category histogram noise
   - vector_sum: 128 维向量求和 (DP-SGD) / 128-dim vector sum

2. 数据脱敏 (Masking) / Data Masking:
   - 单字段手机号/身份证 / Single field mobile/ID card
   - 100 条批量脱敏 / Batch of 100 records
   - 10 字段整记录脱敏 / Full record with 10 fields

3. K-匿名 (K-Anonymity) / K-Anonymity:
   - 100 行表级 Mondrian / 100-row table-level Mondrian

4. 查询混淆 (QOL) / Query Obfuscation:
   - 单条查询混淆 / Single query obfuscation
   - 10 条批量混淆 / Batch of 10 queries

运行方式 / How to Run:
    pytest tests/benchmark_primitives.py --benchmark-only
    pytest tests/benchmark_primitives.py --benchmark-compare

English Description:
Performance benchmarks using pytest-benchmark to measure throughput and latency
of core privacy operators: DP, masking, K-anonymity, and query obfuscation.
"""

import numpy as np
import pytest

from privacy_local_agent.privacy.dp import DPApi
from privacy_local_agent.privacy.kano import KAnonApi
from privacy_local_agent.privacy.masking import MaskingApi
from privacy_local_agent.privacy.qol import QolApi


@pytest.fixture
def dp_api():
    return DPApi()


@pytest.fixture
def masking_api():
    return MaskingApi()


@pytest.fixture
def kano_api():
    return KAnonApi()


@pytest.fixture
def qol_api():
    return QolApi()


# ── DP 基准测试 / DP Benchmarks ────────────────────────────────────


class TestDPBenchmarks:
    """差分隐私操作基准测试 / Differential Privacy Operation Benchmarks.

    测试数据规模：10,000 个数据点，使用固定种子 (42) 保证可重复性。
    关注指标：每次操作的平均延迟 (mean) 和每秒吐量 (ops/sec)。
    """

    def test_dp_count(self, dp_api, benchmark):
        """基准：10K 数据点 DP 计数（Laplace 机制，epsilon=1.0）。

        敏感度 = 1（增减一条记录最多改变计数 1），
        噪声尺度 b = 1/epsilon = 1.0。
        """
        values = np.random.default_rng(42).normal(50, 10, size=10000)
        result = benchmark(dp_api.count, values, epsilon=1.0)
        assert result is not None

    def test_dp_sum(self, dp_api, benchmark):
        """基准：带 clipping [0,100] 的 DP 求和。

        敏感度 = clip_upper - clip_lower = 100，
        噪声尺度 b = 100/epsilon = 100.0。
        """
        values = np.random.default_rng(42).normal(50, 10, size=10000)
        result = benchmark(
            dp_api.sum, values, epsilon=1.0, clip_lower=0.0, clip_upper=100.0
        )
        assert result is not None

    def test_dp_mean(self, dp_api, benchmark):
        """基准：DP 均值（组合定理：epsilon/2 给 count，epsilon/2 给 sum）。"""
        values = np.random.default_rng(42).normal(50, 10, size=10000)
        result = benchmark(
            dp_api.mean, values, epsilon=1.0, clip_lower=0.0, clip_upper=100.0
        )
        assert result is not None

    def test_dp_histogram(self, dp_api, benchmark):
        """基准：5 类别 DP 直方图（每个 bin 独立加噪）。"""
        rng = np.random.default_rng(42)
        categories = ["A", "B", "C", "D", "E"]
        values = rng.choice(categories, size=10000).tolist()
        result = benchmark(dp_api.histogram, values, categories=categories, epsilon=1.0)
        assert result is not None

    def test_dp_vector_sum(self, dp_api, benchmark):
        """基准：128 维向量 DP 求和 (DP-SGD 风格)。

        1000 个 128 维向量，max_norm=1.0 用于梯度裁剪，
        模拟联邦学习中 DP-SGD 的梯度聚合场景。
        """
        rng = np.random.default_rng(42)
        vectors = rng.normal(0, 1, size=(1000, 128))
        result = benchmark(dp_api.vector_sum, vectors, max_norm=1.0, epsilon=1.0)
        assert result is not None


# ── 脱敏基准测试 / Masking Benchmarks ─────────────────────────────────────────


class TestMaskingBenchmarks:
    """数据脱敏操作基准测试 / Data Masking Operation Benchmarks.

    脱敏操作为纯 CPU 字符串操作，无 I/O，延迟应稳定在微秒级。
    """

    def test_mask_mobile(self, masking_api, benchmark):
        """基准：单次手机号脱敏（保留前3后4，中间替换为 ****）。"""
        result = benchmark(masking_api.mask_value, "mobile", "13812345678", "")
        assert "****" in result

    def test_mask_id_card(self, masking_api, benchmark):
        """基准：单次身份证号脱敏（保留前6后4，中间 8 位替换）。"""
        result = benchmark(masking_api.mask_value, "id_card", "110105199001011234", "")
        assert "********" in result

    def test_mask_batch_100(self, masking_api, benchmark):
        """基准：100 条混合字段批量脱敏（mobile/id_card/name/email/address 各 20 条）。"""
        field_names = ["mobile", "id_card", "name", "email", "address"] * 20
        values = [
            "13812345678", "110105199001011234", "张三丰",
            "test@example.com", "北京市朝阳区建国路88号",
        ] * 20
        result = benchmark(masking_api.mask_batch, field_names, values, "")
        assert len(result) == 100

    def test_mask_record(self, masking_api, benchmark):
        """基准：10 字段整记录脱敏（模拟真实业务记录结构）。"""
        record = {
            "mobile": "13812345678",
            "id_card": "110105199001011234",
            "name": "张三丰",
            "email": "test@example.com",
            "address": "北京市朝阳区建国路88号",
            "bank_card": "6222021234567890123",
            "phone": "13998765432",
            "user_name": "李四",
            "mail": "user@domain.org",
            "addr": "上海市浦东新区陆家嘴环路1000号",
        }
        result = benchmark(masking_api.mask_record, record, "")
        assert len(result) == 10


# ── K-匿名基准测试 / K-Anonymity Benchmarks ─────────────────────────────────────


class TestKAnonBenchmarks:
    """数据集级 K-匿名基准测试 / Dataset-Level K-Anonymity Benchmarks.

    Mondrian 算法时间复杂度约为 O(n * d * log(n/k))，
    其中 n=记录数，d=QI 维度数。此测试用 100 行 3 维 QI 验证基准性能。
    """

    def test_kano_table_small(self, kano_api, benchmark):
        """基准：100 行表级 K-匿名（k=5, 3 个 QI 维度, max_depth=5）。"""
        rng = np.random.default_rng(42)
        rows = [
            {
                "age": int(rng.integers(20, 70)),
                "zipcode": f"{rng.integers(100000, 999999)}",
                "gender": rng.choice(["M", "F"]),
            }
            for _ in range(100)
        ]
        result = benchmark(
            kano_api.k_anonymize_table, rows, ["age", "zipcode", "gender"], k=5, max_depth=5
        )
        assert result is not None


# ── 查询混淆基准测试 / QoL Benchmarks ─────────────────────────────────────────────


class TestQolBenchmarks:
    """查询混淆基准测试 / Query Obfuscation Benchmarks.

    QOL 操作为纯内存随机采样 + 字符串拼接，延迟极低。
    主要关注批量操作的线性扩展性。
    """

    def test_obfuscate_single(self, qol_api, benchmark):
        """基准：单条医疗查询混淆（生成 3 个同类 dummy）。"""
        result = benchmark(
            qol_api.obfuscate_query,
            "糖尿病用药指南",
            num_dummies=3,
            domain="medical",
        )
        assert len(result) >= 1

    def test_obfuscate_batch_10(self, qol_api, benchmark):
        """基准：10 条医疗查询批量混淆（验证线性扩展性）。"""
        queries = [
            "糖尿病用药", "高血压治疗", "感冒药推荐",
            "抗生素使用", "疫苗接种", "体检项目",
            "中医调理", "营养补充", "康复训练", "心理咨询",
        ]
        result = benchmark(
            qol_api.obfuscate_query_batch,
            queries,
            num_dummies=3,
            domain="medical",
        )
        assert len(result) == 10
