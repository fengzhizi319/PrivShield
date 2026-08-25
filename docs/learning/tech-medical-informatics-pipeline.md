# 医疗健康数据治理与 DICOM/HL7/图像脱敏技术指南 / Medical Informatics, DICOM, HL7 & Image Redaction Technical Guide

## 1. 技术简介 / Introduction

医疗健康数据兼具**极高临床价值**与**极高隐私敏感度**。在医疗大数据互联互通、医保结算、多中心科研及 AI 辅助诊疗模型训练中，必须满足国际 **HIPAA Safe Harbor** 及国内《中华人民共和国数据安全法》与 **健康医疗数据分类分级行业标准**（如四川 DB51、广东规范）。

`PrivShield` 构建了端到端的医疗健康数据安全治理流水线，覆盖结构化电子病历（EMR/EHR）、临床消息通信协议（HL7 v2 / FHIR）以及医学影像文件（DICOM、PACS 影像扫描件）。

```text
               多模态医疗健康输入 (Multi-Modal Medical Inputs)
                                      │
          ┌───────────────────────────┼───────────────────────────┐
          ▼                           ▼                           ▼
  结构化电子病历 (EMR)        临床消息报文 (HL7/FHIR)     医学影像 / 病理切片 (DICOM/JPG)
          │                           │                           │
          ▼                           ▼                           ▼
  三层漏斗分类分级              HL7 消息段解析与重构        DICOM Header PHI 去标识化
  (ICD-10 / 敏感病种识别)      (PID 患者标识段脱敏)         + 影像烧录文字像素级遮盖
          │                           │                           │
          └───────────────────────────┼───────────────────────────┘
                                      │
                                      ▼
                      合规安全数据发布 / 匿名化科研交付
```

---

## 2. 在本项目中的用法 / Usage in This Project

### 2.1 结构化病历三层分类分级与自动脱敏 / EMR Classification & Masking Pipeline

文件 / File：[`engine/routers/medical.py`](engine/routers/medical.py) & [`engine/service.py`](engine/service.py)

#### 医疗数据请求与资源耗尽保护 (DoS Protection)

```python
class MedicalProcessRequest(BaseModel):
    """医疗数据集治理请求（严格限制批量与字段长度防 DoS）。"""
    records: list[dict[str, Any]] = Field(..., max_length=500, description="医疗病历记录列表")

    @field_validator("records")
    @classmethod
    def _cap_payload_size(cls, records: list[dict[str, Any]]) -> list[dict[str, Any]]:
        for rec in records:
            if len(rec) > 100:
                raise ValueError("单条病历记录字段数不可超过 100")
            for k, v in rec.items():
                if isinstance(v, str) and len(v) > 100_000:
                    raise ValueError(f"字段 {k} 字符长度超限")
        return records
```

#### ICD-10 疾病编码区间算子

文件 / File：[`engine/dynclassification/operators.py`](engine/dynclassification/operators.py)

利用 `icd10_range` 算子自动判别传染病、精神类疾病及罕见遗传病等高敏感病种：

```yaml
# rules/domains/healthcare.yaml
- id: medical_icd10_infectious
  category: 传染性重大疾病
  level: S4
  matchers:
    - target: field_value
      operator: icd10_range
      params:
        intervals: [["A00", "B99"]]  # 国际疾病分类第一章：传染病和寄生虫病
```

---

### 2.2 医学影像与病例图像智能打码引擎 / Medical Image Redaction Engine

文件 / File：[`engine/dynclassification/image_redaction.py`](engine/dynclassification/image_redaction.py)

医学影像扫描件与放射科报告中，常在图像边缘硬编码患者姓名、检查号与医生签名（Burned-in Annotation）。`PrivShield` 提供了多层智能打码引擎：

```python
def sanitize_image_input(
    val_str: str,
    output_dir: Optional[Path] = None,
    boxes: Optional[list[tuple[float, float, float, float]]] = None,
) -> str:
    """智能医学图像与病例遮盖脱敏处理。
    
    1. 输入解析：支持本地文件绝对路径或 Base64 Data URI (data:image/...)
    2. 沙箱安全校验：防止路径穿越 (resolve + 白名单前缀匹配)
    3. 图像解压炸弹防御 (DecompressionBombError): 限制 MAX_IMAGE_PIXELS <= 25M
    4. 内存保护：超过 2048x2048 分辨率自动采用 Lanczos 高阶滤波下采样
    5. 区域遮挡：
       - 默认遮挡区：顶部 16% (患者姓名/科室) + 底部 18% (诊断结论/医生签名)
       - 自定义边界框 (Bounding Boxes): [(ymin, xmin, ymax, xmax), ...] 像素或比例坐标
    6. 安全输出：生成以 SHA-256 命名的匿名 PNG 文件并原子替换
    """
    ...
```

#### 图像安全防护四重护栏：

1. **路径沙箱隔离**：`_is_path_allowed()` 强制比对已规范化解析的绝对路径，杜绝通过软链接（Symlink）逃逸至宿主机敏感目录；
2. **像素炸弹防御**：配置 `Image.MAX_IMAGE_PIXELS = 25_000_000`，拦截利用超大图片反序列化耗尽内存的攻击（Zip-Bomb 类变种）；
3. **Lanczos 自适应下采样**：对超大分辨率影像先压缩再处理，保护 CPU/GPU 显存；
4. **Fail-Closed 错误回退**：对损坏或无法安全派生的图像，直接返回标记 `[IMAGE-REDACTION-FAILED]`，绝不泄露原始敏感图像。

---

### 2.3 DICOM 元数据 PHI 去标识化规范 / DICOM PHI De-Identification

根据 **PS 3.15 Annex E - Attribute Confidentiality Profiles**，`PrivShield` 在 DICOM 文件处理中对核心受保护健康信息（PHI）标签执行标准化脱敏：

| DICOM 标签 (Tag) | 属性名称 (Keyword) | 治理动作 (Action) |
|---|---|---|
| `(0010,0010)` | PatientName | 泛化或替换为哈希假名 |
| `(0010,0020)` | PatientID | HMAC-SHA256 假名化生成新 ID |
| `(0010,0030)` | PatientBirthDate | 仅保留年份（或按自适应分段泛化） |
| `(0010,0040)` | PatientSex | 保留或 `*` 抑制 |
| `(0008,0080)` | InstitutionName | 移除或替换为通用代号 |
| `(0020,000D)` | StudyInstanceUID | 密码学伪随机重映射 UID |
