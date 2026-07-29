"""Profile 加载器与缓存管理器 / Profile Loader & Cache Manager.

该模块负责动态数据分类体系中规则配置文件的加载、缓存、热重载与引擎实例构建。
核心职责包括：
1. 从 YAML 配置文件中加载分类体系定义（DomainTaxonomy）、领域规则 Profile（RuleProfile）与标准组合定义（StandardDef）。
2. 根据指定的 domain（领域）或 standard（标准组合），动态组合与构建 ConfigurableRuleEngine 及 CompositeRuleEngine 实例。
3. 提供基于文件修改时间（mtime）的轻量级热重载（Hot-Reload）检测机制与缓存失效管理。
4. 集成 Prometheus 监控指标，记录引擎加载耗时与缓存大小。
"""

from __future__ import annotations

import os
import threading
import time
from pathlib import Path
from typing import Optional

import yaml

# Prometheus 监控指标：引擎加载耗时分布 & Profile 缓存中的引擎数量
from ..observability.metrics import (
    DYNCLASSIFICATION_ENGINE_LOAD_DURATION,
    DYNCLASSIFICATION_PROFILE_CACHE_SIZE,
)
from .composite import CompositeRuleEngine
from .engine import ConfigurableRuleEngine
from .models import DomainTaxonomy
from .rule_schema import CompositeRuleDef, DowngradeRuleDef, RuleDef, RuleProfile, StandardDef


