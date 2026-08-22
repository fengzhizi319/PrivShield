// Command server is the entry point for the datasource-mgr module.
// Command server 是数据源管理模块的程序入口。
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

	"github.com/gin-gonic/gin"

	"github.com/fengzhizi319/PrivShield/console/datasource-mgr/internal/agent"
	"github.com/fengzhizi319/PrivShield/console/datasource-mgr/internal/config"
	"github.com/fengzhizi319/PrivShield/console/datasource-mgr/internal/handlers"
)

func main() {
	cfg := config.Load()
	agentClient := agent.New(cfg)

	gin.SetMode(gin.ReleaseMode)
	server := handlers.New(agentClient, cfg)
	router := gin.New()
	server.RegisterRoutes(router)

	srv := &http.Server{
		Addr:    cfg.Address(),
		Handler: router,
	}

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("http server shutdown error: %v", err)
		}
	}()

	fmt.Printf("Datasource Manager listening on http://%s\n", cfg.Address())
	fmt.Printf("Upstream agent REST: %s\n", cfg.AgentBaseURL())

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("http server failed: %v", err)
	}
}
