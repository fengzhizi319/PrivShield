// Command server is the entry point for the audit-log module.
// Command server 是脱敏审计日志模块的程序入口。
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	pkgconfig "github.com/fengzhizi319/PrivShield/console/pkg/config"
	"github.com/fengzhizi319/PrivShield/console/pkg/metrics"
	"github.com/fengzhizi319/PrivShield/console/pkg/store"
	"github.com/fengzhizi319/PrivShield/console/pkg/store/memory"
	"github.com/fengzhizi319/PrivShield/console/pkg/store/sqlite"

	"github.com/fengzhizi319/PrivShield/console/audit-log/internal/agent"
	"github.com/fengzhizi319/PrivShield/console/audit-log/internal/config"
	"github.com/fengzhizi319/PrivShield/console/audit-log/internal/handlers"
)

func main() {
	cfg := config.Load()

	// ── Structured logger / 结构化日志 ────────────────────────
	logger := pkgconfig.SetupLogger(cfg.LogFormat, cfg.LogLevel)

	// ── Audit store / 审计存储 ─────────────────────────────────
	auditStore, err := initAuditStore(cfg.DBPath, logger)
	if err != nil {
		log.Fatalf("failed to initialize audit store: %v", err)
	}

	// ── Prometheus metrics / Prometheus 指标 ───────────────────
	mc := metrics.NewCollector("audit-log")

	// ── Agent client / Agent 客户端 ────────────────────────────
	agentClient := agent.New(cfg)

	// ── HTTP Server ────────────────────────────────────────────
	gin.SetMode(gin.ReleaseMode)
	server := handlers.New(agentClient, cfg, auditStore, logger, mc)
	router := gin.New()
	server.RegisterRoutes(router)

	srv := &http.Server{
		Addr:              cfg.Address(),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		logger.Info("shutting down audit-log server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("http server shutdown error", "error", err.Error())
		}
	}()

	logger.Info("audit-log started",
		"addr", cfg.Address(),
		"agent_rest", cfg.AgentBaseURL(),
		"db_path", cfg.DBPath,
		"auth_enabled", cfg.APIKey != "",
	)

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("http server failed: %v", err)
	}
}

func initAuditStore(dbPath string, logger *slog.Logger) (store.AuditStore, error) {
	if dbPath == "" {
		logger.Info("using in-memory audit store (no persistence)")
		return memory.NewAuditStore(), nil
	}

	db, err := sqlite.Open(dbPath, logger)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	as, err := sqlite.NewAuditStore(db)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("create audit store: %w", err)
	}

	logger.Info("sqlite audit store initialized", "path", dbPath)
	return as, nil
}
