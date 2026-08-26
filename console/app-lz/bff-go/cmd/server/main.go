package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fengzhizi319/PrivShield/console/app-lz/bff-go/internal/clients"
	"github.com/fengzhizi319/PrivShield/console/app-lz/bff-go/internal/config"
	"github.com/fengzhizi319/PrivShield/console/app-lz/bff-go/internal/handlers"
	"github.com/fengzhizi319/PrivShield/console/app-lz/bff-go/internal/runner"
)

func main() {
	cfg := config.Load()

	pool := clients.NewClientPool(cfg)
	testRunner := runner.NewTestRunner(pool)
	h := handlers.NewHandler(cfg, pool, testRunner)
	router := handlers.SetupRouter(h)

	addr := fmt.Sprintf("%s:%s", cfg.Host, cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	fmt.Println("==================================================================")
	fmt.Println(" 🚀 启动 PrivShield Console App-LZ BFF (调度之眼 聚合后端)")
	fmt.Println("==================================================================")
	fmt.Printf("  REST API:       http://%s\n", addr)
	fmt.Printf("  Service Hub:    %s\n", cfg.HubURL)
	fmt.Printf("  Datasource Mgr: %s\n", cfg.DatasourceURL)
	fmt.Printf("  Audit Log:      %s\n", cfg.AuditURL)
	fmt.Printf("  Agent Engine:   %s\n", cfg.AgentURL)
	fmt.Printf("  Static SPA:     %s\n", cfg.StaticDir)
	fmt.Println("==================================================================")

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down Console App-LZ BFF gracefully...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Console App-LZ BFF exited cleanly.")
}
