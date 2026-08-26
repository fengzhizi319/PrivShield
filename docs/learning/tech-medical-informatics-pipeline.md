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

---

## 3. 医疗数据处理流水线架构 / Medical Data Processing Pipeline Architecture

文件 / File：[`engine/routers/medical.py`](engine/routers/medical.py) & [`engine/service.py`](engine/service.py)

### 3.1 端到端处理流程

```text
客户端提交医疗数据集 (POST /v1/medical/process)
    │
    ▼
┌────────────────────────────────────────────────────────┐
│  ① 输入规模校验 (DoS Protection)                       │
│  - 记录数 ≤ 500                                         │
│  - 单记录字段数 ≤ 100                                   │
│  - 单字段值长度 ≤ 100,000 字符                       │
└───────────────────────┬────────────────────────────────┘
                        │
                        ▼
┌────────────────────────────────────────────────────────┐
│  ② 三层漏斗分类分级 (3-Layer Classification Funnel)    │
│  - Layer 1: YAML 规则引擎 (ICD-10 编码匹配)          │
│  - Layer 2: Small-NER (实体识别)                        │
│  - Layer 3: LLM 仲裁 (语义理解)                        │
└───────────────────────┬────────────────────────────────┘
                        │
                        ▼
┌────────────────────────────────────────────────────────┐
│  ③ 敏感字段脱敏 (Field-Level Masking)                │
│  - 患者姓名: 保留姓氏，名字用 * 替换                │
│  - 身份证号: 保留前3后4，中间用 * 替换              │
│  - 手机号: 保留前3后4，中间用 * 替换                │
│  - 地址: 仅保留省市，详细地址用 * 替换              │
└───────────────────────┬────────────────────────────────┘
                        │
                        ▼
┌────────────────────────────────────────────────────────┐
│  ④ 图像智能打码 (Image Redaction)                    │
│  - 检测图像输入 (路径/Base64)                          │
│  - 沙箱校验 + 解压炸弹防御                           │
│  - 默认遮挡: 头部 16% + 底部 18%                     │
│  - SHA-256 匿名命名输出                              │
└───────────────────────┬────────────────────────────────┘
                        │
                        ▼
┌────────────────────────────────────────────────────────┐
│  ⑤ 审计日志写入 (Audit Log)                          │
│  - 操作类型、时间戳、操作者身份                    │
│  - 哈希链存证 (不可篡改)                              │
└───────────────────────┬────────────────────────────────┘
                        │
                        ▼
              返回分类报告 + 脱敏数据
```

### 3.2 DoS 防护：输入规模上限

医疗数据脱敏管线包含 NER 推理（百毫秒~秒级/字段）与复杂句法正则，无界输入可被用于 DoS 攻击。PrivShield 通过 Pydantic `field_validator` 在请求解析阶段即拦截超大 payload：

```python
_MAX_RECORDS = 500
_MAX_FIELDS_PER_RECORD = 100
_MAX_FIELD_VALUE_LENGTH = 100_000

class MedicalProcessRequest(BaseModel):
    records: list[dict[str, Any]] = Field(
        ..., max_length=_MAX_RECORDS,
        description="医疗与身份数据记录列表"
    )

    @field_validator("records")
    @classmethod
    def _cap_payload_size(cls, records):
        for rec in records:
            if len(rec) > _MAX_FIELDS_PER_RECORD:
                raise ValueError(f"单条记录字段数超过上限 {_MAX_FIELDS_PER_RECORD}")
            for key, value in rec.items():
                if isinstance(value, str) and len(value) > _MAX_FIELD_VALUE_LENGTH:
                    raise ValueError(f"字段 {key!r} 值长度超过上限")
        return records
```

> **学习要点**：Pydantic 的 `field_validator` 在请求体解析阶段执行，早于业务逻辑。这意味着超大 payload 在 JSON 反序列化后、进入脱敏管线前就被拦截，避免了无谓的 CPU 消耗。

---

## 4. 医学图像打码引擎深度解析 / Image Redaction Engine Deep Dive

文件 / File：[`engine/dynclassification/image_redaction.py`](engine/dynclassification/image_redaction.py)

### 4.1 输入类型识别与解析

医学图像输入可能以多种形式出现，引擎需要统一处理：

