// Package main 提供 PrivShield Go 引擎的双协议（REST + gRPC）服务端入口。
//
// 架构：
//   - REST API (Gin)：面向外部调用方，端口 8079
//   - gRPC Server：面向内部微服务，端口 50051
//   - 信号处理：SIGINT/SIGTERM 优雅停机
//
// 环境变量：
//   - PRIVACY_REST_HOST / PRIVACY_REST_PORT：REST 监听地址
//   - PRIVACY_GRPC_HOST / PRIVACY_GRPC_PORT：gRPC 监听地址
//   - PRIVACY_LOG_LEVEL：日志级别（DEBUG/INFO/WARN/ERROR）
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/fengzhizi319/PrivShield/engine-go/internal/dynclassification"
	"github.com/fengzhizi319/PrivShield/engine-go/internal/grpcserver"
	"github.com/fengzhizi319/PrivShield/engine-go/internal/observability"
	"github.com/fengzhizi319/PrivShield/engine-go/internal/service"
	"github.com/fengzhizi319/PrivShield/privacy-go-sdk/budget"
	"github.com/fengzhizi319/PrivShield/privacy-go-sdk/dp"
	"github.com/fengzhizi319/PrivShield/privacy-go-sdk/masking"
)

// ──────────────────────────────────────────────
// 版本信息（编译时注入）
// ──────────────────────────────────────────────

var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

// ──────────────────────────────────────────────
// 配置
// ──────────────────────────────────────────────

type Config struct {
	RESTHost string
	RESTPort int
	GRPCPort int
	LogLevel string
}

func loadConfig() Config {
	return Config{
		RESTHost: getEnv("PRIVACY_REST_HOST", "0.0.0.0"),
		RESTPort: getEnvInt("PRIVACY_REST_PORT", 8079),
		GRPCPort: getEnvInt("PRIVACY_GRPC_PORT", 50051),
		LogLevel: getEnv("PRIVACY_LOG_LEVEL", "INFO"),
	}
}

// ──────────────────────────────────────────────
// 主入口
// ──────────────────────────────────────────────

func main() {
	cfg := loadConfig()

	// 初始化日志
	observability.InitLogger(cfg.LogLevel)

	slog.Info("Starting PrivShield Go Engine",
		"version", Version,
		"build_time", BuildTime,
		"git_commit", GitCommit,
	)

	// 初始化隐私预算会计
	budgetAcct := budget.NewBudgetAccountant(10.0, 1e-5, 3600)

	// 初始化规则引擎（示例规则）
	rules := []dynclassification.RuleDef{
		{
			ID:            "id_card",
			Level:         dynclassification.LevelSecret,
			Category:      "pii.identity",
			FieldPatterns: []string{`(?i)(id_?card|身份证|identity)`},
			Description:   "中国居民身份证",
		},
		{
			ID:            "phone",
			Level:         dynclassification.LevelConfidential,
			Category:      "pii.contact",
			FieldPatterns: []string{`(?i)(phone|mobile|手机|电话)`},
			Description:   "手机号码",
		},
		{
			ID:            "email",
			Level:         dynclassification.LevelConfidential,
			Category:      "pii.contact",
			FieldPatterns: []string{`(?i)(email|邮箱|邮件)`},
			Description:   "电子邮箱",
		},
		{
			ID:            "bank_card",
			Level:         dynclassification.LevelSecret,
			Category:      "pii.financial",
			FieldPatterns: []string{`(?i)(bank_?card|银行卡|信用卡)`},
			Description:   "银行卡号",
		},
	}

	ruleEngine, err := dynclassification.NewRuleEngine(rules)
	if err != nil {
		slog.Error("Failed to init rule engine", "err", err)
		os.Exit(1)
	}

	// 初始化 REST 服务器
	router := setupRESTRouter(ruleEngine, budgetAcct)

	// 启动 REST 服务器
	restAddr := fmt.Sprintf("%s:%d", cfg.RESTHost, cfg.RESTPort)
	restServer := &http.Server{
		Addr:         restAddr,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		slog.Info("REST server starting", "addr", restAddr)
		if err := restServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("REST server error", "err", err)
			os.Exit(1)
		}
	}()

	// 启动 gRPC 服务器
	grpcAddr := fmt.Sprintf("0.0.0.0:%d", cfg.GRPCPort)
	grpcLis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		slog.Error("gRPC listen failed", "err", err)
		os.Exit(1)
	}

	// 创建 PrivacyService 编排层
	svcCfg := service.DefaultConfig()
	svc, err := service.NewPrivacyService(svcCfg)
	if err != nil {
		slog.Error("Failed to init PrivacyService", "err", err)
		os.Exit(1)
	}

	grpcSrv := grpcserver.NewServer(svc)
	go func() {
		slog.Info("gRPC server starting", "addr", grpcAddr)
		if err := grpcSrv.Serve(grpcLis); err != nil {
			slog.Error("gRPC server error", "err", err)
		}
	}()

	slog.Info("Configuration summary",
		"rest_addr", restAddr,
		"grpc_port", cfg.GRPCPort,
		"log_level", cfg.LogLevel,
		"budget_total_epsilon", budgetAcct.TotalEpsilon(),
	)

	// 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	slog.Info("Shutdown signal received", "signal", sig)

	// 优雅停机
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := restServer.Shutdown(ctx); err != nil {
		slog.Error("REST server shutdown error", "err", err)
	}
	grpcSrv.GracefulStop()

	slog.Info("Server stopped gracefully")
}

