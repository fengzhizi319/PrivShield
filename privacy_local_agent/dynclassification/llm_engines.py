"""基于本地纯文本大模型 Qwen3.5-0.8B-Privacy-Classifier-Smoother 的数据分类分级器。

中文说明：
支持纯文本数据的零样本敏感定级推理（微调后的医疗隐私分类专用模型）。
具备延迟加载、自动降级等企业级能力。
注意：该模型为纯文本 CausalLM，不支持图像/视觉输入。

架构设计：
- Qwen3Classifier：纯文本分类器主类，继承 LlmClassifier 抽象基类
- 延迟加载：首次调用 classify() 时才加载模型权重（避免启动阻塞和显存浪费）
- 双重检查锁定：线程安全的模型初始化（gRPC 多线程环境）
- 专用推理线程池：隔离推理与 gRPC 工作线程，配合超时机制防止永久阻塞
- JSON 结果解析：正则提取 + 容错降级

降级策略：
- torch/transformers 未安装 → 初始化失败 → classify() 返回 None → 上层降级
- 模型目录不存在 → FileNotFoundError → 降级
- 推理超时（默认 180s）→ 放弃本次推理 → 返回 None → 降级
- JSON 解析失败 → 返回 None → 降级

English Description:
Data classification and grading engine based on local text-only LLM
Qwen3.5-0.8B-Privacy-Classifier-Smoother (fine-tuned medical privacy classifier).
Supports zero-shot sensitivity grading for plain text data. Features lazy-loading
and graceful degradation. Note: text-only model, no image/vision input support.
"""

# 启用延迟注解求值，允许在类型提示中引用尚未定义的类名
from __future__ import annotations

# 导入 JSON 解析模块，用于解析大模型返回的 JSON 结构化结果
import json
# 导入操作系统接口，用于文件路径拼接、目录存在性检查、环境变量读取
import os
# 导入正则表达式模块，用于 JSON 提取
import re
# 导入线程模块，用于创建互斥锁保护模型初始化和推理的线程安全
import threading
# 导入时间模块，用于测量推理耗时（monotonic 单调时钟）
import time
# 导入线程池执行器，用于将模型推理隔离到专用线程（配合超时机制）
from concurrent.futures import ThreadPoolExecutor
# 导入线程池超时异常类型，用于捕获推理超时事件
from concurrent.futures import TimeoutError as FuturesTimeoutError
# 导入类型注解工具：Any 通用类型，cast 类型断言
from typing import Any, cast

# 导入结构化日志工厂函数（支持 JSON 格式日志输出）
from ..observability.logging_config import get_logger
# 导入 Prometheus 指标：
# - CLASSIFICATION_LLM_DURATION：LLM 推理延迟直方图（按引擎标签）
# - CLASSIFICATION_LLM_TOTAL：LLM 调用次数计数器（按状态标签：success/error/timeout/init_failed）
from ..observability.metrics import (
    CLASSIFICATION_LLM_DURATION,
    CLASSIFICATION_LLM_TOTAL,
)
from ..env_loader import load_env_file
# 导入 LLM 分类器抽象基类和敏感度等级枚举
from .base import LlmClassifier, SensitivityLevel
# 导入日志脱敏工具函数（对敏感路径/值进行掩码处理后再记录日志）
# 以及不可信文本 prompt 中和工具（Prompt 注入防护）
from .utils import wrap_untrusted_text

# 创建模块级结构化日志器，用于记录 LLM 分类器相关事件
logger = get_logger(__name__)

# 默认模型目录名 / Default model directory name
_DEFAULT_MODEL_DIR = "Qwen3.5-0.8B-Privacy-Classifier-Smoother"