```python
IMAGE_FILE_EXTENSIONS = (
    ".jpg", ".jpeg", ".png", ".bmp", ".webp",
    ".tif", ".tiff", ".dcm", ".dicom"
)

def is_image_input(val_str: str) -> bool:
    """判断输入是否为图像（文件路径或 Base64 Data URI）。"""
    stripped = val_str.strip()
    return (
        len(stripped) < 512  # 路径长度上限
        and any(stripped.lower().endswith(ext) for ext in IMAGE_FILE_EXTENSIONS)
    ) or stripped.lower().startswith(("data:image/", "image:"))
```

### 4.2 四重安全防护栏

```python
def sanitize_image_input(val_str, output_dir=None, boxes=None):
    """智能医学图像遮盖脱敏处理。"""

    # ① 输入解析
    if val_str.strip().lower().startswith("data:image/"):
        # Base64 Data URI → 解码为字节流
        image_bytes = _decode_base64_data_uri(val_str)
    elif is_image_input(val_str):
        # 文件路径 → 沙箱校验
        path = Path(val_str)
        if not _is_path_allowed(path):
            raise PermissionError(f"Image path not in allowed directories: {val_str}")
        image_bytes = path.read_bytes()

    # ② 解压炸弹防御
    from PIL import Image
    Image.MAX_IMAGE_PIXELS = 25_000_000  # 2500 万像素上限

    img = Image.open(BytesIO(image_bytes))

    # ③ OOM 防护：超大分辨率自动下采样
    if img.width > 2048 or img.height > 2048:
        img.thumbnail((2048, 2048), Image.LANCZOS)

    # ④ 敏感区域遮挡
    width, height = img.size
    if boxes is None:
        # 默认遮挡区：头部 16% (患者姓名/科室) + 底部 18% (诊断/签名)
        boxes = [
            (0, 0, height * 0.16, width),          # 顶部条
            (height * 0.82, 0, height, width),       # 底部条
        ]

    draw = ImageDraw.Draw(img)
    for (ymin, xmin, ymax, xmax) in boxes:
        draw.rectangle([xmin, ymin, xmax, ymax], fill="black")

    # ⑤ 安全输出
    return _save_anonymized_image(img, output_dir)
```

### 4.3 匿名化输出与磁盘防满

```python
def _save_anonymized_image(img, output_dir):
    """以 SHA-256 命名保存匿名化图像，并自动清理旧文件。"""
    # 用原始内容的 SHA-256 前 12 位作为文件名（不可逆，无法还原原始图像）
    content_hash = hashlib.sha256(img.tobytes()).hexdigest()[:12]
    output_path = output_dir / f"sanitized_{content_hash}.png"
    img.save(output_path, "PNG")

    # 磁盘防满：自动清理超过 200 个旧文件
    _cleanup_old_sanitized_images(output_dir, max_files=200)
    return str(output_path)

def _cleanup_old_sanitized_images(output_dir: Path, max_files: int = 200):
    """当输出目录文件数超过上限时，按 mtime 删除最旧的文件。"""
    files = sorted(output_dir.glob("sanitized_*"), key=lambda p: p.stat().st_mtime)
    while len(files) > max_files:
        files.pop(0).unlink(missing_ok=True)
```

---

## 5. ICD-10 疾病编码区间算子 / ICD-10 Range Operator

文件 / File：[`engine/dynclassification/operators.py`](engine/dynclassification/operators.py)

ICD-10（国际疾病分类第十版）是全球通用的疾病编码标准。PrivShield 实现了 `icd10_range` 算子，自动判别高敏感病种：

```yaml
# rules/domains/healthcare.yaml
- id: medical_icd10_infectious
  category: 传染性重大疾病
  level: S4
  matchers:
    - target: field_value
      operator: icd10_range
      params:
        intervals: [["A00", "B99"]]  # 第一章：传染病和寄生虫病

- id: medical_icd10_mental
  category: 精神类疾病
  level: S4
  matchers:
    - target: field_value
      operator: icd10_range
      params:
        intervals: [["F00", "F99"]]  # 第五章：精神与行为障碍

- id: medical_icd10_rare_genetic
  category: 罕见遗传病
  level: S4
  matchers:
    - target: field_value
      operator: icd10_range
      params:
        intervals: [["Q00", "Q99"]]  # 第十七章：先天性畸形
```

