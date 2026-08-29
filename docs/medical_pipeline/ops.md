# 医疗敏感数据治理流水线 — 运维与数据生成指南 (Ops Guide)

---

## 1. 模拟数据生成

`PrivShield` 提供高保真医疗数据生成脚本 [`scripts/data/generate_medical_data.py`](../../scripts/data/generate_medical_data.py)，用于生成测试与压测所需的高质量样本。

```bash
# 生成 100 条康养与医保测试数据
python scripts/data/generate_medical_data.py --output data/kangyang.csv --count 100

# 查看生成的数据字段
head -n 5 data/kangyang.csv
```

### 数据生成特性
- **真实身份证号校验**: 严格遵循 GB 11643-1999 校验位算法；
- **L4/L5 真实重症分布**: 包含肺癌、HIV、重度精神障碍、阿尔茨海默病等真实临床描述；
- **医学影像引用**: 自动关联 `[DICOM-CT: /radiology/...dcm]`。

---

## 2. 医学影像安全沙箱配置 (`PRIVACY_IMAGE_ALLOWED_DIRS`)

在处理 DICOM 等医学影像脱敏时，为防止任意文件读取与路径穿越攻击，网关与 Agent 强制开启路径白名单校验：

```bash
# 配置允许读取的影像文件目录白名单（多个路径以冒号分隔）
export PRIVACY_IMAGE_ALLOWED_DIRS="/data/radiology:/mnt/pacs/dicom"
```

- 尝试读取非白名单目录或包含 `../` 的非法路径将直接返回 HTTP 403 / `IMAGE_ACCESS_DENIED`。

---

## 3. 性能与容量规划

| 数据规模 | 单核处理耗时 | 16 核并发耗时 | 建议内存预留 |
|---|---|---|---|
| 1,000 条记录 | ~8.5 ms | ~1.2 ms | 32 MiB |
| 10,000 条记录 | ~85 ms | ~9.5 ms | 64 MiB |
| 100,000 条记录 | ~850 ms | ~92 ms | 128 MiB |

---

## 4. 故障排查 SOP

| 故障现象 | 根因定位 | 处理方案 |
|---|---|---|
| 医疗接口返回 `500 PROCESS_MEDICAL_FAILED` | 输入 JSON 结构不符合数组对象规范 | 确认请求体为 `{"records": [{...}]}` 格式。 |
| DICOM 处理报 `PATH_TRAVERSAL_DETECTED` | 影像文件路径指向了未授权的父级目录 | 检查 `PRIVACY_IMAGE_ALLOWED_DIRS` 白名单配置。 |
| 诊断敏感词脱敏不彻底 | 出现了全新的冷门临床简称 | 在 [`privacy-go-sdk/medical/rules.go`](../../privacy-go-sdk/medical/rules.go) 中补充新增词条。 |
