# 调度之眼 (Console App-LZ) — 调度中枢全景测试与治理控制台

> **数联天下 · 数盾 (`PrivShield`)** 数据服务调度中枢 (`services/service-hub`) 全景测试、观测与微服务治理前端控制台。

---

## 1. 项目简介

`console/app-lz` 是专为 `services/service-hub` 打造的集成测试与微服务网格全景工作台，打通四大核心服务：
- **`services/service-hub`** (:8082 / :50052)：数据流水线调度中枢
- **`services/datasource-mgr`** (:8083 / :50053)：模拟数据源管理与切片探查
- **`services/audit-log`** (:8084 / :50054)：不可篡改 SHA-256 / Merkle 树审计存证
- **`engine` Agent** (:8079 / :50051)：动态分类分级与隐私计算引擎

前端设计风格与 `console/web` 保持高度一致（React 18 + TypeScript + Vite + Tailwind CSS + Lucide Icons），提供 **7 大核心工作台**。

---

## 2. 7 大核心工作台

1. **集群拓扑与健康矩阵 (Topology & Mesh Health)**：4 服务实时 RTT、探针与连通性自检。
2. **6 阶段流水线动态大屏 (6-Stage Pipeline Visualizer)**：`Ingest` ➔ `Fetch` ➔ `Classify` ➔ `Desensitize` ➔ `Return` ➔ `Audit` 流转动效与数据脱敏前后对比。
3. **任务生命周期与租约看板 (Task Lifecycle & Lease Inspector)**：任务检索、阶段耗时时间线，以及 Phase B PostgreSQL 原子租约 (`FOR UPDATE SKIP LOCKED`) 争抢监控。
4. **一键全场景自动化测试执行器 (One-Click E2E Test Suite Runner)**：内置 7 大自动化测试套件（TS-01~TS-07），图形化执行与实时断言报告输出。
5. **数据源资产探查器 (Datasource Explorer)**：医保/康养数据源在线浏览与切片采样。
6. **不可篡改审计验真 (Audit Log & Merkle Verifier)**：存证流水查看与 Merkle 树防篡改在线验真。
7. **性能监控与耗时直方图 (Metrics & Performance Analyzer)**：实时 QPS、6 阶段耗时瀑布图与 P50/P90/P95/P99 延迟分位数分析。

---

## 3. 架构与规范文档

- [系统架构与全景设计文档 (`docs/design.md`)](file:///home/charles/code/PrivShield/console/app-lz/docs/design.md)
- [API 接口与数据契约规范 (`docs/api.md`)](file:///home/charles/code/PrivShield/console/app-lz/docs/api.md)
- [产品需求与测试套件说明书 (`docs/prd.md`)](file:///home/charles/code/PrivShield/console/app-lz/docs/prd.md)