class ProfileLoader:
    """Profile 加载器与缓存管理器。

    负责从 YAML 文件加载分类体系、领域规则包和标准组合定义，
    并根据 domain / standard 组合构建与管理规则引擎实例（ConfigurableRuleEngine & CompositeRuleEngine）。

    目录结构约定：
        rules_dir/
        ├── taxonomies/     # 分类体系 YAML 目录（如 default.yaml, medical.yaml）
        ├── domains/        # 领域规则包 YAML 目录（如 general-pii.yaml, medical.yaml）
        └── standards/      # 标准组合 YAML 目录（如 compliance-standard.yaml）

    Attributes:
        rules_dir (Path): 规则配置根目录路径。
        hot_reload_enabled (bool): 是否开启基于文件变动监测的热重载功能。
        reload_interval_seconds (float): 两次热重载检查之间的最小时间间隔（秒），0 表示不做时间间隔限制。
        _last_check_time (float): 上次执行热重载检测的时间戳。
        _lock (threading.RLock): 可重入锁，保证多线程并发访问和缓存清理时的线程安全。
        _taxonomy_cache (dict[str, DomainTaxonomy]): 分类体系缓存（Key 为 taxonomy 名称）。
        _profile_cache (dict[str, RuleProfile]): 领域规则 Profile 缓存（Key 为 domain 名称）。
        _standard_cache (dict[str, StandardDef]): 标准组合定义缓存（Key 为 standard_id）。
        _engine_cache (dict[str, ConfigurableRuleEngine]): ConfigurableRuleEngine 引擎实例缓存（Key 为 "domain:standard"）。
        _composite_cache (dict[str, CompositeRuleEngine]): CompositeRuleEngine 复合引擎实例缓存（Key 为 "domain:standard"）。
        _file_mtimes (dict[Path, float]): 规则 YAML 文件最近一次加载时的修改时间记录（Key 为文件路径，Value 为 mtime 时间戳）。
    """

    def __init__(self, rules_dir: str | Path | None = None):
        """初始化 Profile 加载器。

        优先使用传入的 `rules_dir` 参数；若未指定，则读取环境变量 `PRIVACY_DYNCLASSIFICATION_RULES_DIR`；
        若环境变量亦未设置，则默认使用 `"rules"` 目录。

        Args:
            rules_dir: 规则配置根目录路径。可选。
        """
        # Resolve rules directory: explicit param > env var > default "rules".
        env_rules_dir = os.environ.get("PRIVACY_DYNCLASSIFICATION_RULES_DIR", "rules")
        target_dir = rules_dir if rules_dir is not None else env_rules_dir
        self.rules_dir = Path(target_dir)

        # Hot-reload configuration: whether to detect file changes automatically.
        self.hot_reload_enabled = (
            os.environ.get("PRIVACY_DYNCLASSIFICATION_HOT_RELOAD", "true").lower() == "true"
        )
        # Minimum interval between hot-reload checks (seconds). 0 = no throttle.
        self.reload_interval_seconds = float(
            os.environ.get("PRIVACY_DYNCLASSIFICATION_RELOAD_INTERVAL", "0")
        )
        # Timestamp of last hot-reload check (for interval throttling).
        self._last_check_time = 0.0

        # Reentrant lock: protects all cache mutations and hot-reload scans.
        # RLock allows the same thread to acquire multiple times (e.g. nested calls).
        self._lock = threading.RLock()
        # Cache dictionaries: avoid re-parsing YAML files on every request.
        self._taxonomy_cache: dict[str, DomainTaxonomy] = {}      # name -> taxonomy
        self._profile_cache: dict[str, RuleProfile] = {}          # domain -> profile
        self._standard_cache: dict[str, StandardDef] = {}         # standard_id -> definition
        self._engine_cache: dict[str, ConfigurableRuleEngine] = {}  # "domain:standard" -> engine
        self._composite_cache: dict[str, CompositeRuleEngine] = {}  # "domain:standard" -> composite
        # File modification times: used for hot-reload change detection.
        self._file_mtimes: dict[Path, float] = {}

    def check_and_reload(self, force: bool = False) -> bool:
        """检查 `rules_dir` 目录下的所有 YAML 文件修改时间，若有变动则自动重载（清空缓存）。

        线程安全。在未开启热重载或配置目录不存在时直接返回 False。

        Args:
            force: 是否忽略 `reload_interval_seconds` 时间间隔检查，强制扫描所有文件 mtime。

        Returns:
            bool: 是否检测到文件变动并触发了缓存重载。
        """
        # Skip if hot-reload is disabled or rules directory doesn't exist.
        if not self.hot_reload_enabled or not self.rules_dir.exists():
            return False

        now = time.time()
        with self._lock:
            # Throttle: skip check if within the configured interval (unless forced).
            if (
                not force
                and self.reload_interval_seconds > 0
                and self._last_check_time > 0
                and (now - self._last_check_time) < self.reload_interval_seconds
            ):
                return False
            # Update last check timestamp.
            self._last_check_time = now

            changed = False
            # Recursively scan all .yaml files under rules_dir.
            for yaml_file in self.rules_dir.glob("**/*.yaml"):
                mtime = yaml_file.stat().st_mtime
                # Detect new files or modified files by comparing mtime.
                if yaml_file not in self._file_mtimes or self._file_mtimes[yaml_file] != mtime:
                    self._file_mtimes[yaml_file] = mtime
                    changed = True

            # If any file changed, invalidate all caches to force fresh reload.
            if changed:
                self.invalidate_cache()

        return changed

    # ------------------------------------------------------------------
    # 加载方法 / Load Methods
    # ------------------------------------------------------------------

    def load_taxonomy(self, name: str) -> DomainTaxonomy:
        """加载指定名称的分类体系定义（DomainTaxonomy）。

        从 `rules_dir/taxonomies/{name}.yaml` 文件中读取 YAML 内容，解析为 `DomainTaxonomy` Pydantic 模型并缓存。

        Args:
            name: Taxonomy 配置文件名（不含 `.yaml` 后缀），如 `"default"`。

        Returns:
            DomainTaxonomy: 解析后的分类体系模型实例。

        Raises:
            FileNotFoundError: 当对应的 YAML 配置文件不存在时抛出。
            yaml.YAMLError: 当 YAML 文件格式不合法时抛出。
            pydantic.ValidationError: 当配置数据与 DomainTaxonomy Schema 不匹配时抛出。
        """
        # Check cache first to avoid redundant file I/O and parsing.
        if name not in self._taxonomy_cache:
            # Construct file path: rules_dir/taxonomies/<name>.yaml
            path = self.rules_dir / "taxonomies" / f"{name}.yaml"
            # Read and parse YAML, then validate against Pydantic schema.
            data = yaml.safe_load(path.read_text(encoding="utf-8"))
            self._taxonomy_cache[name] = DomainTaxonomy.model_validate(data)
        return self._taxonomy_cache[name]

    def load_profile(self, domain: str) -> RuleProfile:
        """加载领域规则 Profile 定义（RuleProfile）。

        从 `rules_dir/domains/{domain}.yaml` 文件中读取 YAML 内容，解析为 `RuleProfile` Pydantic 模型并缓存。

        Args:
            domain: 领域包配置文件名（不含 `.yaml` 后缀），如 `"general-pii"` 或 `"medical"`。

        Returns:
            RuleProfile: 解析后的领域规则 Profile 模型实例。

        Raises:
            FileNotFoundError: 当对应的 YAML 配置文件不存在时抛出。
            yaml.YAMLError: 当 YAML 文件格式不合法时抛出。
            pydantic.ValidationError: 当配置数据与 RuleProfile Schema 不匹配时抛出。
        """
        # Check cache first.
        if domain not in self._profile_cache:
            # Construct file path: rules_dir/domains/<domain>.yaml
            path = self.rules_dir / "domains" / f"{domain}.yaml"
            # Read, parse, validate, and cache.
            data = yaml.safe_load(path.read_text(encoding="utf-8"))
            self._profile_cache[domain] = RuleProfile.model_validate(data)
        return self._profile_cache[domain]

    def load_standard(self, standard_id: str) -> StandardDef:
        """加载标准组合定义（StandardDef）。

        从 `rules_dir/standards/{standard_id}.yaml` 文件中读取 YAML 内容，解析为 `StandardDef` Pydantic 模型并缓存。

        Args:
            standard_id: 标准配置文件名（不含 `.yaml` 后缀），如 `"hipaa"` 或 `"gb-t-35273"`。

        Returns:
            StandardDef: 解析后的标准组合定义模型实例。

        Raises:
            FileNotFoundError: 当对应的 YAML 配置文件不存在时抛出。
            yaml.YAMLError: 当 YAML 文件格式不合法时抛出。
            pydantic.ValidationError: 当配置数据与 StandardDef Schema 不匹配时抛出。
        """
        # Check cache first.
        if standard_id not in self._standard_cache:
            # Construct file path: rules_dir/standards/<standard_id>.yaml
            path = self.rules_dir / "standards" / f"{standard_id}.yaml"
            # Read, parse, validate, and cache.
            data = yaml.safe_load(path.read_text(encoding="utf-8"))
            self._standard_cache[standard_id] = StandardDef.model_validate(data)
        return self._standard_cache[standard_id]

    # ------------------------------------------------------------------
    # 引擎构建 / Engine Building
    # ------------------------------------------------------------------

    def get_engine(
        self,
        domain: Optional[str] = None,
        standard: Optional[str] = None,
    ) -> ConfigurableRuleEngine:
        """获取或构建可配置规则引擎实例（ConfigurableRuleEngine），支持按缓存键复用。

        构建规则优先级：
        1. 若提供了 `standard` 参数，优先根据标准组合定义（StandardDef）构建引擎；
        2. 若仅提供了 `domain` 参数，根据特定领域 Profile（RuleProfile）与默认分类体系构建引擎；
        3. 若两者均未提供，构建包含默认通用领域包（如 `general-pii`, `medical`）的默认引擎。

        Args:
            domain: 领域标识（如 `"general-pii"`）。
            standard: 标准标识（如 `"compliance-std"`）。

        Returns:
            ConfigurableRuleEngine: 构建或从缓存中获取的规则引擎实例。
        """
        # Build cache key from domain and standard (or 'default' if not specified).
        cache_key = f"{domain or 'default'}:{standard or 'default'}"
        if cache_key not in self._engine_cache:
            # Measure engine construction time for Prometheus histogram.
            start_t = time.perf_counter()
            engine = self._build_engine(domain, standard)
            duration = time.perf_counter() - start_t
            # Record load duration metric (for SLO monitoring).
            DYNCLASSIFICATION_ENGINE_LOAD_DURATION.labels(
                domain=domain or "default",
                standard=standard or "default",
            ).observe(duration)
            # Cache the constructed engine for future reuse.
            self._engine_cache[cache_key] = engine
            # Update cache size gauge metric.
            DYNCLASSIFICATION_PROFILE_CACHE_SIZE.set(len(self._engine_cache))
        return self._engine_cache[cache_key]

    def get_composite_engine(
        self,
        domain: Optional[str] = None,
        standard: Optional[str] = None,
    ) -> CompositeRuleEngine:
        """获取或构建复合规则引擎实例（CompositeRuleEngine），支持按缓存键复用。

        复合规则引擎专门用于评估跨字段、多条件的组合规则（CompositeRuleDef）。

        Args:
            domain: 领域标识。
            standard: 标准标识。

        Returns:
            CompositeRuleEngine: 构建或从缓存中获取的复合规则引擎实例。
        """
        # Use the same cache key format as the main engine.
        cache_key = f"{domain or 'default'}:{standard or 'default'}"
        if cache_key not in self._composite_cache:
            engine = self._build_composite_engine(domain, standard)
            self._composite_cache[cache_key] = engine
        return self._composite_cache[cache_key]

    def _build_engine(
        self, domain: Optional[str], standard: Optional[str]
    ) -> ConfigurableRuleEngine:
        """内部方法：根据传入的 domain / standard 组合分发构建逻辑。

        Dispatch priority: standard > domain > default.
        """
        if standard:
            # Highest priority: build from standard combination definition.
            return self._build_engine_from_standard(standard)
        elif domain:
            # Second: build from a single domain profile.
            return self._build_engine_from_domain(domain)
        else:
            # Fallback: build default engine with preset domain packs.
            return self._build_default_engine()

    def _build_engine_from_standard(self, standard_id: str) -> ConfigurableRuleEngine:
        """内部方法：从标准组合定义（StandardDef）构建引擎。

        Steps:
        1. Load StandardDef configuration.
        2. Load the referenced DomainTaxonomy.
        3. Load all domain profiles referenced by the standard.
        4. Apply rule-level overrides if defined.
        5. Append extra rules as a temporary profile.
        6. Instantiate ConfigurableRuleEngine.
        """
        # Step 1: Load the standard combination definition.
        std_def = self.load_standard(standard_id)
        # Step 2: Load the taxonomy referenced by this standard.
        taxonomy = self.load_taxonomy(std_def.taxonomy)

        # Step 3: Load all domain profiles listed in the standard.
        profiles = [self.load_profile(d) for d in std_def.domains]

        # Step 4: Apply per-rule attribute overrides (e.g. change level/priority).
        if std_def.rule_overrides:
            profiles = self._apply_rule_overrides(profiles, std_def.rule_overrides)

        # Step 5: If standard defines extra rules, wrap them as an additional profile.
        if std_def.extra_rules or std_def.extra_downgrade_rules:
            extra_profile = RuleProfile(
                domain=f"{standard_id}_extra",
                rules=std_def.extra_rules,
                downgrade_rules=std_def.extra_downgrade_rules,
            )
            profiles.append(extra_profile)

        # Build a composite domain string for tagging (e.g. "general-pii,medical").
        domain_str = ",".join(std_def.domains) if std_def.domains else standard_id

        # Step 6: Construct and return the engine.
        return ConfigurableRuleEngine(
            taxonomy=taxonomy,
            profiles=profiles,
            domain=domain_str,
            standard_id=standard_id,
        )

    def _build_engine_from_domain(self, domain: str) -> ConfigurableRuleEngine:
        """内部方法：从单个领域包构建引擎。

        Uses the default taxonomy and a single domain profile.
        """
        # Load the default taxonomy (shared across single-domain engines).
        taxonomy = self.load_taxonomy("default")
        # Load the specific domain profile.
        profile = self.load_profile(domain)
        return ConfigurableRuleEngine(
            taxonomy=taxonomy,
            profiles=[profile],
            domain=domain,
            standard_id="",
        )

    def _build_default_engine(self) -> ConfigurableRuleEngine:
        """内部方法：构建默认引擎。

        Loads default taxonomy and attempts to load preset domain packs.
        Missing domain files are gracefully skipped (FileNotFoundError caught).
        """
        taxonomy = self.load_taxonomy("default")
        profiles: list[RuleProfile] = []

        # Attempt to load common preset domain packs; skip if not configured.
        for domain_name in ["general-pii", "medical"]:
            try:
                profiles.append(self.load_profile(domain_name))
            except FileNotFoundError:
                # Domain pack not available - skip gracefully.
                pass

        return ConfigurableRuleEngine(
            taxonomy=taxonomy,
            profiles=profiles,
            domain="default",
            standard_id="",
        )

    def _build_composite_engine(
        self, domain: Optional[str], standard: Optional[str]
    ) -> CompositeRuleEngine:
        """内部方法：从关联的 Profile 和标准定义中收集复合规则，构建复合规则引擎。

        Collects CompositeRuleDef from all relevant profiles based on
        the same dispatch logic as _build_engine (standard > domain > default).
        """
        composite_rules: list[CompositeRuleDef] = []
        domain_str = ""
        standard_str = ""

        if standard:
            # Standard mode: collect composite rules from all referenced domains.
            std_def = self.load_standard(standard)
            standard_str = standard
            domain_str = ",".join(std_def.domains)
            for d in std_def.domains:
                profile = self.load_profile(d)
                composite_rules.extend(profile.composite_rules)
            # Also include standard-specific extra composite rules.
            composite_rules.extend(std_def.extra_composite_rules)
        elif domain:
            # Single domain mode: collect from that domain's profile.
            domain_str = domain
            profile = self.load_profile(domain)
            composite_rules.extend(profile.composite_rules)
        else:
            # Default mode: collect from preset domain packs.
            domain_str = "default"
            for domain_name in ["general-pii", "medical"]:
                try:
                    profile = self.load_profile(domain_name)
                    composite_rules.extend(profile.composite_rules)
                except FileNotFoundError:
                    pass

        return CompositeRuleEngine(
            rules=composite_rules,
            domain=domain_str,
            standard_id=standard_str,
        )

    def _apply_rule_overrides(
        self, profiles: list[RuleProfile], overrides: dict[str, dict]
    ) -> list[RuleProfile]:
        """内部方法：应用规则级属性覆盖。

        遵循不可变原则（Immutability）：返回包含新规则副本的 `RuleProfile` 新列表，不直接修改原始 Profile 对象。

        Args:
            profiles: 原始 RuleProfile 实例列表。
            overrides: 覆盖规则字典，Key 为 rule.id，Value 为需要覆盖属性的字典。

        Returns:
            list[RuleProfile]: 应用覆盖更新后的 RuleProfile 新列表。
        """
        new_profiles: list[RuleProfile] = []
        for profile in profiles:
            new_rules: list[RuleDef] = []
            for rule in profile.rules:
                if rule.id in overrides:
                    # Create a modified copy: dump to dict, merge overrides, re-validate.
                    rule_data = rule.model_dump()
                    rule_data.update(overrides[rule.id])
                    new_rules.append(RuleDef.model_validate(rule_data))
                else:
                    # No override for this rule: keep original reference.
                    new_rules.append(rule)
            # Create a shallow copy of the profile with updated rules list.
            new_profile = profile.model_copy(update={"rules": new_rules})
            new_profiles.append(new_profile)
        return new_profiles

    # ------------------------------------------------------------------
    # 缓存管理 / Cache Management
    # ------------------------------------------------------------------

    def invalidate_cache(self) -> None:
        """清空所有内部缓存（Taxonomy、Profile、Standard、Engine 及 Composite 缓存）。

        常在热重载触发或手动重新加载规则配置时调用。
        同时重置 Prometheus 的 Profile 缓存大小指标为 0。
        """
        # Clear all cache layers to force fresh loading on next access.
        self._taxonomy_cache.clear()
        self._profile_cache.clear()
        self._standard_cache.clear()
        self._engine_cache.clear()
        self._composite_cache.clear()
        # Reset Prometheus gauge to reflect empty cache state.
        DYNCLASSIFICATION_PROFILE_CACHE_SIZE.set(0)

    # ------------------------------------------------------------------
    # 发现方法 / Discovery Methods
    # ------------------------------------------------------------------

    def list_taxonomies(self) -> list[str]:
        """列出规则配置根目录中所有可用的 Taxonomy 名称。

        Scans `rules_dir/taxonomies/*.yaml` and returns file stems.
        """
        tax_dir = self.rules_dir / "taxonomies"
        if not tax_dir.exists():
            return []
        # Extract filename stems (without .yaml extension) as taxonomy names.
        return [p.stem for p in tax_dir.glob("*.yaml")]

    def list_domains(self) -> list[str]:
        """列出规则配置根目录中所有可用的领域包（Domain）名称。

        Scans `rules_dir/domains/*.yaml` and returns file stems.
        """
        dom_dir = self.rules_dir / "domains"
        if not dom_dir.exists():
            return []
        return [p.stem for p in dom_dir.glob("*.yaml")]

    def list_standards(self) -> list[str]:
        """列出规则配置根目录中所有可用的标准组合（Standard）名称。

        Scans `rules_dir/standards/*.yaml` and returns file stems.
        """
        std_dir = self.rules_dir / "standards"
        if not std_dir.exists():
            return []
        return [p.stem for p in std_dir.glob("*.yaml")]
