# K-匿名与多维空间划分泛化算法技术指南 / K-Anonymity & Multidimensional Generalization Technical Guide

## 1. 技术简介 / Introduction

**K-匿名（K-Anonymity）** 是由 Pierangela Samarati 和 Latanya Sweeney 于 1998 年提出的经典数据发布隐私保护模型。其核心思想是：在公开发布的数据集中，任何一条个体记录的**准标识符（Quasi-Identifiers, QI）** 取值组合，至少与数据集中其他 $k-1$ 条记录完全相同，从而形成大小至少为 $k$ 的**等价类（Equivalence Class）**。

这意味着任何外部攻击者即便掌握了外部辅助知识库（如选民登记册、公开社保名单），也无法以高于 $1/k$ 的概率将发布数据中的某一行重新关联（Re-identification）到具体自然人。

### 1.1 关键概念定义 / Core Concepts

1. **直接标识符（Direct Identifiers）**：能唯一定位个体的字段（如身份证号、姓名、手机号），在数据发布前必须 100% 剥离或强哈希脱敏。
2. **准标识符（Quasi-Identifiers, QI）**：单个字段无法唯一识别个体，但多个字段组合起来具有高度唯一性的属性集（如 `[年龄, 性别, 邮编, 职业]`）。研究表明，在美国仅通过 `[ZIP Code, Gender, Date of Birth]` 组合即可唯一识别 87% 的人口。
3. **敏感属性（Sensitive Attributes, SA）**：需要保护的核心机密信息（如疾病诊断、薪资收入、信用评级）。
4. **等价类（Equivalence Class, EC）**：数据集中所有准标识符属性完全相同的一组记录子集。若所有等价类的记录数均满足 $|EC_i| \ge k$，则该数据集满足 $k$-匿名。
5. **扩展模型**：
   - **$l$-多样性（$l$-Diversity）**：要求每个等价类中的敏感属性至少包含 $l$ 个不同取值，防御齐次性攻击（Homogeneity Attack）。
   - **$t$-紧密性（$t$-Closeness）**：要求每个等价类中敏感属性的概率分布与全局分布的距离（通常用 Wasserstein / Earth Mover's Distance 衡量）不超过阈值 $t$。

---

## 2. 在本项目中的用法 / Usage in This Project

