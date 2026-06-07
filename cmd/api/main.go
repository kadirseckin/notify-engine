package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"

	"notify-engine/internal/config"
	"notify-engine/internal/handler"
	"notify-engine/internal/middleware"
	"notify-engine/internal/queue"
	"notify-engine/internal/repository"
	"notify-engine/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)
	cfg := config.Load()

	db, err := sqlx.Connect("postgres", cfg.Database.DSN())
	if err != nil {
		logger.Error("db connect failed", "error", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	logger.Info("connected to PostgreSQL")

	rdb := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB})
	defer func() { _ = rdb.Close() }()
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		logger.Error("redis connect failed", "error", err)
		os.Exit(1)
	}
	logger.Info("connected to Redis")

	publisher, err := queue.NewPublisher(cfg.RabbitMQ, logger)
	if err != nil {
		logger.Error("rabbitmq connect failed", "error", err)
		os.Exit(1)
	}
	defer func() { _ = publisher.Close() }()
	logger.Info("connected to RabbitMQ")

	repo := repository.NewNotificationRepository(db)
	tmplRepo := repository.NewTemplateRepository(db)
	svc := service.NewNotificationService(repo, tmplRepo, publisher, logger)
	notifHandler := handler.NewNotificationHandler(svc)
	tmplHandler := handler.NewTemplateHandler(tmplRepo)
	healthHandler := handler.NewHealthHandler(db, rdb, cfg.RabbitMQ.URL, cfg)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), middleware.CorrelationID(), middleware.RequestLogger(logger))

	r.GET("/health", healthHandler.Health)
	r.GET("/metrics", healthHandler.Metrics)
	handler.RegisterSwaggerRoutes(r)
	v1 := r.Group("/api/v1")
	notifHandler.RegisterRoutes(v1)
	tmplHandler.RegisterRoutes(v1)

	srv := &http.Server{
		Addr: fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port), Handler: r,
		ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second,
	}

	go func() {
		logger.Info("API starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("forced shutdown", "error", err)
	}
	logger.Info("server stopped")
}