// ──────────────────────────────────────────────
// REST 路由
// ──────────────────────────────────────────────

func setupRESTRouter(engine *dynclassification.RuleEngine, budgetAcct *budget.BudgetAccountant) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()

	// 中间件
	router.Use(gin.Recovery())
	router.Use(observability.RequestLogger())
	router.Use(observability.PrometheusMiddleware())

	// 健康检查
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// 隐私原语 API
	api := router.Group("/api/v1")

	// 掩码
	api.POST("/mask", func(c *gin.Context) {
		var req struct {
			Field string `json:"field" binding:"required"`
			Value string `json:"value" binding:"required"`
			Type  string `json:"type" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		var masked string
		switch req.Type {
		case "id_card":
			masked = masking.MaskIdCard(req.Value)
		case "phone":
			masked = masking.MaskPhone(req.Value)
		case "bank_card":
			masked = masking.MaskBankCard(req.Value)
		case "name":
			masked = masking.MaskChineseName(req.Value)
		case "email":
			masked = masking.MaskEmail(req.Value)
		case "address":
			masked = masking.MaskAddress(req.Value)
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown mask type"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"field":  req.Field,
			"masked": masked,
		})
	})

	// 差分隐私
	api.POST("/dp/noisy_count", func(c *gin.Context) {
		var req struct {
			Count   int     `json:"count" binding:"required"`
			Epsilon float64 `json:"epsilon" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if !budgetAcct.Consume(req.Epsilon, 0) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "budget exhausted"})
			return
		}

		noisy := dp.NoisyCount(req.Count, req.Epsilon)
		c.JSON(http.StatusOK, gin.H{
			"noisy_count": noisy,
			"epsilon":     req.Epsilon,
		})
	})

	api.POST("/dp/noisy_sum", func(c *gin.Context) {
		var req struct {
			Values    []float64 `json:"values" binding:"required"`
			Epsilon   float64   `json:"epsilon" binding:"required"`
			Sensitivity float64 `json:"sensitivity" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if !budgetAcct.Consume(req.Epsilon, 0) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "budget exhausted"})
			return
		}

		noisy := dp.NoisySum(req.Values, req.Epsilon, req.Sensitivity)
		c.JSON(http.StatusOK, gin.H{
			"noisy_sum": noisy,
			"epsilon":   req.Epsilon,
		})
	})

	// 动态分类
	api.POST("/classify", func(c *gin.Context) {
		var req struct {
			Records []map[string]string `json:"records" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		results := engine.ClassifyBatch(req.Records)
		c.JSON(http.StatusOK, gin.H{
			"classifications": results,
		})
	})

	// 预算查询
	api.GET("/budget", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"total_epsilon":     budgetAcct.TotalEpsilon(),
			"used_epsilon":      budgetAcct.UsedEpsilon(),
			"remaining_epsilon": budgetAcct.RemainingEpsilon(),
			"total_delta":       budgetAcct.TotalDelta(),
			"used_delta":        budgetAcct.UsedDelta(),
			"remaining_delta":   budgetAcct.RemainingDelta(),
		})
	})

	return router
}

// ──────────────────────────────────────────────
// 辅助函数
// ──────────────────────────────────────────────

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		fmt.Sscanf(v, "%d", &n)
		return n
	}
	return defaultVal
}
