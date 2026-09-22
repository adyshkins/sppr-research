package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"sppr-prototype/backend/internal/config"
	"sppr-prototype/backend/internal/db"
	"sppr-prototype/backend/internal/handler"
	"sppr-prototype/backend/internal/repository"
	"sppr-prototype/backend/internal/service"
)

func main() {
	cfg := config.Load()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}))

	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to create database pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{cfg.FrontendOrigin},
		AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		AllowCredentials: false,
	}))

	queries := db.New(pool)
	repo := repository.NewSQLCRepository(queries)
	workflowService := service.NewWorkflowService(repo)
	validationService := service.NewValidationService(repo)

	healthHandler := handler.NewHealthHandler(pool)
	workflowHandler := handler.NewWorkflowHandler(workflowService)
	caseHandler := handler.NewCaseHandler(queries)
	validationHandler := handler.NewValidationHandler(validationService)

	router.GET("/health", healthHandler.Health)
	router.GET("/ready", healthHandler.Ready)

	api := router.Group("/api")
	{
		api.POST("/workflow/process", workflowHandler.Process)
		api.GET("/cases", caseHandler.List)
		api.GET("/cases/:id", caseHandler.GetByID)
		api.GET("/cases/:id/trace", caseHandler.Trace)
		api.GET("/cases/:id/diagnostics", caseHandler.Diagnostics)
		api.GET("/cases/:id/scenarios", caseHandler.Scenarios)
		api.GET("/cases/:id/actions", caseHandler.Actions)
		api.POST("/cases/:id/submit-validation", validationHandler.Submit)
		api.POST("/cases/:id/expert-decision", validationHandler.Decide)
		api.POST("/cases/:id/execute", validationHandler.Execute)
		api.POST("/cases/:id/archive", validationHandler.Archive)
	}

	server := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("starting backend", "addr", server.Addr, "env", cfg.AppEnv)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	logger.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown failed", "error", err)
		os.Exit(1)
	}

	logger.Info("server stopped")
}
