// Package agent provides an HTTP client to the upstream PrivShield agent.
// Package agent 封装与上游 PrivShield Python 隐私计算核心引擎（Sidecar / Agent）交互的 HTTP 客户端。
//
// 架构设计：
// 本客户端作为轻量级薄封装（Thin Wrapper），底层复用 pkg/agent.Client 共享基础库，
// 天然享有以下企业级能力：
// 1. 多 Agent 实例自动负载均衡与高可用健康探测（BaseURLs）；
// 2. 自动注入 Authorization Bearer API Key 安全鉴权头；
// 3. 熔断器模式（Circuit Breaker）与超时自动重试，防御下游级联故障；
// 4. 专职提供动态分类分级（/v1/dynclassification/*）与隐私脱敏算子（/v1/privacy/*）调用。
package agent

import (
	"context"
	"encoding/json"

	pkgagent "github.com/fengzhizi319/PrivShield/pkg/agent"
	"github.com/fengzhizi319/PrivShield/services/service-hub/internal/config"
)

// Client wraps the shared agent client with service-hub-specific endpoints.
// Client 结构体在底层共享 pkgagent.Client 基础上，扩展装配 service-hub 流水线特需的领域调用方法。
type Client struct {
	*pkgagent.Client
}

// New creates a new agent client from the given config.
// New 函数根据 service-hub 的运行配置构造并初始化 Agent 客户端实例。
// 执行步骤：
// 1. 从 Config 提取所有 Agent URL 列表（支持单节点与多节点配置）及 APIKey；
// 2. 初始化底层 pkgagent.Client 实例并绑定熔断重试机制；
// 3. 返回封装后的 *Client 实例。
func New(cfg *config.Config) *Client {
	shared := pkgagent.New(pkgagent.Config{
		BaseURLs: cfg.AgentBaseURLs(),
		APIKey:   cfg.AgentAPIKey,
	})
	return &Client{Client: shared}
}

// Classify sends data to the dynamic classification endpoint.
// Classify 方法将待评估的数据对象发送至 PrivShield Agent 的动态分类分级评估接口。
//
// 执行逻辑：
// 1. 将传入的原始 payload 包装为 Agent 协议标准的 {"record": payload} 结构；
// 2. 发起 HTTP POST 请求调用 /v1/dynclassification/eval_record 端点；
// 3. 经过 Agent「三层漏斗（规则->NER->LLM）」决策后，返回各字段的敏感级别（L1~L5）与分类标签。
//
// Agent 端点规范：
// - URL: POST /v1/dynclassification/eval_record
// - 请求结构: {"record": {"field1": "val1", ...}}
// - 响应结构: {"fields": {"field1": {"level": "L3", "category": "PII", ...}}, "overall_level": "L3"}
func (c *Client) Classify(ctx context.Context, payload any) (map[string]any, error) {
	wrapped := map[string]any{
		"record": payload,
	}
	return c.Post(ctx, "/v1/dynclassification/eval_record", wrapped)
}

// Mask sends data to the field-level masking endpoint.
// Mask 方法将原始载荷批量发送至字段级脱敏接口。
//
// 执行逻辑：
// 1. 发起 HTTP POST 请求调用 /v1/privacy/mask 端点；
// 2. Agent 依据内置规则或动态策略对字段名已知的敏感数据执行掩码或置换；
// 3. 返回脱敏后的结构化数据字典。
func (c *Client) Mask(ctx context.Context, payload any) (map[string]any, error) {
	return c.Post(ctx, "/v1/privacy/mask", payload)
}

// MaskRecord sends a full record to the record-level masking endpoint.
// MaskRecord 方法将整条单条数据记录（键值对 map[string]string）发送至记录级脱敏接口。
//
// 执行逻辑：
// 1. 构造包含 record 键值映射与可选上下文 context 的请求体；
// 2. 发起 HTTP POST 请求调用 /v1/privacy/mask_record 端点；
// 3. Agent 结合字段名与值内容完成自适应动态脱敏并返回脱敏记录。
func (c *Client) MaskRecord(ctx context.Context, record map[string]string) (map[string]any, error) {
	payload := map[string]any{
		"record":  record,
		"context": "",
	}
	return c.Post(ctx, "/v1/privacy/mask_record", payload)
}

// MedicalProcessResult holds the response from engine's /v1/medical/process endpoint.
// MedicalProcessResult 医疗流水线一次调用的返回结构：分类分级报告 + 脱敏合规数据 + 汇总统计。
type MedicalProcessResult struct {
	ClassificationReport []map[string]any `json:"classification_report"`
	SanitizedData        []map[string]any `json:"sanitized_data"`
	Summary              map[string]any   `json:"summary"`
}

// ProcessMedical sends records to the engine's medical pipeline endpoint.
// ProcessMedical 将批量记录发送至 engine /v1/medical/process 专业医疗流水线，
// 一次 HTTP 调用同时完成 3-Layer 分类分级 + L4/L5 高敏文本剥离 + PII 强掩码 +
// ICD-10 编码脱敏 + 诊断残留清除，替代原先 classify + desensitize 两步分离调用。
//
// Agent 端点规范：
// - URL: POST /v1/medical/process
// - 请求结构: {"records": [{...}, {...}, ...]}
// - 响应结构: {"classification_report": [...], "sanitized_data": [...], "summary": {...}}
func (c *Client) ProcessMedical(ctx context.Context, records []map[string]any) (*MedicalProcessResult, error) {
	payload := map[string]any{
		"records": records,
	}
	result, err := c.Post(ctx, "/v1/medical/process", payload)
	if err != nil {
		return nil, err
	}

	// 将通用 map 解析为结构化结果
	mpr := &MedicalProcessResult{}
	if report, ok := result["classification_report"]; ok {
		if items, ok := report.([]map[string]any); ok {
			mpr.ClassificationReport = items
		}
	}
	if sanitized, ok := result["sanitized_data"]; ok {
		if items, ok := sanitized.([]map[string]any); ok {
			mpr.SanitizedData = items
		}
	}
	if summary, ok := result["summary"]; ok {
		if m, ok := summary.(map[string]any); ok {
			mpr.Summary = m
		}
	}
	return mpr, nil
}

// ToRecords normalizes a generic payload into []map[string]any for ProcessMedical.
// ToRecords 将通用载荷（单条 map、切片、JSON 字符串）统一转换为记录切片。
func ToRecords(payload any) []map[string]any {
	switch v := payload.(type) {
	case []map[string]any:
		return v
	case map[string]any:
		return []map[string]any{v}
	case []any:
		records := make([]map[string]any, 0, len(v))
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				records = append(records, m)
			}
		}
		return records
	case string:
		var parsed any
		if err := json.Unmarshal([]byte(v), &parsed); err == nil {
			return ToRecords(parsed)
		}
	}
	return nil
}
