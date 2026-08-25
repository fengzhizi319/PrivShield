# 差分隐私与本地差分隐私技术指南 / Differential Privacy & Local DP Technical Guide

## 1. 技术简介 / Introduction

**差分隐私（Differential Privacy, DP）** 是密码学与理论计算机科学中被公认为最强、具备严格数学证明的隐私保护标准。它由 Cynthia Dwork 等人在 2006 年提出，核心目标是在统计分析或机器学习中，使攻击者即便拥有任意辅助背景知识，也无法从计算结果中确定任意单一特定个体的记录是否存在于原始数据集中。

### 1.1 核心数学定义 / Mathematical Foundation

设 $D$ 和 $D'$ 为仅相差一条单条记录的任意两个相邻数据集（即汉明距离 $\|D \Delta D'\| = 1$），$M$ 为随机化算法（机制）：

- **纯差分隐私（$\epsilon$-DP / Pure DP）**：对于任意输出子集 $S \subseteq \text{Range}(M)$，满足：
  $$\mathbb{P}[M(D) \in S] \le e^{\epsilon} \cdot \mathbb{P}[M(D') \in S]$$
  其中 $\epsilon > 0$ 为隐私损失预算（Privacy Budget）。$\epsilon$ 越小，隐私保护越强，数据可用性（效用）越低。

- **近似差分隐私（$(\epsilon, \delta)$-DP / Approximate DP）**：
  $$\mathbb{P}[M(D) \in S] \le e^{\epsilon} \cdot \mathbb{P}[M(D') \in S] + \delta$$
  其中 $\delta \in [0, 1)$ 为隐私泄露的失效概率（通常要求 $\delta \ll 1/|D|$）。

- **本地差分隐私（Local DP, LDP）**：在数据离开用户终端前由客户端本地进行随机扰动，数据收集者或中心服务器从头到尾无法触碰明文数据。

---

## 2. 在本项目中的用法 / Usage in This Project

`PrivShield` 在 `engine/privacy/` 中实现了完备的集中式与本地差分隐私算子、动态预算记账、Rényi 差分隐私（RDP）复合计算及防篡改审计。

```text
                                客户端请求 / Client Request
                                            │
                                            ▼
                           FastAPI Router / gRPC Servicer
                       (/v1/privacy/dp/*, /v1/privacy/ldp/*)
                                            │
                                            ▼
                 ┌─────────────────────────────────────────────────────┐
                 │ ★ BudgetAccountant / BudgetRegistry (budget.py)    │
                 │   - 命名空间预算申请 (Namespace spend check)         │
                 │   - 并发扣减与时间窗口重置 (Time-window reset)      │
                 │   - HMAC-SHA256 不可篡改审计日志存证               │
                 │   - 超额抛出 PrivacyBudgetExhaustedError 拦截       │
                 └──────────────────────────┬──────────────────────────┘
                                            │ [预算扣减成功]
                                            ▼
                 ┌─────────────────────────────────────────────────────┐
                 │ ★ PrivacyService / DP Engines (dp.py, ldp.py)       │
                 │   - 数值截断 (Numeric Clipping [lower, upper])       │
                 │   - 全局/局部敏感度动态计算 (Sensitivity)           │
                 │   - 噪声生成 (Laplace / Analytic Gaussian / BitFlip) │
                 │   - 分布式无噪累加器聚合 (Accumulator Merge)        │
                 └──────────────────────────┬──────────────────────────┘
                                            │
                                            ▼
                                扰动后隐私安全结果返回
```

### 2.1 差分隐私聚合与加噪机制 / DP Aggregation & Noise Generation

文件 / File：[`engine/privacy/dp.py`](engine/privacy/dp.py)

#### (1) Laplace 机制与敏感度计算

对查询函数 $f$，其 $L_1$ 全局敏感度 $\Delta f = \max_{\|D \Delta D'\|=1} \|f(D) - f(D')\|_1$。注入尺度为 $b = \frac{\Delta f}{\epsilon}$ 的拉普拉斯噪声：

```python
import math
import random
import numpy as np

def _laplace_noise(scale: float) -> float:
    """生成均值为 0、尺度为 scale 的拉普拉斯噪声。"""
    if scale <= 0:
        return 0.0
    u = random.random() - 0.5
    return -scale * math.copysign(1.0, u) * math.log(1.0 - 2.0 * abs(u))

def dp_count(
    values: Sequence[Any],
    epsilon: float = 0.1,
    namespace: str = "default",
) -> float:
    """差分隐私计数：Count 的 L1 敏感度严格恒等于 1.0。"""
    # 1. 预算记账器申请预算
    accountant = default_registry.get_accountant(namespace)
    accountant.spend(epsilon=epsilon, delta=0.0)
    
    # 2. 真实统计值计算
    true_count = float(len(values))
    
    # 3. 注入 Laplace(1.0 / epsilon) 噪声
    scale = 1.0 / epsilon
    noise = _laplace_noise(scale)
    return max(0.0, true_count + noise)
```

#### (2) 数值裁剪与 Sum / Mean 敏感度约束

在计算求和与均值时，未界定的数据具有无穷大敏感度。`PrivShield` 强制在查看数据前通过 `clip_lower` 与 `clip_upper` 对输入进行边界裁剪：

```python
def dp_sum(
    values: Sequence[float],
    epsilon: float = 0.1,
    clip_lower: float = 0.0,
    clip_upper: float = 100.0,
    mechanism: Mechanism = Mechanism.LAPLACE,
    delta: float = 0.0,
    namespace: str = "default",
) -> float:
    """差分隐私求和：敏感度 Delta = max(|clip_lower|, |clip_upper|)。"""
    accountant = default_registry.get_accountant(namespace)
    accountant.spend(epsilon=epsilon, delta=delta)

    # 1. 强制数值裁剪
    clipped = np.clip(np.asarray(values, dtype=np.float64), clip_lower, clip_upper)
    true_sum = float(np.sum(clipped))
    
    # 2. 敏感度计算
    sensitivity = max(abs(clip_lower), abs(clip_upper))
    
    # 3. 噪声注入
    if mechanism == Mechanism.LAPLACE:
        noise = _laplace_noise(sensitivity / epsilon)
    elif mechanism == Mechanism.GAUSSIAN:
        sigma = sensitivity * math.sqrt(2.0 * math.log(1.25 / delta)) / epsilon
        noise = random.gauss(0.0, sigma)
    return true_sum + noise
```

### 2.2 本地差分隐私（LDP）与随机响应 / Local DP & Randomized Response

文件 / File：[`engine/privacy/ldp.py`](engine/privacy/ldp.py)

#### (1) Warner 经典二元随机响应 (Randomized Response)

对于用户敏感布尔属性（例如“是否有某类疾病”），客户端以扰动概率 $p = \frac{e^\epsilon}{1 + e^\epsilon}$ 输出真实值，以 $1-p$ 翻转输出：

```python
class RandomizedResponse:
    """Warner 二元随机响应算子。"""
    def __init__(self, epsilon: float = 1.0):
        self.epsilon = epsilon
        self.p = math.exp(epsilon) / (1.0 + math.exp(epsilon))

    def perturb(self, bit: int) -> int:
        """客户端扰动单位数据。"""
        if random.random() < self.p:
            return bit
        return 1 - bit

    def aggregate(self, reports: Sequence[int]) -> float:
        """服务端对已扰动样本进行无偏估计还原真实比例。"""
        n = len(reports)
        if n == 0:
            return 0.0
        p_hat = sum(reports) / n
        # 无偏估计公式：(p_hat - (1-p)) / (2p - 1)
        est = (p_hat - (1.0 - self.p)) / (2.0 * self.p - 1.0)
        return max(0.0, min(1.0, est))
```

#### (2) 多分类 Frequency Oracle (k-RR)

针对 $k$ 个离散类别的频数统计，`ldp.py` 实现了 $k$-Randomized Response，确保多类别频数直方图的无偏重构。

---

### 2.3 隐私预算持久化记账与审计 / Budget Accounting & Audit

文件 / File：[`engine/privacy/budget.py`](engine/privacy/budget.py)

#### (1) 线程安全内存与 SQLite 双后端记账

```python
class BudgetAccountant:
    """单命名空间隐私预算记账器。"""
    def __init__(
        self,
        namespace: str = "default",
        total_epsilon: float = 10.0,
        total_delta: float = 1e-4,
        window_seconds: float | None = None,
        db_path: str | None = None,
    ):
        self.namespace = namespace
        self.total_epsilon = total_epsilon
        self.total_delta = total_delta
        self.window_seconds = window_seconds
        self._lock = threading.Lock()
        self._eps_spent = 0.0
        self._del_spent = 0.0
        self._window_start = time.time()

    def spend(self, epsilon: float, delta: float = 0.0) -> None:
        """原子扣减预算；若超额则抛出 PrivacyBudgetExhausted。"""
        with self._lock:
            # 1. 检查时间窗口自动清零
            if self.window_seconds and (time.time() - self._window_start >= self.window_seconds):
                self._eps_spent = 0.0
                self._del_spent = 0.0
                self._window_start = time.time()

            # 2. 边界检查
            if self._eps_spent + epsilon > self.total_epsilon + 1e-9:
                raise PrivacyBudgetExhausted(
                    f"Epsilon budget exhausted in namespace '{self.namespace}': "
                    f"spent={self._eps_spent:.4f}, requested={epsilon:.4f}, limit={self.total_epsilon:.4f}"
                )
            self._eps_spent += epsilon
            self._del_spent += delta
```

#### (2) HMAC-SHA256 不可篡改预算审计日志

每次调用 `spend` 均会生成带时间戳的防篡改签名日志行：
`timestamp | namespace | total_eps | total_del | spent_eps | spent_del | HMAC_SHA256_HEX`

---

## 3. 生产最佳实践与常见陷阱 / Production Best Practices & Pitfalls

1. **敏感度未裁剪（Unbounded Sensitivity）**：
   - 绝不能对无上界的数值直接执行 DP 求和。必须显式设置 `clip_lower` 与 `clip_upper`。
2. **浮点数时间侧信道与数值精度注入（Mironov 攻击）**：
   - 使用高质量安全随机数源（如 `secrets.SystemRandom` 或标准拉普拉斯均匀采样算法），规避浮点舍入导致的成员推断。
3. **分布式场景多 Worker 双重加噪**：
   - 分布式 MapReduce 任务应使用 `engine.privacy.dp.Accumulator`。在 Worker 端仅累加无噪中间态，合并后由 Master 节点统一调用一次 `finalize_dp()`。
4. **多实例预算一致性**：
   - 单进程内存记账无法跨容器同步。在 Kubernetes 多副本部署中，必须配置 `PRIVACY_BUDGET_DB=/data/budget.sqlite3` 挂载分布式共享卷。
