// Command server is the entry point for the datasource-mgr module.
// Command server 是数据源管理模块的程序入口。
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

	"github.com/fengzhizi319/PrivShield/console/datasource-mgr/internal/agent"
	"github.com/fengzhizi319/PrivShield/console/datasource-mgr/internal/config"
	"github.com/fengzhizi319/PrivShield/console/datasource-mgr/internal/handlers"
)

func main() {
	cfg := config.Load()

	// ── Structured logger / 结构化日志 ────────────────────────
	logger := pkgconfig.SetupLogger(cfg.LogFormat, cfg.LogLevel)

	// ── DataSource store / 数据源存储 ─────────────────────────
	dsStore, err := initDSStore(cfg.DBPath, logger)
	if err != nil {
		log.Fatalf("failed to initialize datasource store: %v", err)
	}

	// ── Prometheus metrics / Prometheus 指标 ───────────────────
	mc := metrics.NewCollector("datasource-mgr")

	// ── Agent client / Agent 客户端 ────────────────────────────
	agentClient := agent.New(cfg)

	// ── HTTP Server ────────────────────────────────────────────
	gin.SetMode(gin.ReleaseMode)
	server := handlers.New(agentClient, cfg, dsStore, logger, mc)
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

		logger.Info("shutting down datasource-mgr server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("http server shutdown error", "error", err.Error())
		}
	}()

	logger.Info("datasource-mgr started",
		"addr", cfg.Address(),
		"agent_rest", cfg.AgentBaseURL(),
		"db_path", cfg.DBPath,
		"auth_enabled", cfg.APIKey != "",
	)

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("http server failed: %v", err)
	}
}

func initDSStore(dbPath string, logger *slog.Logger) (store.DataSourceStore, error) {
	if dbPath == "" {
		logger.Info("using in-memory datasource store (no persistence)")
		return memory.NewDataSourceStore(), nil
	}

	db, err := sqlite.Open(dbPath, logger)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	ds, err := sqlite.NewDataSourceStore(db)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("create datasource store: %w", err)
	}

	logger.Info("sqlite datasource store initialized", "path", dbPath)
	return ds, nil
}