class Qwen3Classifier(LlmClassifier):
    """基于本地部署 Qwen3.5-0.8B-Privacy-Classifier-Smoother 的纯文本分类器。

    中文说明：
    支持对纯文本数据进行敏感等级评估。
    本类是三层分类漏斗的第三层（Layer-3），在规则引擎（Layer-1）和 NER（Layer-2）
    之后执行，作为最终的兜底分类手段。
    注意：该模型为纯文本 CausalLM，不支持图像输入。

    线程安全设计：
    - _lock：互斥锁，保护模型初始化（双重检查锁定）和推理过程（串行化）
    - _executor：单线程池，将推理隔离到独立线程，配合超时机制

    English Description:
    Text-only classifier based on the fine-tuned Qwen3.5-0.8B medical privacy model.
    Supports sensitivity level assessment for plain text inputs only (no image/vision).
    """

    # 推理超时（秒）：0.8B 纯文本模型在 CPU 上推理通常较快，
    # 超时后放弃本次推理并返回 None 触发降级，避免无限阻塞 gRPC 工作线程。
    # 可通过环境变量 PRIVACY_VLM_TIMEOUT 覆盖，默认 180 秒。
    _INFERENCE_TIMEOUT = int(os.environ.get("PRIVACY_VLM_TIMEOUT", "180"))

    def __init__(
        self,
        model_path: str | None = None,
        classify_prompt_template: str | None = None,
        device: str | None = None,
    ):
        """初始化分类器 / Initialize Classifier.

        仅设置路径和状态标志，不实际加载模型（延迟加载策略）。
        模型权重加载推迟到首次 classify() 调用时的 _lazy_init() 中执行。

        Args:
            model_path: 模型本地路径 / Local model path.
                如果不指定，默认使用项目根目录下的 .models/Qwen3.5-0.8B-Privacy-Classifier-Smoother。
                (Defaults to .models/Qwen3.5-0.8B-Privacy-Classifier-Smoother under project root)
            classify_prompt_template: 自定义分类 system prompt 模板 / Custom classification prompt template.
                支持占位符: {domain}, {standard_id}, {levels_desc}。
                None 时使用内置医疗领域默认 prompt。
            device: 目标计算设备（"cuda" / "cpu" / "mps" / None）。
        """
        # 如果未指定模型路径，自动计算默认路径 (.models/Qwen3.5-0.8B-Privacy-Classifier-Smoother)
        if not model_path:
            current_dir = os.path.dirname(os.path.abspath(__file__))
            project_root = os.path.dirname(os.path.dirname(current_dir))
            model_path = os.path.join(project_root, ".models", _DEFAULT_MODEL_DIR)

        # 保存模型路径供后续 _lazy_init 使用
        self.model_path = model_path
        # 保存自定义分类 prompt 模板（None 时使用内置默认）
        self._classify_prompt_template = classify_prompt_template
        # 显式指定的计算设备
        self.device = device
        # 模型实例占位（延迟初始化后赋值）
        self._model: Any = None
        # Tokenizer 实例占位（用于构建模型输入张量）
        self._tokenizer: Any = None
        # 初始化完成标志（False 表示尚未加载模型）
        self._initialized = False
        # 初始化错误缓存（记录首次失败原因，后续直接抛出不重试）
        self._init_error: Exception | None = None
        # 线程锁：gRPC 使用线程池处理请求，多个工作线程可能并发调用
        # _lazy_init / classify，需要互斥保护以防止：
        #   1. 多线程同时初始化模型导致重复加载或竞态
        #   2. 多线程同时推理导致显存/内存争用引发 OOM 崩溃
        self._lock = threading.Lock()
        # 专用推理线程池：将模型推理隔离到单独线程，配合超时机制，
        # 即使推理卡死也不会永久阻塞 gRPC 工作线程。
        # max_workers=1 确保同一时刻只有一个推理任务在执行（串行化）。
        self._executor = ThreadPoolExecutor(max_workers=1, thread_name_prefix="llm-infer")

    def _lazy_init(self):
        """延迟初始化模型 / Lazy-Initialize Model.

        中文说明：避免导入时或非 LLM 运行时占用显存或因缺少依赖报错。
        使用双重检查锁定（double-checked locking）确保线程安全：
        仅首次调用时加锁初始化，后续调用直接返回，避免锁竞争开销。

        双重检查锁定流程：
        1. 第一次检查（无锁）：快速路径，已初始化则直接返回
        2. 获取锁
        3. 第二次检查（有锁）：防止等锁期间另一线程已完成初始化
        4. 执行实际初始化逻辑

        English Description: Avoids occupying GPU memory at import time or when LLM
        is not needed, and prevents errors from missing dependencies.
        Uses double-checked locking for thread safety.

        Raises:
            FileNotFoundError: 本地模型目录不存在 / Local model directory not found.
        """
        # === 第一次检查（无锁快速路径）===
        # 已初始化则直接返回，避免不必要的锁竞争
        if self._initialized:
            return
        # 之前初始化失败过，直接抛出缓存的错误（不重复尝试加载）
        if self._init_error:
            raise self._init_error

        # === 获取互斥锁 ===
        with self._lock:
            # === 第二次检查（有锁）===
            # 另一个线程可能已在等锁期间完成初始化
            if self._initialized:
                return
            if self._init_error:
                raise self._init_error

            try:
                # 延迟导入 PyTorch（避免模块顶层导入导致的启动延迟和依赖问题）
                import torch
                # 延迟导入 transformers 库中的 CausalLM 模型类和 tokenizer
                from transformers import AutoModelForCausalLM, AutoTokenizer

                # 验证模型目录是否存在（用户需先下载微调模型）
                if not os.path.exists(self.model_path) or not os.path.isdir(self.model_path):
                    raise FileNotFoundError(
                        f"本地模型未找到，请先下载微调模型至: {self.model_path}"
                    )

                # 检测计算设备，优先级：显式/环境变量指定 > CUDA GPU > macOS MPS > CPU
                device = self._select_device(torch, custom_device=self.device)

                # 记录模型加载开始的结构化日志
                logger.info(
                    "qwen3_model_loading",
                    extra={"model_path": self.model_path, "device": device},
                )

                # 选择模型精度：CUDA/GPU 使用 FP16 节省显存，CPU/MPS 使用 FP32 保证精度
                is_cuda = device.startswith("cuda")
                dtype = torch.float16 if is_cuda else torch.float32

                # 从本地目录加载预训练 CausalLM 模型权重
                if is_cuda:
                    self._model = AutoModelForCausalLM.from_pretrained(
                        self.model_path,
                        torch_dtype=dtype,
                        device_map="auto" if device == "cuda" else device,
                        trust_remote_code=True,
                    )
                elif device == "mps":
                    self._model = AutoModelForCausalLM.from_pretrained(
                        self.model_path,
                        torch_dtype=torch.float32,
                        device_map="mps",
                        trust_remote_code=True,
                    )
                else:
                    self._model = AutoModelForCausalLM.from_pretrained(
                        self.model_path,
                        torch_dtype=torch.float32,
                        trust_remote_code=True,
                    )
                    self._model = self._model.to("cpu")

                # 加载模型对应的 tokenizer
                self._tokenizer = AutoTokenizer.from_pretrained(
                    self.model_path, trust_remote_code=True
                )
                # 标记初始化完成
                self._initialized = True
                # 记录模型初始化成功的结构化日志
                logger.info(
                    "qwen3_model_initialized",
                    extra={"model_path": self.model_path, "device": device, "engine": "qwen3"},
                )

            except Exception as e:
                # 初始化失败：缓存错误对象，后续调用直接抛出（不重复尝试）
                self._init_error = e
                # 记录初始化失败的警告日志
                logger.warning(
                    "qwen3_model_init_failed",
                    extra={"error": str(e), "model_path": self.model_path},
                )
                # 重新抛出异常，让调用方（classify）捕获并触发降级
                raise e

    @property
    def _processor(self) -> Any:
        return self._tokenizer

    @_processor.setter
    def _processor(self, value: Any) -> None:
        self._tokenizer = value

    @property
    def is_ready(self) -> bool:
        """模型是否已完成初始化且未发生错误 / Whether Model Is Ready.

        用于健康检查接口 /readyz/llm 判断 LLM 层是否可用。

        Returns:
            模型就绪状态 / Model readiness status.
        """
        # 两个条件同时满足才视为就绪：已初始化 且 无错误
        return self._initialized and self._init_error is None

    def warmup(self) -> bool:
        """主动触发模型加载 / Proactively Trigger Model Loading.

        中文说明：同步阻塞调用，建议在后台线程/协程中调用。
        服务启动时可通过 PRIVACY_WARMUP_LLM=true 环境变量触发异步预热，
        避免首次请求时因模型加载导致的高延迟。

        English Description: Synchronous blocking call; recommended to invoke in a
        background thread or coroutine.

        Returns:
            是否成功完成初始化 / Whether initialization succeeded.
        """
        try:
            # 触发延迟初始化（加载模型权重）
            self._lazy_init()
            return True  # 初始化成功
        except Exception:
            return False  # 初始化失败（依赖缺失/模型不存在等）

    @staticmethod
    def _is_cuda_compatible(torch: Any) -> bool:
        """验证当前 PyTorch 是否真能在检测到的 CUDA 设备上执行 kernel。

        某些 GPU 的算力（compute capability）比当前 PyTorch 构建支持的范围更新
        （例如 RTX 50 系列的 sm_120 与 PyTorch 2.6+cu124）。此时
        ``torch.cuda.is_available()`` 仍会返回 True，但任何真正的 CUDA 运算都会抛出
        ``RuntimeError: no kernel image is available for execution on the device``。

        本方法执行一次微小的张量运算来确认 CUDA 不仅"可见"而且"可用"，避免后续
        模型加载时因不兼容架构而崩溃。

        Returns:
            当前 PyTorch 能在 CUDA 上执行 kernel 时返回 True，否则 False。
        """
        if not torch.cuda.is_available():
            return False
        try:
            # 执行一次需要 CUDA kernel 的微小运算，捕获算力不兼容等真实错误。
            a = torch.tensor([1.0, 2.0, 3.0], device="cuda")
            b = torch.tensor([1.0, 1.0, 1.0], device="cuda")
            _ = (a + b).sum().item()
            return True
        except RuntimeError:
            return False

    @staticmethod
    def _select_device(torch: Any, custom_device: str | None = None) -> str:
        """根据硬件环境选择推理设备 / Select Inference Device.

        设备选择级联策略 (Cascading Order):
        1. 显式配置优先：custom_device 参数 或 PRIVACY_LLM_DEVICE / PRIVACY_DEVICE 环境变量。
        2. CUDA GPU 探测（NVIDIA 卡优先）：开启 CUDA 算力与显存校验，通过则选择 "cuda"。
        3. Mac Metal 探测（Apple Silicon 芯片）：若 macOS MPS 可用，选择 "mps"。
        4. CPU 回退：无 GPU 或加速器时降级至 "cpu"。

        Returns:
            设备字符串："cuda" / "mps" / "cpu" 等。
        """
        # 1. 显式配置或环境变量优先
        target_device = (
            custom_device
            or os.environ.get("PRIVACY_LLM_DEVICE")
            or os.environ.get("PRIVACY_DEVICE")
        )
        if target_device:
            target_device_lower = target_device.lower()
            if target_device_lower in ("cpu", "mps") or target_device_lower.startswith("cuda"):
                if target_device_lower.startswith("cuda") and not Qwen3Classifier._is_cuda_compatible(torch):
                    logger.warning(
                        "qwen3_custom_cuda_not_compatible_fallback_next",
                        extra={"target_device": target_device},
                    )
                else:
                    return target_device_lower

        # 2. 级联 1：优先检测 NVIDIA CUDA GPU
        if torch.cuda.is_available() and Qwen3Classifier._is_cuda_compatible(torch):
            try:
                total_free = sum(
                    torch.cuda.mem_get_info(i)[0] for i in range(torch.cuda.device_count())
                )
                # Qwen3.5-0.8B FP16 约需 1.6GB 显存，可通过 PRIVACY_VLM_MIN_VRAM_GB 配置（默认 1.6GB）
                min_vram_gb = float(os.environ.get("PRIVACY_VLM_MIN_VRAM_GB", "1.6"))
                min_vram_bytes = min_vram_gb * 1024 * 1024 * 1024
                if total_free >= min_vram_bytes:
                    logger.info(
                        "qwen3_select_cuda",
                        extra={
                            "device_count": torch.cuda.device_count(),
                            "free_vram_gb": round(total_free / (1024**3), 2),
                        },
                    )
                    return "cuda"
                logger.info(
                    "qwen3_cuda_vram_insufficient_checking_mps_or_cpu",
                    extra={
                        "free_vram_gb": round(total_free / (1024**3), 2),
                        "required_vram_gb": min_vram_gb,
                    },
                )
            except Exception as e:
                logger.warning(
                    "qwen3_vram_check_failed_checking_mps_or_cpu",
                    extra={"error": str(e)},
                )
                # 即使显存获取异常，CUDA 本身依然兼容可用，直接使用 CUDA
                return "cuda"

        # 3. 级联 2：检测 Apple Silicon Mac Metal (MPS)
        if hasattr(torch.backends, "mps") and torch.backends.mps.is_available():
            logger.info("qwen3_select_mps_metal", extra={"engine": "qwen3", "device": "mps"})
            return "mps"

        # 4. 级联 3：无可用 GPU 时降级至 CPU
        logger.info("qwen3_select_cpu_fallback", extra={"engine": "qwen3", "device": "cpu"})
        return "cpu"

    def classify(
        self,
        text: str,
        upstream_level: SensitivityLevel,
        upstream_confidence: float,
        sanitize: bool = False,
    ) -> dict[str, Any] | None:
        """使用本地 Qwen3.5 微调模型对输入进行分类。"""
        try:
            self._lazy_init()
        except Exception:
            # 初始化失败（依赖缺失/模型不存在），递增失败计数并返回 None 触发降级
            CLASSIFICATION_LLM_TOTAL.labels(status="init_failed").inc()
            return None  # 初始化失败，直接返回 None，自动触发底层降级逻辑

        # 记录推理开始时间（monotonic 单调时钟，不受系统时间调整影响）
        start_time = time.monotonic()
        try:
            # 将实际推理提交到专用线程池，设置超时保护。
            # 如果推理超时（如模型卡死），放弃本次推理并返回 None 触发降级，
            # 避免永久阻塞 gRPC 工作线程导致后续所有请求排队失败。
            future = self._executor.submit(
                self._do_classify, text, upstream_level, upstream_confidence
            )
            # 等待推理结果，超过 _INFERENCE_TIMEOUT 秒则抛出 FuturesTimeoutError
            result = future.result(timeout=self._INFERENCE_TIMEOUT)

            # 计算推理耗时并记录到 Prometheus 直方图指标
            duration = time.monotonic() - start_time
            CLASSIFICATION_LLM_DURATION.labels(engine="qwen3").observe(duration)
            # 记录推理完成的 debug 日志
            logger.debug(
                "llm_classify_completed",
                extra={
                    "duration_s": round(duration, 4),
                    "has_result": result is not None,
                },
            )
            return result

        except FuturesTimeoutError:
            # 推理超时：模型可能卡死或输入过于复杂
            duration = time.monotonic() - start_time
            # 递增超时状态计数器
            CLASSIFICATION_LLM_TOTAL.labels(status="timeout").inc()
            # 记录超时耗时到直方图
            CLASSIFICATION_LLM_DURATION.labels(engine="qwen3").observe(duration)
            # 记录超时错误日志
            logger.error(
                "llm_classify_timeout",
                extra={
                    "timeout_s": self._INFERENCE_TIMEOUT,
                    "duration_s": round(duration, 4),
                },
            )
            return None  # 返回 None 触发降级

        except Exception as e:
            # 推理过程中发生其他异常（OOM/模型错误等）
            duration = time.monotonic() - start_time
            # 递增错误状态计数器
            CLASSIFICATION_LLM_TOTAL.labels(status="error").inc()
            # 记录错误耗时到直方图
            CLASSIFICATION_LLM_DURATION.labels(engine="qwen3").observe(duration)
            # 记录错误详情日志
            logger.error(
                "llm_classify_error",
                extra={"error": str(e), "duration_s": round(duration, 4)},
            )
            return None  # 返回 None 触发降级

    def _do_classify(
        self, text: str, upstream_level: SensitivityLevel, upstream_confidence: float
    ) -> dict[str, Any] | None:
        """实际执行模型推理的内部方法（在专用线程中运行）。

        使用 self._lock 保护推理过程，确保同一时刻只有一个线程在执行
        模型推理，避免多线程并发推理导致显存/内存争用引发 OOM 崩溃。

        注意：此方法在 _executor 线程池的工作线程中执行，而非 gRPC 线程。
        锁的获取可能阻塞（当另一个推理正在进行时），但外层的超时机制
        会确保 gRPC 线程不会永久等待。

        Args:
            text: 待分类文本。
            upstream_level: 上游敏感度等级。
            upstream_confidence: 上游置信度。

        Returns:
            分类结果字典或 None。
        """
        # 获取互斥锁，串行化推理（防止并发推理导致 OOM）
        with self._lock:
            # 委托给 _classify_inner 执行实际的推理逻辑
            return self._classify_inner(text, upstream_level, upstream_confidence)

    def _classify_inner(
        self, text: str, upstream_level: SensitivityLevel, upstream_confidence: float
    ) -> dict[str, Any] | None:
        """模型推理核心逻辑（已持有锁）。

        执行步骤：
        1. 构建 system prompt（定义评估标准和输出格式）
        2. 构建 user content（纯文本输入）
        3. 使用 tokenizer 构建模型输入张量
        4. 执行模型 generate 推理
        5. 解码生成 token 为文本
        6. 从文本中提取 JSON 结构化结果

        Args:
            text: 待分类文本。
            upstream_level: 上游敏感度等级（供 prompt 参考）。
            upstream_confidence: 上游置信度（供 prompt 参考）。

        Returns:
            解析后的分类结果字典或 None。
        """
        try:
            # 构建 system prompt：定义角色、评估标准和输出 JSON 格式
            # 优先使用自定义模板（支持 {domain}/{standard_id}/{levels_desc} 占位符）
            #
            # 训练/推理对齐说明（仅 Qwen3Classifier 加载微调模型，其他引擎保留原 prompt）：
            # 本引擎的 L1~L5 等级定义与输出字段契约（final_level/confidence/reasoning/
            # sanitized_text）与微调训练侧 llmlora/src/dataset/loader.py 的
            # SYSTEM_PROMPT 及输出 schema 保持一致，避免训练/推理分布偏移导致
            # 微调模型输出漂移；渲染 prompt 时亦与训练侧一致地关闭 thinking 模式。
            if self._classify_prompt_template:
                system_prompt = self._classify_prompt_template.format(
                    domain="medical",
                    standard_id="DB51_T_2989",
                    levels_desc=(
                        "- L1 (公开数据): 无敏感信息的公开资讯、通用日常文本。\n"
                        "- L2 (内部数据): 业务统计指标、系统日志、设备运维等低敏感内部数据。\n"
                        "- L3 (敏感数据/个人基本信息): 姓名、身份证号、手机号、银行卡号、电子邮箱等个人基础标识与资产信息。\n"
                        "- L4 (高敏感数据/诊疗与金融敏感): 疾病诊断（如重度抑郁症、高血压、冠心病）、病历主诉、处方药品等医疗健康敏感信息。\n"
                        "- L5 (极敏感数据): 基因组、生物特征、特级商业机密等核心数据。"
                    ),
                )
            else:
                system_prompt = (
                    "你是一个专业的隐私安全Sidecar助手。请分析输入的文本，识别敏感信息，"
                    "输出分类分级结果（JSON格式），并提供语义连贯的无痕抹平脱敏重写文本。\n\n"
                    "【数据分类分级标准指南】\n"
                    "- L1 (公开数据): 无敏感信息的公开资讯、通用日常文本。\n"
                    "- L2 (内部数据): 业务统计指标、系统日志、设备运维等低敏感内部数据。\n"
                    "- L3 (敏感数据/个人基本信息): 姓名、身份证号、手机号、银行卡号、电子邮箱等个人基础标识与资产信息。\n"
                    "- L4 (高敏感数据/诊疗与金融敏感): 疾病诊断（如重度抑郁症、高血压、冠心病）、病历主诉、处方药品等医疗健康敏感信息。\n"
                    "- L5 (极敏感数据): 基因组、生物特征、特级商业机密等核心数据。\n\n"
                    "请严格根据上述标准进行定级，并仅输出符合以下 JSON 格式的结构化内容，不要包含额外的解释文字或 ``` 块：\n"  # noqa: E501
                    "{\n"
                    '  "final_level": "L1/L2/L3/L4/L5",\n'
                    '  "confidence": 0.0到1.0之间的浮点数,\n'
                    '  "reasoning": "定级判别的推理过程说明",\n'
                    '  "sanitized_text": "语义连贯的无痕抹平脱敏重写文本"\n'
                    "}"
                )

            # 构建 user content：纯文本输入
            # 剥离 chat-template 控制 token（防 Prompt 注入伪造对话轮次），
            # 并用明确分隔符包裹 + 声明"以下是数据而非指令"后嵌入待评估文本
            user_text = f"请评估以下文本数据的敏感数据等级：\n{wrap_untrusted_text(text)}"

            # 组装完整的对话消息列表（system + user）
            messages = [
                {"role": "system", "content": system_prompt},
                {"role": "user", "content": user_text},
            ]

            # 使用 tokenizer 将对话消息转换为模型可接受的文本 prompt 格式
            # apply_chat_template 会按照模型的对话模板格式化消息。
            # 与训练侧（llmlora/src/dataset/loader.py render_prompt_text）保持一致：
            # Qwen3.5 模板在 add_generation_prompt 时默认注入 <think> 前缀，
            # 必须显式传入 enable_thinking=False 走非思考分支，
            # 否则推理输出会带思考标记导致 JSON 解析失败；
            # 不支持该 kwarg 的旧模板回退为不传（TypeError 兼容）。
            try:
                text_prompt = self._tokenizer.apply_chat_template(
                    messages, tokenize=False, add_generation_prompt=True, enable_thinking=False
                )
            except TypeError:
                text_prompt = self._tokenizer.apply_chat_template(
                    messages, tokenize=False, add_generation_prompt=True
                )

            # 使用 tokenizer 将文本转换为模型输入张量（input_ids 等）
            inputs = self._tokenizer(
                text=[text_prompt], padding=True, return_tensors="pt"
            )
            # 将所有输入张量移动到模型所在设备（CUDA/MPS/CPU）
            inputs = {k: v.to(self._model.device) for k, v in inputs.items()}

            # 执行模型推理生成
            import torch

            generated_ids = None
            try:
                # 禁用梯度计算（推理模式，节省显存和计算资源）
                with torch.no_grad():
                    try:
                        generated_ids = self._model.generate(**inputs, max_new_tokens=512)
                    except RuntimeError as err:
                        if "cudnn" in str(err).lower() or "SUBLIBRARY_VERSION_MISMATCH" in str(err):
                            logger.warning("cuDNN 版本冲突，自动禁用 cuDNN 加速改用 PyTorch 原生 CUDA 卷积...")
                            torch.backends.cudnn.enabled = False
                            generated_ids = self._model.generate(**inputs, max_new_tokens=512)
                        else:
                            raise

                # 裁剪生成结果：去掉输入 prompt 部分，只保留新生成的 token
                generated_ids_trimmed = [
                    out_ids[len(in_ids) :]
                    for in_ids, out_ids in zip(inputs["input_ids"], generated_ids)
                ]
                output_text = self._tokenizer.batch_decode(
                    generated_ids_trimmed,
                    skip_special_tokens=True,
                    clean_up_tokenization_spaces=False,
                )[0]

                result = self._parse_json_result(output_text, upstream_level, upstream_confidence)
                CLASSIFICATION_LLM_TOTAL.labels(status="success").inc()
                return result
            finally:
                # 显式释放临时张量与 CUDA 显存缓存（防 CUDA VRAM OOM 积累）
                del inputs, generated_ids
                if torch.cuda.is_available():
                    torch.cuda.empty_cache()

        except Exception as e:
            CLASSIFICATION_LLM_TOTAL.labels(status="error").inc()
            logger.error(
                "llm_classify_inner_error",
                extra={"error": str(e)},
            )
            return None  # 返回 None 触发上层降级逻辑

    def _parse_json_result(
        self, output_text: str, upstream_level: SensitivityLevel, upstream_confidence: float
    ) -> dict[str, Any] | None:
        """解析大模型返回的 JSON / Parse LLM JSON Output.

        中文说明：使用正则表达式清洗并解析大模型返回的 JSON。
        大模型可能在 JSON 前后包含额外文字或 ```json``` 代码块标记，
        因此使用正则提取 {} 区间内容进行解析。

        容错策略：
        - 优先提取 {} 包裹的 JSON 内容
        - 如果提取失败，尝试直接解析整个输出文本
        - 解析失败或关键字段缺失则返回 None 触发降级

        English Description: Cleans and parses JSON from LLM output using regex.

        Args:
            output_text: 模型生成的原始文本 / Raw generated text from model.
            upstream_level: 上游敏感度等级 / Upstream sensitivity level.
            upstream_confidence: 上游置信度 / Upstream confidence.

        Returns:
            解析后的结果字典（含 final_level 等字段）或 None / Parsed result dict or None.
        """
        # 先剥离 Qwen3.5 等思考模型可能输出的 <think>...</think> 思考链
        cleaned_text = re.sub(r"<think>.*?</think>", "", output_text, flags=re.DOTALL).strip()

        # 使用正则表达式提取第一个 {} 包裹的 JSON 内容（DOTALL 匹配跨行）
        json_match = re.search(r"(\{.*\})", cleaned_text, re.DOTALL)
        json_str = json_match.group(1) if json_match else cleaned_text

        try:
            # 尝试解析 JSON 字符串为 Python 字典
            res = json.loads(json_str)
            # 校验关键字段 final_level 是否存在
            if isinstance(res, dict) and "final_level" in res:
                # 兼容训练/推理字段契约：统一补全 category 与 sub_category
                cat = res.get("category") or res.get("sub_category") or "GENERAL"
                res["category"] = cat
                res["sub_category"] = cat
                return cast("dict[str, Any]", res)
        except Exception as e:
            logger.warning(
                "llm_json_parse_failed",
                extra={"error": str(e)},
            )

        return None


