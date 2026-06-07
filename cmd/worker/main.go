package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"

	"notify-engine/internal/config"
	"notify-engine/internal/delivery"
	"notify-engine/internal/queue"
	"notify-engine/internal/ratelimiter"
	"notify-engine/internal/repository"
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

	rdb := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB})
	defer func() { _ = rdb.Close() }()
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		logger.Error("redis connect failed", "error", err)
		os.Exit(1)
	}

	repo := repository.NewNotificationRepository(db)
	provider := delivery.NewWebhookProvider(cfg.Provider)
	limiter := ratelimiter.NewRedisRateLimiter(rdb, cfg.Worker.RateLimitPerSec)

	// Publisher (for scheduler to publish due notifications)
	publisher, err := queue.NewPublisher(cfg.RabbitMQ, logger)
	if err != nil {
		logger.Error("publisher create failed", "error", err)
		os.Exit(1)
	}
	defer func() { _ = publisher.Close() }()

	// Consumer
	consumer, err := queue.NewConsumer(cfg.RabbitMQ, cfg.Worker, repo, provider, limiter, logger)
	if err != nil {
		logger.Error("consumer create failed", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := consumer.Start(ctx); err != nil {
		logger.Error("consumer start failed", "error", err)
		os.Exit(1)
	}

	// Scheduler — polls for due scheduled notifications every 10s
	scheduler := queue.NewScheduler(repo, publisher, logger)
	go scheduler.Start(ctx)

	logger.Info("worker started",
		"concurrency", cfg.Worker.Concurrency,
		"rate_limit", cfg.Worker.RateLimitPerSec,
		"scheduler", "enabled",
	)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down worker...")
	cancel()
	scheduler.Stop()
	consumer.Shutdown()
	logger.Info("worker stopped")
}
