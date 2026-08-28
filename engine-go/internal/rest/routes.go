// Package rest 提供 REST API 路由注册。
//
// 所有错误响应统一使用 middleware.AbortWithError 输出标准信封格式，
// 与 Python 引擎及其他 Go 微服务保持跨语言一致。
package rest

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/fengzhizi319/PrivShield/engine-go/internal/service"
	"github.com/fengzhizi319/PrivShield/pkg/middleware"
	"github.com/fengzhizi319/PrivShield/privacy-go-sdk/dp"
	"github.com/fengzhizi319/PrivShield/privacy-go-sdk/kano"
)

// RegisterRoutes 注册所有 REST API 路由
func RegisterRoutes(r *gin.Engine, svc *service.PrivacyService) {
	// 健康检查
	r.GET("/health", healthHandler)
	r.GET("/livez", livezHandler)
	r.GET("/readyz", readyzHandler)
	r.GET("/readyz/llm", readyzLLMHandler)

	// API v1
	v1 := r.Group("/api/v1")
	{
		// 掩码
		v1.POST("/mask", maskHandler(svc))
		v1.POST("/mask/record", maskRecordHandler(svc))
		v1.POST("/mask/batch", maskBatchHandler(svc))
		v1.POST("/mask/dataframe", maskDataFrameHandler(svc))

		// 差分隐私 — 基础
		v1.POST("/dp/count", dpCountHandler(svc))
		v1.POST("/dp/sum", dpSumHandler(svc))
		v1.POST("/dp/mean", dpMeanHandler(svc))
		v1.POST("/dp/histogram", dpHistogramHandler(svc))
		// 差分隐私 — 噪声
		v1.POST("/dp/noisy_count", noisyCountHandler(svc))
		v1.POST("/dp/noisy_sum", noisySumHandler(svc))
		v1.POST("/dp/noisy_mean", noisyMeanHandler(svc))
		v1.POST("/dp/noisy_histogram", dpNoisyHistogramHandler(svc))
		// 差分隐私 — 分块
		v1.POST("/dp/chunked_count", dpChunkedCountHandler(svc))
		v1.POST("/dp/chunked_sum", dpChunkedSumHandler(svc))
		v1.POST("/dp/chunked_mean", dpChunkedMeanHandler(svc))
		v1.POST("/dp/chunked_histogram", dpChunkedHistogramHandler(svc))
		// 差分隐私 — 向量/高级
		v1.POST("/dp/vector_sum", dpVectorSumHandler(svc))
		v1.POST("/dp/vector_mean", dpVectorMeanHandler(svc))
		v1.POST("/dp/aggregate", dpAggregateHandler(svc))
		v1.POST("/dp/adaptive_clip", dpAdaptiveClipHandler(svc))
		v1.POST("/dp/groupby", dpGroupByHandler(svc))

		// 本地差分隐私
		v1.POST("/ldp/randomized_response", randomizedResponseHandler(svc))
		v1.POST("/ldp/orr", orrHandler(svc))
		v1.POST("/ldp/perturb/binary", perturbBinaryBatchHandler(svc))
		v1.POST("/ldp/perturb/categorical", perturbCategoricalBatchHandler(svc))
		v1.POST("/ldp/estimate/binary", estimateBinaryFrequencyHandler(svc))
		v1.POST("/ldp/estimate/categorical", estimateCategoricalHistogramHandler(svc))

		// K-匿名
		v1.POST("/kano/anonymize", kAnonymizeHandler(svc))

		// 查询混淆
		v1.POST("/qol/obfuscate", obfuscateHandler(svc))
		v1.POST("/qol/obfuscate/batch", obfuscateBatchHandler(svc))

		// 动态分类
		v1.POST("/classify", classifyHandler(svc))
		v1.POST("/classify/batch", classifyBatchHandler(svc))

		// 医疗流水线
		v1.POST("/medical/sanitize", medicalSanitizeHandler(svc))
		v1.POST("/medical/sanitize/batch", medicalBatchHandler(svc))

		// HMAC 散列
		v1.POST("/hash/hmac", hashHMACHanlder(svc))

		// 预算查询与重置
		v1.GET("/budget", budgetHandler(svc))
		v1.POST("/budget/reset", budgetResetHandler(svc))
	}
}

// ──────────────────────────────────────────────
// 健康检查
// ──────────────────────────────────────────────

func healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "engine": "go"})
}

// ──────────────────────────────────────────────
// 掩码处理器
// ──────────────────────────────────────────────

func maskHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Field string `json:"field" binding:"required"`
			Value string `json:"value" binding:"required"`
			Type  string `json:"type" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		result, err := svc.MaskField(req.Type, req.Value)
		if err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "MASK_FAILED", "脱敏处理失败", err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"field": req.Field, "masked": result})
	}
}

func maskRecordHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Record map[string]string `json:"record" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		result := svc.MaskRecord(req.Record)
		c.JSON(http.StatusOK, gin.H{"result": result})
	}
}

func maskBatchHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Records []map[string]string `json:"records" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		results := svc.MaskBatch(req.Records)
		c.JSON(http.StatusOK, gin.H{"results": results})
	}
}

// ──────────────────────────────────────────────
// 差分隐私处理器
// ──────────────────────────────────────────────

func noisyCountHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Count   int     `json:"count" binding:"required"`
			Epsilon float64 `json:"epsilon" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		result, err := svc.NoisyCount(c.Request.Context(), req.Count, req.Epsilon)
		if err != nil {
			middleware.AbortWithError(c, http.StatusTooManyRequests, "BUDGET_EXHAUSTED", "隐私预算已耗尽", err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"noisy_count": result, "epsilon": req.Epsilon})
	}
}

func noisySumHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Values      []float64 `json:"values" binding:"required"`
			Epsilon     float64   `json:"epsilon" binding:"required"`
			Sensitivity float64   `json:"sensitivity" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		result, err := svc.NoisySum(c.Request.Context(), req.Values, req.Epsilon, req.Sensitivity)
		if err != nil {
			middleware.AbortWithError(c, http.StatusTooManyRequests, "BUDGET_EXHAUSTED", "隐私预算已耗尽", err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"noisy_sum": result, "epsilon": req.Epsilon})
	}
}

func noisyMeanHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Values    []float64 `json:"values" binding:"required"`
			Epsilon   float64   `json:"epsilon" binding:"required"`
			Delta     float64   `json:"delta" binding:"required"`
			ClipBound float64   `json:"clip_bound" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		result, err := svc.NoisyMean(c.Request.Context(), req.Values, req.Epsilon, req.Delta, req.ClipBound)
		if err != nil {
			middleware.AbortWithError(c, http.StatusTooManyRequests, "BUDGET_EXHAUSTED", "隐私预算已耗尽", err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"noisy_mean": result, "epsilon": req.Epsilon})
	}
}

// ──────────────────────────────────────────────
// LDP 处理器
// ──────────────────────────────────────────────

func randomizedResponseHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Value   bool    `json:"value" binding:"required"`
			Epsilon float64 `json:"epsilon" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		result := svc.RandomizedResponse(req.Value, req.Epsilon)
		c.JSON(http.StatusOK, gin.H{"result": result})
	}
}

func orrHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Value      int     `json:"value" binding:"required"`
			Epsilon    float64 `json:"epsilon" binding:"required"`
			DomainSize int     `json:"domain_size" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		result := svc.ORRResponse(req.Value, req.Epsilon, req.DomainSize)
		c.JSON(http.StatusOK, gin.H{"result": result})
	}
}

func perturbBinaryBatchHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Values  []int   `json:"values" binding:"required"`
			Epsilon float64 `json:"epsilon" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		result := svc.PerturbBinaryBatch(req.Values, req.Epsilon)
		c.JSON(http.StatusOK, gin.H{"result": result})
	}
}

func perturbCategoricalBatchHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Values     []string  `json:"values" binding:"required"`
			Categories []string  `json:"categories" binding:"required"`
			Epsilon    float64   `json:"epsilon" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		result := svc.PerturbCategoricalBatch(req.Values, req.Categories, req.Epsilon)
		c.JSON(http.StatusOK, gin.H{"result": result})
	}
}

func estimateBinaryFrequencyHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			ReportedValues []int   `json:"reported_values" binding:"required"`
			Epsilon        float64 `json:"epsilon" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		result := svc.EstimateBinaryFrequency(req.ReportedValues, req.Epsilon)
		c.JSON(http.StatusOK, gin.H{"frequency": result})
	}
}

func estimateCategoricalHistogramHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			ReportedValues []string  `json:"reported_values" binding:"required"`
			Categories     []string  `json:"categories" binding:"required"`
			Epsilon        float64   `json:"epsilon" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		result := svc.EstimateCategoricalHistogram(req.ReportedValues, req.Categories, req.Epsilon)
		c.JSON(http.StatusOK, gin.H{"histogram": result})
	}
}

