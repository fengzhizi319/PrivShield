// Package rest 提供 REST API 路由注册。
package rest

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/fengzhizi319/PrivShield/engine-go/internal/service"
	"github.com/fengzhizi319/PrivShield/privacy-go-sdk/kano"
)

// RegisterRoutes 注册所有 REST API 路由
func RegisterRoutes(r *gin.Engine, svc *service.PrivacyService) {
	// 健康检查
	r.GET("/health", healthHandler)

	// API v1
	v1 := r.Group("/api/v1")
	{
		// 掩码
		v1.POST("/mask", maskHandler(svc))
		v1.POST("/mask/record", maskRecordHandler(svc))
		v1.POST("/mask/batch", maskBatchHandler(svc))

		// 差分隐私
		v1.POST("/dp/noisy_count", noisyCountHandler(svc))
		v1.POST("/dp/noisy_sum", noisySumHandler(svc))
		v1.POST("/dp/noisy_mean", noisyMeanHandler(svc))

		// 本地差分隐私
		v1.POST("/ldp/randomized_response", randomizedResponseHandler(svc))
		v1.POST("/ldp/orr", orrHandler(svc))

		// K-匿名
		v1.POST("/kano/anonymize", kAnonymizeHandler(svc))

		// 查询混淆
		v1.POST("/qol/obfuscate", obfuscateHandler(svc))

		// 动态分类
		v1.POST("/classify", classifyHandler(svc))
		v1.POST("/classify/batch", classifyBatchHandler(svc))

		// 医疗流水线
		v1.POST("/medical/sanitize", medicalSanitizeHandler(svc))
		v1.POST("/medical/sanitize/batch", medicalBatchHandler(svc))

		// HMAC 散列
		v1.POST("/hash/hmac", hashHMACHanlder(svc))

		// 预算查询
		v1.GET("/budget", budgetHandler(svc))
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
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		result, err := svc.MaskField(req.Type, req.Value)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		result, err := svc.NoisyCount(c.Request.Context(), req.Count, req.Epsilon)
		if err != nil {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
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
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		result, err := svc.NoisySum(c.Request.Context(), req.Values, req.Epsilon, req.Sensitivity)
		if err != nil {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
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
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		result, err := svc.NoisyMean(c.Request.Context(), req.Values, req.Epsilon, req.Delta, req.ClipBound)
		if err != nil {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
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
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		result := svc.ORRResponse(req.Value, req.Epsilon, req.DomainSize)
		c.JSON(http.StatusOK, gin.H{"result": result})
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
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		// 转换类型
		kanoRecords := make([]kano.Record, len(req.Records))
		for i, r := range req.Records {
			kanoRecords[i] = kano.Record(r)
		}
		result, err := svc.KAnonymize(kanoRecords, req.QIFields, req.K)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		queries, realIdx := svc.ObfuscateQuery(req.Query, req.NumDecoys, req.Domain)
		c.JSON(http.StatusOK, gin.H{
			"queries":    queries,
			"real_index": realIdx,
		})
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
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		result := svc.SanitizeMedicalRecord(req.Record, req.Domain)
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
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		results := svc.SanitizeMedicalBatch(req.Records, req.Domain)
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
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