`PrivShield` 在 [`engine/privacy/kano.py`](file:///home/charles/code/PrivShield/engine/privacy/kano.py) 中提供了两套互补的 K-匿名能力：
1. **面向流式/单记录处理的启发式自适应层级泛化（Record-Level Adaptive Hierarchy）**
2. **面向批量数据集的 Mondrian 多维空间中位数划分算法（Mondrian Multidimensional Partitioning）**

```text
               输入记录 / 数据集 (Records / Dataset)
                         │
                         ├── 单记录 / 流式场景 ──► 自适应分段启发式泛化 (Adaptive Hierarchy)
                         │                        - 年龄自适应分段 (<60 粗区间 / >=60 细区间)
                         │                        - 邮编前缀抑制 (Prefix suppression)
                         │                        - 学历/职业分类树向上合并
                         │
                         └── 批量表格场景 ───────► Mondrian 多维空间划分算法 (Mondrian Partitioning)
                                                  - 跨多维 QI 构建 KD-Tree
                                                  - 动态计算最优切分维度 (Max normalized span)
                                                  - 中位数二分切分 (Median split)
                                                  - 递归终止条件 (|Partition| < 2k)
                                                  - 等价类合并与区间/类别泛化
```

---

### 2.1 单记录自适应分段泛化 / Single-Record Adaptive Generalization

文件 / File：[`engine/privacy/kano.py`](file:///home/charles/code/PrivShield/engine/privacy/kano.py#L150-L240)

在医疗健康与养老照护业务中，若对所有年龄统一采用粗粒度区间（例如统一 10 岁一组），会导致 60 岁以上老年慢性病风险评估精度严重受损。`PrivShield` 创新实现了 `adaptive_age_hierarchy`：

```python
def adaptive_age_hierarchy(
    value: str | int | float,
    under_60_interval: int = 3,
    senior_interval: int = 2,
    output_format: str = "floor",
) -> str:
    """单条记录自适应分段年龄泛化函数。
    
    1. < 60 岁：年龄精细度对健康建议影响较小，采用较粗区间（默认 3 岁或 5 岁区间）；
       计算公式：start = age - (age % under_60_interval)
    2. >= 60 岁：考虑康养与慢性病高敏因素，采用精细区间（2 岁区间）；
       计算公式：start = age - (age % senior_interval)
    """
    clean_val = str(value).rstrip("岁").strip()
    age = int(float(clean_val))

    if age < 60:
        interval = max(1, under_60_interval)
    else:
        interval = max(1, senior_interval)

    start = age - (age % interval)
    if output_format == "range":
        end = start + interval - 1
        return f"[{start}-{end}]" if interval > 1 else str(start)
    return str(start)
```

#### 内置准标识符泛化层级表 (Generalization Hierarchies)

| QI 类型 | Level 0 (原始) | Level 1 (轻度) | Level 2 (中度) | Level 3 (深度) | Level 4+ (抑制) |
|---|---|---|---|---|---|
| **年龄 (Age)** | `28` | `[25-30]` (5岁区间) | `[20-30]` (10岁区间) | `[20-40]` (20岁区间) | `*` |
| **邮编 (ZipCode)** | `610041` | `610***` (前3位) | `61****` (前2位) | `6*****` (前1位) | `*` |
| **薪资 (Salary)** | `23` | `[20K-25K]` (5K) | `[20K-30K]` (10K) | `[0K-50K]` (50K) | `*` |
| **学历 (Education)** | `硕士` | `高等教育` | `*` | `*` | `*` |
| **性别 (Gender)** | `男` / `女` | `*` | `*` | `*` | `*` |

---

### 2.2 批量数据集 Mondrian 算法实现 / Mondrian Algorithm Deep-Dive

文件 / File：[`engine/privacy/kano.py`](file:///home/charles/code/PrivShield/engine/privacy/kano.py)

Mondrian 算法是一种贪心自顶向下的多维空间划分算法。它将多维 QI 数据空间视为一个超矩形，递归寻找归一化跨度最大的维度沿中位数进行切分，直至子空间样本数无法再被二分（$< 2k$）：

```python
class MondrianAnonymizer:
    """Mondrian 多维空间划分 K-匿名化引擎。"""
    
    def __init__(self, k: int = 3, qi_columns: list[str] | None = None):
        self.k = max(2, k)
        self.qi_columns = qi_columns or []

    def _choose_dimension(self, partition: list[dict[str, Any]]) -> str:
        """选择归一化取值跨度（Normalized Span）最大的维度作为切分轴。"""
        max_span = -1.0
        best_dim = self.qi_columns[0]

        for dim in self.qi_columns:
            vals = [row[dim] for row in partition if row[dim] is not None]
            if not vals:
                continue
            if isinstance(vals[0], (int, float)):
                # 数值型：(max - min) / global_scale
                span = float(max(vals) - min(vals))
            else:
                # 类别型：唯一样本数 / 总体基数
                span = float(len(set(vals)))
            if span > max_span:
                max_span = span
                best_dim = dim
        return best_dim

    def _split_partition(
        self, partition: list[dict[str, Any]], dim: str
    ) -> tuple[list[dict[str, Any]], list[dict[str, Any]]] | None:
        """沿选中维度的中位数执行空间二分切分。"""
        if len(partition) < 2 * self.k:
            return None  # 无法再拆分出两个 >= k 的子等价类

        sorted_part = sorted(partition, key=lambda r: (r[dim] is None, r[dim]))
        mid = len(sorted_part) // 2

        left = sorted_part[:mid]
        right = sorted_part[mid:]

        # 严格检查：左右分区大小均必须 >= k
        if len(left) < self.k or len(right) < self.k:
            return None
        return left, right

    def anonymize(self, dataset: list[dict[str, Any]]) -> list[dict[str, Any]]:
        """递归执行划分并对终态等价类进行全字段区间泛化。"""
        partitions = [dataset]
        final_partitions = []

        while partitions:
            part = partitions.pop(0)
            dim = self._choose_dimension(part)
            split_res = self._split_partition(part, dim)
            if split_res is None:
                final_partitions.append(part)
            else:
                left, right = split_res
                partitions.extend([left, right])

        # 对每个等价类应用区间/集合泛化
        anonymized_data = []
        for eq_class in final_partitions:
            summary = self._generalize_equivalence_class(eq_class)
            for row in eq_class:
                anonymized_row = dict(row)
                anonymized_row.update(summary)
                anonymized_data.append(anonymized_row)
        return anonymized_data
```

---

### 2.3 信息损失度（Information Loss Metrics）度量

泛化操作不可避免地会引入数据精度损失。`PrivShield` 提供了标准化确定性惩罚（Normalized Certainty Penalty, NCP）与可辨识性度量（Discernibility Metric, DM）：

$$\text{NCP}(R) = \sum_{i=1}^{d} \frac{|I_i|}{|MAX_i - MIN_i|}$$
其中 $|I_i|$ 为等价类在第 $i$ 个维度的泛化区间宽度。NCP 越低，数据可用性与效用越高。

---

## 3. 生产最佳实践与安全权衡 / Best Practices

1. **维度灾难（Curse of Dimensionality）**：
   - 准标识符列数越多（如 $> 10$ 列），由于数据在高维空间中高度稀疏，Mondrian 算法将被迫将大量区间泛化为全集或 `*` 导致数据效用骤降。
   - **最佳实践**：通过字段敏感度探查（`services/datasource-mgr`），仅将高关联重识别风险的 3~5 个字段设为 QI。
2. **结合差分隐私与 K-匿名**：
   - K-匿名保护的是单条记录发布的抗重识别性；差分隐私保护的是聚合统计查询。
   - 生产环境中：**微观明细数据共享采用 Mondrian K-匿名**；**宏观统计分析与报表采用 DP 机制**。