// ──────────────────────────────────────────────
// K-匿名处理器
// ──────────────────────────────────────────────

func kAnonymizeHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Records  []map[string]string `json:"records" binding:"required"`
			QIFields []string            `json:"qi_fields" binding:"required"`
			K        int                 `json:"k" binding:"required,min=1"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		// 转换类型
		kanoRecords := make([]kano.Record, len(req.Records))
		for i, r := range req.Records {
			kanoRecords[i] = kano.Record(r)
		}
		result, err := svc.KAnonymize(kanoRecords, req.QIFields, req.K)
		if err != nil {
			middleware.AbortWithError(c, http.StatusInternalServerError, "KANONYMIZE_FAILED", "K-匿名处理失败", err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"records":     result.Records,
			"k":           result.K,
			"group_count": result.GroupCount,
		})
	}
}

// ──────────────────────────────────────────────
// 查询混淆处理器
// ──────────────────────────────────────────────

func obfuscateHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Query     string `json:"query" binding:"required"`
			NumDecoys int    `json:"num_decoys" binding:"required,min=1"`
			Domain    string `json:"domain" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		queries, realIdx := svc.ObfuscateQuery(req.Query, req.NumDecoys, req.Domain)
		c.JSON(http.StatusOK, gin.H{
			"queries":    queries,
			"real_index": realIdx,
		})
	}
}

func obfuscateBatchHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Queries   []string `json:"queries" binding:"required"`
			NumDecoys int      `json:"num_decoys" binding:"required,min=1"`
			Domain    string   `json:"domain" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		results := svc.ObfuscateQueryBatch(req.Queries, req.NumDecoys, req.Domain)
		c.JSON(http.StatusOK, gin.H{"results": results})
	}
}

// ──────────────────────────────────────────────
// 分类处理器
// ──────────────────────────────────────────────

func classifyHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Field string `json:"field" binding:"required"`
			Value string `json:"value" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		result := svc.Classify(req.Field, req.Value)
		c.JSON(http.StatusOK, result)
	}
}

func classifyBatchHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Records []map[string]string `json:"records" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		results := svc.ClassifyBatch(req.Records)
		c.JSON(http.StatusOK, gin.H{"classifications": results})
	}
}

// ──────────────────────────────────────────────
// 医疗流水线处理器
// ──────────────────────────────────────────────

func medicalSanitizeHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Record map[string]string `json:"record" binding:"required"`
			Domain string            `json:"domain" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		result, err := svc.SanitizeMedicalRecord(req.Record, req.Domain)
		if err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_DATASOURCE_ID", "未知或不支持的数据源标识", err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"result": result})
	}
}

func medicalBatchHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Records []map[string]string `json:"records" binding:"required"`
			Domain  string              `json:"domain" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		results, err := svc.SanitizeMedicalBatch(req.Records, req.Domain)
		if err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_DATASOURCE_ID", "未知或不支持的数据源标识", err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"results": results})
	}
}

// ──────────────────────────────────────────────
// HMAC 散列处理器
// ──────────────────────────────────────────────

func hashHMACHanlder(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Value string `json:"value" binding:"required"`
			Salt  string `json:"salt" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		result := svc.HashHMAC(req.Value, req.Salt)
		c.JSON(http.StatusOK, gin.H{"hash": result})
	}
}

// ──────────────────────────────────────────────
// 预算查询处理器
// ──────────────────────────────────────────────

func budgetHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := svc.BudgetStatus()
		c.JSON(http.StatusOK, status)
	}
}

func budgetResetHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := svc.BudgetReset()
		c.JSON(http.StatusOK, status)
	}
}

// ──────────────────────────────────────────────
// 健康检查（liveness / readiness）
// ──────────────────────────────────────────────

func livezHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func readyzHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func readyzLLMHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "llm": "not_loaded"})
}

// ──────────────────────────────────────────────
// DataFrame 脱敏处理器
// ──────────────────────────────────────────────

func maskDataFrameHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Data    []map[string]string `json:"data" binding:"required"`
			Columns []string            `json:"columns"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		results := make([]map[string]string, len(req.Data))
		for i, row := range req.Data {
			results[i] = svc.MaskRecord(row)
		}
		c.JSON(http.StatusOK, gin.H{"data": results})
	}
}

// ──────────────────────────────────────────────
// DP 基础处理器（count/sum/mean/histogram）
// ──────────────────────────────────────────────

func dpCountHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Count   int     `json:"count" binding:"required"`
			Epsilon float64 `json:"epsilon" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		result, err := svc.NoisyCount(c.Request.Context(), req.Count, req.Epsilon)
		if err != nil {
			middleware.AbortWithError(c, http.StatusTooManyRequests, "BUDGET_EXHAUSTED", "隐私预算已耗尽", err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"result": result, "epsilon": req.Epsilon})
	}
}

func dpSumHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Values      []float64 `json:"values" binding:"required"`
			Epsilon     float64   `json:"epsilon" binding:"required"`
			ClipLower   float64   `json:"clip_lower"`
			ClipUpper   float64   `json:"clip_upper"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		sensitivity := req.ClipUpper - req.ClipLower
		if sensitivity <= 0 {
			sensitivity = 1.0
		}
		result, err := svc.NoisySum(c.Request.Context(), req.Values, req.Epsilon, sensitivity)
		if err != nil {
			middleware.AbortWithError(c, http.StatusTooManyRequests, "BUDGET_EXHAUSTED", "隐私预算已耗尽", err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"result": result, "epsilon": req.Epsilon})
	}
}

func dpMeanHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Values    []float64 `json:"values" binding:"required"`
			Epsilon   float64   `json:"epsilon" binding:"required"`
			Delta     float64   `json:"delta" binding:"required"`
			ClipBound float64   `json:"clip_bound"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		if req.ClipBound <= 0 {
			req.ClipBound = 1.0
		}
		result, err := svc.NoisyMean(c.Request.Context(), req.Values, req.Epsilon, req.Delta, req.ClipBound)
		if err != nil {
			middleware.AbortWithError(c, http.StatusTooManyRequests, "BUDGET_EXHAUSTED", "隐私预算已耗尽", err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"result": result, "epsilon": req.Epsilon})
	}
}

func dpHistogramHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Values     []string `json:"values" binding:"required"`
			Categories []string `json:"categories" binding:"required"`
			Epsilon    float64  `json:"epsilon" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		trueCounts := make(map[string]int)
		for _, cat := range req.Categories {
			trueCounts[cat] = 0
		}
		for _, v := range req.Values {
			if _, ok := trueCounts[v]; ok {
				trueCounts[v]++
			}
		}
		result := dp.NoisyHistogram(trueCounts, req.Epsilon)
		c.JSON(http.StatusOK, gin.H{"result": result})
	}
}

func dpNoisyHistogramHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			TrueCounts map[string]int `json:"true_counts" binding:"required"`
			Epsilon    float64       `json:"epsilon" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		result := dp.NoisyHistogram(req.TrueCounts, req.Epsilon)
		c.JSON(http.StatusOK, gin.H{"result": result})
	}
}

// ──────────────────────────────────────────────
// DP 分块处理器
// ──────────────────────────────────────────────

func dpChunkedCountHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Chunks  [][]float64 `json:"chunks" binding:"required"`
			Epsilon float64     `json:"epsilon" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		total := 0
		for _, chunk := range req.Chunks {
			total += len(chunk)
		}
		result, err := svc.NoisyCount(c.Request.Context(), total, req.Epsilon)
		if err != nil {
			middleware.AbortWithError(c, http.StatusTooManyRequests, "BUDGET_EXHAUSTED", "隐私预算已耗尽", err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"result": result})
	}
}

func dpChunkedSumHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Chunks    [][]float64 `json:"chunks" binding:"required"`
			Epsilon   float64     `json:"epsilon" binding:"required"`
			ClipLower float64     `json:"clip_lower"`
			ClipUpper float64     `json:"clip_upper"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		var allValues []float64
		for _, chunk := range req.Chunks {
			allValues = append(allValues, chunk...)
		}
		sensitivity := req.ClipUpper - req.ClipLower
		if sensitivity <= 0 {
			sensitivity = 1.0
		}
		result, err := svc.NoisySum(c.Request.Context(), allValues, req.Epsilon, sensitivity)
		if err != nil {
			middleware.AbortWithError(c, http.StatusTooManyRequests, "BUDGET_EXHAUSTED", "隐私预算已耗尽", err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"result": result})
	}
}

func dpChunkedMeanHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Chunks    [][]float64 `json:"chunks" binding:"required"`
			Epsilon   float64     `json:"epsilon" binding:"required"`
			Delta     float64     `json:"delta" binding:"required"`
			ClipBound float64     `json:"clip_bound"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		var allValues []float64
		for _, chunk := range req.Chunks {
			allValues = append(allValues, chunk...)
		}
		if req.ClipBound <= 0 {
			req.ClipBound = 1.0
		}
		result, err := svc.NoisyMean(c.Request.Context(), allValues, req.Epsilon, req.Delta, req.ClipBound)
		if err != nil {
			middleware.AbortWithError(c, http.StatusTooManyRequests, "BUDGET_EXHAUSTED", "隐私预算已耗尽", err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"result": result})
	}
}

func dpChunkedHistogramHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Chunks     [][]string `json:"chunks" binding:"required"`
			Categories []string   `json:"categories" binding:"required"`
			Epsilon    float64    `json:"epsilon" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		trueCounts := make(map[string]int)
		for _, cat := range req.Categories {
			trueCounts[cat] = 0
		}
		for _, chunk := range req.Chunks {
			for _, v := range chunk {
				if _, ok := trueCounts[v]; ok {
					trueCounts[v]++
				}
			}
		}
		result := dp.NoisyHistogram(trueCounts, req.Epsilon)
		c.JSON(http.StatusOK, gin.H{"result": result})
	}
}

// ──────────────────────────────────────────────
// DP 向量/高级处理器
// ──────────────────────────────────────────────

func dpVectorSumHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Vectors [][]float64 `json:"vectors" binding:"required"`
			MaxNorm float64     `json:"max_norm" binding:"required"`
			Epsilon float64     `json:"epsilon" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		result := svc.DPVectorSum(req.Vectors, req.MaxNorm, req.Epsilon)
		c.JSON(http.StatusOK, gin.H{"noisy_vector": result})
	}
}

func dpVectorMeanHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Vectors [][]float64 `json:"vectors" binding:"required"`
			MaxNorm float64     `json:"max_norm" binding:"required"`
			Epsilon float64     `json:"epsilon" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		result := svc.DPVectorMean(req.Vectors, req.MaxNorm, req.Epsilon)
		c.JSON(http.StatusOK, gin.H{"mean_vector": result})
	}
}

func dpAggregateHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Rows    []map[string]string `json:"rows" binding:"required"`
			Epsilon float64             `json:"epsilon" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		result := map[string]any{
			"row_count": len(req.Rows),
			"epsilon":   req.Epsilon,
		}
		jsonBytes, _ := json.Marshal(result)
		c.JSON(http.StatusOK, gin.H{"results_json": string(jsonBytes)})
	}
}

func dpAdaptiveClipHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Values    []float64 `json:"values" binding:"required"`
			Epsilon   float64   `json:"epsilon" binding:"required"`
			InitialClip float64 `json:"initial_clip"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		clipUpper := req.InitialClip
		if clipUpper <= 0 {
			clipUpper = 1.0
		}
		clipLower := clipUpper * 0.1
		c.JSON(http.StatusOK, gin.H{"clip_lower": clipLower, "clip_upper": clipUpper})
	}
}

func dpGroupByHandler(svc *service.PrivacyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Rows      []map[string]string `json:"rows" binding:"required"`
			GroupCol  string              `json:"group_col" binding:"required"`
			TargetCol string              `json:"target_col" binding:"required"`
			Agg       string              `json:"agg" binding:"required"`
			Epsilon   float64             `json:"epsilon" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "请求参数校验失败", err.Error())
			return
		}
		groups := make(map[string][]float64)
		for _, row := range req.Rows {
			groupVal := row[req.GroupCol]
			var targetVal float64
			for _, ch := range row[req.TargetCol] {
				if (ch >= '0' && ch <= '9') || ch == '.' || ch == '-' {
					targetVal = targetVal*10 + float64(ch-'0')
				}
			}
			groups[groupVal] = append(groups[groupVal], targetVal)
		}
		result := make(map[string]float64)
		ctx := context.Background()
		for k, vals := range groups {
			switch req.Agg {
			case "count":
				noisy, _ := svc.NoisyCount(ctx, len(vals), req.Epsilon)
				result[k] = noisy
			case "sum":
				sum := 0.0
				for _, v := range vals {
					sum += v
				}
				noisy, _ := svc.NoisySum(ctx, []float64{sum}, req.Epsilon, 1.0)
				result[k] = noisy
			default:
				result[k] = float64(len(vals))
			}
		}
		jsonBytes, _ := json.Marshal(result)
		c.JSON(http.StatusOK, gin.H{"result_json": string(jsonBytes)})
	}
}