# 向后兼容别名：旧代码可能通过 Qwen2VLClassifier 引用 / Backward-compatible alias
Qwen2VLClassifier = Qwen3Classifier


class OpenAILlmClassifier(LlmClassifier):
    """基于 OpenAI 兼容 HTTP API（如 vLLM、Ollama、DeepSeek、Qwen API）的数据分类分级器。

    支持通过 HTTP POST 服务与部署在 8000 端口的 vLLM OpenAI API 通信，
    执行 Layer-3 LLM 敏感度评估。

    使用 Python 标准库 urllib.request 实现，无需额外的 ML/PyTorch/httpx 依赖。
    """

    _INFERENCE_TIMEOUT = int(
        os.environ.get("PRIVACY_VLM_TIMEOUT", os.environ.get("PRIVACY_LLM_TIMEOUT", "180"))
    )

    def __init__(
        self,
        api_base: str | None = None,
        model_name: str | None = None,
        api_key: str | None = None,
        classify_prompt_template: str | None = None,
        timeout: float | None = None,
    ):
        """初始化 OpenAI/vLLM 接口分类器。

        Args:
            api_base: OpenAI API 基础 URL，默认从 PRIVACY_LLM_API_BASE 或 PRIVACY_VLLM_URL 读取，
                回退至 "http://127.0.0.1:8000/v1"。
            model_name: 模型名称，默认从 PRIVACY_LLM_MODEL_NAME 读取，
                回退至 "Qwen3.5-0.8B-Privacy-Classifier-Smoother"。
            api_key: API 密钥（可选，默认 PRIVACY_LLM_API_KEY 或 "EMPTY"）。
            classify_prompt_template: 自定义分类 system prompt 模板。
            timeout: 超时时间（秒）。
        """
        load_env_file()

        base_url = (
            api_base
            or os.environ.get("PRIVACY_LLM_API_BASE")
            or os.environ.get("PRIVACY_VLLM_URL")
            or "http://127.0.0.1:8000/v1"
        )
        base_url = base_url.rstrip("/")
        if not base_url.endswith("/v1") and not base_url.endswith("/chat/completions"):
            self.chat_url = f"{base_url}/v1/chat/completions"
        elif base_url.endswith("/v1"):
            self.chat_url = f"{base_url}/chat/completions"
        else:
            self.chat_url = base_url

        self.api_base = base_url
        self.model_name = (
            model_name
            or os.environ.get("PRIVACY_LLM_MODEL_NAME")
            or _DEFAULT_MODEL_DIR
        )
        self.api_key = api_key or os.environ.get("PRIVACY_LLM_API_KEY", "EMPTY")
        self._classify_prompt_template = classify_prompt_template
        self.timeout = timeout or float(self._INFERENCE_TIMEOUT)

        self._initialized = True
        self._init_error: Exception | None = None
        self._lock = threading.Lock()
        self._executor = ThreadPoolExecutor(max_workers=2, thread_name_prefix="vllm-http-infer")

    @property
    def is_ready(self) -> bool:
        """服务器是否已就绪。"""
        return self._initialized and self._init_error is None

    def warmup(self) -> bool:
        """预热连接（保持与基类一致）。"""
        return True

    def classify(
        self,
        text: str,
        upstream_level: SensitivityLevel,
        upstream_confidence: float,
        sanitize: bool = False,
    ) -> dict[str, Any] | None:
        """通过 HTTP 调用 vLLM / OpenAI 服务对文本进行定级。"""
        start_time = time.monotonic()
        try:
            future = self._executor.submit(
                self._do_classify_http, text, upstream_level, upstream_confidence
            )
            result = future.result(timeout=self.timeout)
            duration = time.monotonic() - start_time
            CLASSIFICATION_LLM_DURATION.labels(engine="vllm").observe(duration)
            return result
        except FuturesTimeoutError:
            duration = time.monotonic() - start_time
            CLASSIFICATION_LLM_TOTAL.labels(status="timeout").inc()
            CLASSIFICATION_LLM_DURATION.labels(engine="vllm").observe(duration)
            logger.error("vllm_http_classify_timeout", extra={"timeout_s": self.timeout, "url": self.chat_url})
            return None
        except Exception as e:
            duration = time.monotonic() - start_time
            CLASSIFICATION_LLM_TOTAL.labels(status="error").inc()
            CLASSIFICATION_LLM_DURATION.labels(engine="vllm").observe(duration)
            logger.error("vllm_http_classify_error", extra={"error": str(e), "url": self.chat_url})
            return None

    def _do_classify_http(
        self, text: str, upstream_level: SensitivityLevel, upstream_confidence: float
    ) -> dict[str, Any] | None:
        """执行 HTTP POST 请求与 JSON 解析。"""
        if self._classify_prompt_template:
            system_prompt = self._classify_prompt_template.format(
                domain="medical",
                standard_id="DB51_T_2989",
                levels_desc=(
                    "- L5 (极高风险): 包含人类基因序列、遗传信息、基因突变或罕见病样本。\n"
                    "- L4 (高风险): 包含精神疾病、敏感传染病或完整的住院病历。\n"
                    "- L3 (中风险): 包含个人身份信息（PII）、普通的门诊诊疗记录或常规检验指标数值。\n"
                    "- L2 (低风险): 仅包含医院科室运营、设备使用率或脱敏后的去标识化统计数据。\n"
                    "- L1 (公开级): 年度门诊总量等医院公开宣传、无任何敏感特征的统计指标。"
                ),
            )
        else:
            system_prompt = (
                "你是一个医疗数据分类分级领域的资深安全专家。请对输入的医疗数据进行敏感等级评估。\n"
                "评估标准如下：\n"
                "- L5 (极高风险): 包含人类基因序列、遗传信息、基因突变（如 BRCA1/TP53）或罕见病样本。\n"
                "- L4 (高风险): 包含精神疾病（如精神分裂）、敏感传染病（如 HIV/AIDS/梅毒）或完整的住院病历。\n"
                "- L3 (中风险): 包含个人身份信息（PII，如身份证号、手机号）、普通的门诊诊疗记录或常规检验指标数值（如血常规）。\n"
                "- L2 (低风险): 仅包含医院科室运营、设备使用率或脱敏后的去标识化统计数据。\n"
                "- L1 (公开级): 年度门诊总量等医院公开宣传、无任何敏感和特征的统计指标。\n\n"
                "请严格根据上述标准进行定级，并仅输出符合以下 JSON 格式的结构化内容，不要包含额外的解释文字或 ``` 块：\n"
                "{\n"
                '  "final_level": "L1/L2/L3/L4/L5",\n'
                '  "sub_category": "分类标签简称",\n'
                '  "confidence": 0.0到1.0之间的浮点数,\n'
                '  "reasoning": "定级判别的推理过程说明",\n'
                '  "needs_human_review": true/false\n'
                "}"
            )

        user_text = f"请评估以下文本数据的敏感数据等级：\n{wrap_untrusted_text(text)}"
        payload = {
            "model": self.model_name,
            "messages": [
                {"role": "system", "content": system_prompt},
                {"role": "user", "content": user_text},
            ],
            "temperature": 0.0,
            "max_tokens": 512,
        }

        import urllib.error
        import urllib.request

        req_data = json.dumps(payload).encode("utf-8")
        # noqa: S310 —— chat_url 来自显式运维配置（vLLM API base），非用户输入拼接
        req = urllib.request.Request(  # noqa: S310
            self.chat_url,
            data=req_data,
            headers={
                "Content-Type": "application/json",
                "Authorization": f"Bearer {self.api_key}",
            },
            method="POST",
        )

        try:
            with urllib.request.urlopen(req, timeout=self.timeout) as resp:  # noqa: S310
                if resp.status != 200:
                    logger.warning("vllm_http_non_200", extra={"status": resp.status})
                    CLASSIFICATION_LLM_TOTAL.labels(status="error").inc()
                    return None
                resp_body = resp.read().decode("utf-8")
                resp_json = json.loads(resp_body)
                content = resp_json["choices"][0]["message"]["content"]
                result = self._parse_json_result(content, upstream_level, upstream_confidence)
                if result:
                    CLASSIFICATION_LLM_TOTAL.labels(status="success").inc()
                else:
                    CLASSIFICATION_LLM_TOTAL.labels(status="error").inc()
                return result
        except (urllib.error.HTTPError, urllib.error.URLError, TimeoutError, OSError) as err:
            logger.warning("vllm_http_request_failed", extra={"error": str(err)})
            CLASSIFICATION_LLM_TOTAL.labels(status="error").inc()
            return None

    def _parse_json_result(
        self, output_text: str, upstream_level: SensitivityLevel, upstream_confidence: float
    ) -> dict[str, Any] | None:
        json_match = re.search(r"(\{.*\})", output_text, re.DOTALL)
        json_str = json_match.group(1) if json_match else output_text
        try:
            res = json.loads(json_str)
            if "final_level" in res:
                return cast("dict[str, Any]", res)
        except Exception as e:
            logger.warning("llm_json_parse_failed", extra={"error": str(e)})
        return None


# 别名定义
VLLMLlmClassifier = OpenAILlmClassifier