**ICD-10 编码区间与敏感度映射**：

| ICD-10 章节 | 编码区间 | 敏感度 | 说明 |
|---|---|---|---|
| 传染病 | A00-B99 | S4 | HIV、结核、肝炎等 |
| 肿瘤 | C00-D49 | S4 | 恶性肿瘤信息 |
| 精神类 | F00-F99 | S4 | 精神疾病污名化风险 |
| 先天性畸形 | Q00-Q99 | S4 | 遗传病信息 |
| 损伤中毒 | S00-T88 | S3 | 可能涉及暴力/事故 |
| 其他 | 其余 | S2 | 一般医疗信息 |

---

## 6. HL7/FHIR 临床消息协议处理 / HL7 & FHIR Message Processing

### 6.1 HL7 v2 消息段解析

HL7 v2 是医院信息系统（HIS）之间通信的事实标准。消息由多个「段」（Segment）组成，每段包含多个「字段」（Field）：

```text
MSH|^~\&|HIS|Hospital|Lab|Hospital|20260825||ORU^R01|MSG001|P|2.3
PID|1||12345^^^MRN||张三||19900307|M|||||||||||
OBX|1|ST|GLU^Glucose||8.5|mmol/L|3.9-6.1|H|||F
```

PrivShield 对 HL7 消息中的 PID 段（患者标识）执行自动脱敏：

```text
原始: PID|1||12345^^^MRN||张三||19900307|M
脱敏: PID|1||HMAC(12345)^^^MRN||张*||1990****|M
```

### 6.2 FHIR 资源脱敏

FHIR（Fast Healthcare Interoperability Resources）是新一代医疗数据交换标准。PrivShield 对常见 FHIR 资源中的 PHI 字段执行脱敏：

| FHIR 资源 | PHI 字段 | 脱敏策略 |
|---|---|---|
| `Patient` | `name`, `identifier`, `birthDate` | 假名化/泛化 |
| `Patient` | `address`, `telecom` | 移除或泛化 |
| `Observation` | `subject.reference` | HMAC 重映射 |
| `DiagnosticReport` | `patient.reference` | HMAC 重映射 |

---

## 7. 合规标准对照 / Compliance Standards Mapping

| 合规要求 | PrivShield 实现 | 代码位置 |
|---|---|---|
| HIPAA Safe Harbor (美国) | 18 类 PHI 标识符全覆盖脱敏 | `masking.py` |
| DICOM PS 3.15 Annex E | 60+ DICOM 标签去标识化 | `image_redaction.py` |
| 四川 DB51 健康数据分级 | ICD-10 病种敏感度映射 | `healthcare.yaml` |
| 广东健康医疗数据规范 | 三层分类分级 + 自适应脱敏 | `funnel.py` |
| 《数据安全法》 | 六类数据分类 + 分级保护 | `taxonomies/` |

---

## 8. 运维实战命令 / Operations Commands

```bash
# 提交医疗数据进行分类分级与脱敏
curl -X POST http://localhost:8079/v1/medical/process \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $API_KEY" \
  -d '{
    "records": [
      {
        "patient_name": "张三",
        "id_card": "110101199003072345",
        "diagnosis": "急性感染性腹泻 A04.9",
        "mobile": "13800138000"
      }
    ]
  }'

# 提交医学图像进行智能打码
curl -X POST http://localhost:8079/v1/classify/field \
  -H "Content-Type: application/json" \
  -d '{"field_name": "xray_image", "field_value": "data/xray_chest.png"}'

# 运行医疗流水线测试
PYTHONPATH=. pytest tests/test_medical_pipeline.py -v
```

---

## 9. 扩展阅读 / Further Reading

1. **HIPAA Safe Harbor**：https://www.hhs.gov/hipaa/for-professionals/special-topics/research/safe-harbor
2. **DICOM PS 3.15**：https://dicom.nema.org/medical/dicom/current/output/chtml/part15/PS3.15.html
3. **ICD-10 浏览器**：https://icd.who.int/browse10/2019/en
4. **HL7 v2 标准**：https://www.hl7.org/implement/standards/product_brief.cfm?product_id=15
5. **FHIR R4 规范**：https://www.hl7.org/fhir/R4/
