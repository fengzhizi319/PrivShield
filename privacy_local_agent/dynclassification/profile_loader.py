"""Profile 加载器与缓存管理器 / Profile Loader & Cache Manager.

负责从 YAML 文件加载 Taxonomy、RuleProfile、StandardDef，
并根据 domain/standard 组合构建 ConfigurableRuleEngine 实例。
支持热加载（缓存失效）和引擎实例缓存。
"""

from __future__ import annotations

import threading
from pathlib import Path
from typing import Optional

import yaml


from .composite import CompositeRuleEngine
from .engine import ConfigurableRuleEngine
from .models import DomainTaxonomy
from .rule_schema import CompositeRuleDef, DowngradeRuleDef, RuleDef, RuleProfile, StandardDef


class ProfileLoader:
    """Profile 加载器与缓存管理器。

    从 YAML 文件加载分类体系、领域规则包和标准组合定义，
    并根据 domain/standard 构建规则引擎实例。

    目录结构约定：
        rules_dir/
        ├── taxonomies/     # 分类体系 YAML
        ├── domains/        # 领域规则包 YAML
        └── standards/      # 标准组合 YAML

    Attributes:
        rules_dir: 规则配置根目录。
    """

    def __init__(self, rules_dir: str | Path = "rules"):
        """初始化 Profile 加载器。

        Args:
            rules_dir: 规则配置根目录路径。
        """
        self.rules_dir = Path(rules_dir)
        self._lock = threading.RLock()
        self._taxonomy_cache: dict[str, DomainTaxonomy] = {}
        self._profile_cache: dict[str, RuleProfile] = {}
        self._standard_cache: dict[str, StandardDef] = {}
        self._engine_cache: dict[str, ConfigurableRuleEngine] = {}
        self._composite_cache: dict[str, CompositeRuleEngine] = {}
        self._file_mtimes: dict[Path, float] = {}

    def check_and_reload(self) -> bool:
        """检查 rules_dir 目录下的 YAML 文件修改时间，若有变动则自动重载。

        Returns:
            bool - 是否触发了重载。
        """
        if not self.rules_dir.exists():
            return False

        changed = False
        with self._lock:
            for yaml_file in self.rules_dir.glob("**/*.yaml"):
                mtime = yaml_file.stat().st_mtime
                if yaml_file not in self._file_mtimes or self._file_mtimes[yaml_file] != mtime:
                    self._file_mtimes[yaml_file] = mtime
                    changed = True

            if changed:
                self.invalidate_cache()

        return changed


    # ------------------------------------------------------------------
    # 加载方法 / Load Methods
    # ------------------------------------------------------------------

    def load_taxonomy(self, name: str) -> DomainTaxonomy:
        """加载分类体系定义。

        Args:
            name: taxonomy 文件名（不含 .yaml 后缀）。

        Returns:
            DomainTaxonomy 实例。

        Raises:
            FileNotFoundError: 文件不存在。
        """
        if name not in self._taxonomy_cache:
            path = self.rules_dir / "taxonomies" / f"{name}.yaml"
            data = yaml.safe_load(path.read_text(encoding="utf-8"))
            self._taxonomy_cache[name] = DomainTaxonomy.model_validate(data)
        return self._taxonomy_cache[name]

    def load_profile(self, domain: str) -> RuleProfile:
        """加载领域规则 Profile。

        Args:
            domain: 领域包文件名（不含 .yaml 后缀）。

        Returns:
            RuleProfile 实例。

        Raises:
            FileNotFoundError: 文件不存在。
        """
        if domain not in self._profile_cache:
            path = self.rules_dir / "domains" / f"{domain}.yaml"
            data = yaml.safe_load(path.read_text(encoding="utf-8"))
            self._profile_cache[domain] = RuleProfile.model_validate(data)
        return self._profile_cache[domain]

    def load_standard(self, standard_id: str) -> StandardDef:
        """加载标准组合定义。

        Args:
            standard_id: 标准文件名（不含 .yaml 后缀）。

        Returns:
            StandardDef 实例。

        Raises:
            FileNotFoundError: 文件不存在。
        """
        if standard_id not in self._standard_cache:
            path = self.rules_dir / "standards" / f"{standard_id}.yaml"
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
        """获取或构建规则引擎实例（带缓存）。

        Args:
            domain: 领域标识（与 standard 二选一）。
            standard: 标准标识（优先于 domain）。

        Returns:
            ConfigurableRuleEngine 实例。
        """
        cache_key = f"{domain or 'default'}:{standard or 'default'}"
        if cache_key not in self._engine_cache:
            engine = self._build_engine(domain, standard)
            self._engine_cache[cache_key] = engine
        return self._engine_cache[cache_key]

    def get_composite_engine(
        self,
        domain: Optional[str] = None,
        standard: Optional[str] = None,
    ) -> CompositeRuleEngine:
        """获取或构建复合规则引擎实例（带缓存）。

        Args:
            domain: 领域标识。
            standard: 标准标识。

        Returns:
            CompositeRuleEngine 实例。
        """
        cache_key = f"{domain or 'default'}:{standard or 'default'}"
        if cache_key not in self._composite_cache:
            engine = self._build_composite_engine(domain, standard)
            self._composite_cache[cache_key] = engine
        return self._composite_cache[cache_key]

    def _build_engine(
        self, domain: Optional[str], standard: Optional[str]
    ) -> ConfigurableRuleEngine:
        """根据 domain/standard 构建规则引擎。"""
        if standard:
            return self._build_engine_from_standard(standard)
        elif domain:
            return self._build_engine_from_domain(domain)
        else:
            return self._build_default_engine()

    def _build_engine_from_standard(self, standard_id: str) -> ConfigurableRuleEngine:
        """从标准组合构建引擎。"""
        std_def = self.load_standard(standard_id)
        taxonomy = self.load_taxonomy(std_def.taxonomy)

        # 加载所有领域包
        profiles = [self.load_profile(d) for d in std_def.domains]

        # 应用规则级覆盖
        if std_def.rule_overrides:
            profiles = self._apply_rule_overrides(profiles, std_def.rule_overrides)

        # 追加额外规则
        if std_def.extra_rules or std_def.extra_downgrade_rules:
            extra_profile = RuleProfile(
                domain=f"{standard_id}_extra",
                rules=std_def.extra_rules,
                downgrade_rules=std_def.extra_downgrade_rules,
            )
            profiles.append(extra_profile)

        # 确定 domain 标识
        domain_str = ",".join(std_def.domains) if std_def.domains else standard_id

        return ConfigurableRuleEngine(
            taxonomy=taxonomy,
            profiles=profiles,
            domain=domain_str,
            standard_id=standard_id,
        )

    def _build_engine_from_domain(self, domain: str) -> ConfigurableRuleEngine:
        """从单个领域包构建引擎。"""
        taxonomy = self.load_taxonomy("default")
        profile = self.load_profile(domain)
        return ConfigurableRuleEngine(
            taxonomy=taxonomy,
            profiles=[profile],
            domain=domain,
            standard_id="",
        )

    def _build_default_engine(self) -> ConfigurableRuleEngine:
        """构建默认引擎（加载所有通用领域包）。"""
        taxonomy = self.load_taxonomy("default")
        profiles: list[RuleProfile] = []

        # 尝试加载通用领域包
        for domain_name in ["general-pii", "medical"]:
            try:
                profiles.append(self.load_profile(domain_name))
            except FileNotFoundError:
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
        """构建复合规则引擎。"""
        composite_rules: list[CompositeRuleDef] = []
        domain_str = ""
        standard_str = ""

        if standard:
            std_def = self.load_standard(standard)
            standard_str = standard
            domain_str = ",".join(std_def.domains)
            for d in std_def.domains:
                profile = self.load_profile(d)
                composite_rules.extend(profile.composite_rules)
            composite_rules.extend(std_def.extra_composite_rules)
        elif domain:
            domain_str = domain
            profile = self.load_profile(domain)
            composite_rules.extend(profile.composite_rules)
        else:
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
        """应用规则级覆盖（返回新列表，不修改原对象）。"""
        new_profiles: list[RuleProfile] = []
        for profile in profiles:
            new_rules: list[RuleDef] = []
            for rule in profile.rules:
                if rule.id in overrides:
                    # 创建覆盖后的规则副本
                    rule_data = rule.model_dump()
                    rule_data.update(overrides[rule.id])
                    new_rules.append(RuleDef.model_validate(rule_data))
                else:
                    new_rules.append(rule)
            new_profile = profile.model_copy(update={"rules": new_rules})
            new_profiles.append(new_profile)
        return new_profiles

    # ------------------------------------------------------------------
    # 缓存管理 / Cache Management
    # ------------------------------------------------------------------

    def invalidate_cache(self) -> None:
        """清除所有缓存（热加载时调用）。"""
        self._taxonomy_cache.clear()
        self._profile_cache.clear()
        self._standard_cache.clear()
        self._engine_cache.clear()
        self._composite_cache.clear()

    # ------------------------------------------------------------------
    # 发现方法 / Discovery Methods
    # ------------------------------------------------------------------

    def list_taxonomies(self) -> list[str]:
        """列出所有可用的 taxonomy 名称。"""
        tax_dir = self.rules_dir / "taxonomies"
        if not tax_dir.exists():
            return []
        return [p.stem for p in tax_dir.glob("*.yaml")]

    def list_domains(self) -> list[str]:
        """列出所有可用的领域包名称。"""
        dom_dir = self.rules_dir / "domains"
        if not dom_dir.exists():
            return []
        return [p.stem for p in dom_dir.glob("*.yaml")]

    def list_standards(self) -> list[str]:
        """列出所有可用的标准名称。"""
        std_dir = self.rules_dir / "standards"
        if not std_dir.exists():
            return []
        return [p.stem for p in std_dir.glob("*.yaml")]
