package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"notify-engine/internal/config"
	"notify-engine/internal/delivery"
	"notify-engine/internal/model"
	"notify-engine/internal/ratelimiter"
	"notify-engine/internal/repository"
)

type Consumer struct {
	conn      *amqp.Connection
	channel   *amqp.Channel
	cfg       config.RabbitMQConfig
	workerCfg config.WorkerConfig
	repo      repository.NotificationRepository
	provider  delivery.Provider
	limiter   ratelimiter.RateLimiter
	logger    *slog.Logger
	done      chan struct{}
	wg        sync.WaitGroup
}

func NewConsumer(cfg config.RabbitMQConfig, workerCfg config.WorkerConfig, repo repository.NotificationRepository,
	provider delivery.Provider, limiter ratelimiter.RateLimiter, logger *slog.Logger) (*Consumer, error) {
	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq connect: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("rabbitmq channel: %w", err)
	}
	if err := ch.Qos(cfg.PrefetchCount, 0, false); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("set qos: %w", err)
	}
	return &Consumer{conn: conn, channel: ch, cfg: cfg, workerCfg: workerCfg,
		repo: repo, provider: provider, limiter: limiter, logger: logger, done: make(chan struct{})}, nil
}

func (c *Consumer) Start(ctx context.Context) error {
	for _, ch := range []model.Channel{model.ChannelSMS, model.ChannelEmail, model.ChannelPush} {
		qName := fmt.Sprintf("%s.%s", c.cfg.QueuePrefix, ch)
		deliveries, err := c.channel.Consume(qName, fmt.Sprintf("worker-%s", ch), false, false, false, false, nil)
		if err != nil {
			return fmt.Errorf("consume %s: %w", qName, err)
		}
		for i := 0; i < c.workerCfg.Concurrency; i++ {
			c.wg.Add(1)
			go c.worker(ctx, string(ch), deliveries, i)
		}
		c.logger.Info("consuming", "queue", qName, "workers", c.workerCfg.Concurrency)
	}
	return nil
}

func (c *Consumer) worker(ctx context.Context, channel string, deliveries <-chan amqp.Delivery, id int) {
	defer c.wg.Done()
	l := c.logger.With("channel", channel, "worker_id", id)
	for {
		select {
		case <-ctx.Done():
			l.Info("worker shutting down")
			return
		case <-c.done:
			return
		case msg, ok := <-deliveries:
			if !ok {
				return
			}
			c.processMessage(ctx, msg, channel, l)
		}
	}
}

func (c *Consumer) processMessage(ctx context.Context, msg amqp.Delivery, channel string, logger *slog.Logger) {
	var n model.Notification
	if err := json.Unmarshal(msg.Body, &n); err != nil {
		logger.Error("unmarshal failed", "error", err)
		if err := msg.Nack(false, false); err != nil {
			logger.Error("nack failed", "error", err)
		}
		return
	}
	logger = logger.With("notification_id", n.ID)
	logger.Info("processing")

	// Read fresh state from DB (retry_count may have been incremented by previous attempts)
	if fresh, err := c.repo.GetByID(ctx, n.ID); err == nil {
		n.RetryCount = fresh.RetryCount
	}

	_ = c.repo.UpdateStatus(ctx, n.ID, model.StatusSending, nil, nil)

	// Rate limit wait loop
	for {
		allowed, wait, err := c.limiter.Allow(ctx, channel)
		if err != nil {
			logger.Error("rate limiter error", "error", err)
			break
		}
		if allowed {
			break
		}
		logger.Debug("rate limited", "wait_ms", wait.Milliseconds())
		select {
		case <-ctx.Done():
			if err := msg.Nack(false, true); err != nil {
				logger.Error("nack failed", "error", err)
			}
			return
		case <-time.After(wait):
		}
	}

	req := delivery.DeliveryRequest{To: n.Recipient, Channel: string(n.Channel), Content: n.Content}
	if n.Subject != nil {
		req.Subject = *n.Subject
	}

	start := time.Now()
	resp, err := c.provider.Send(ctx, req)
	latency := time.Since(start)

	if err != nil {
		c.handleFailure(ctx, msg, &n, err, logger)
		return
	}

	_ = c.repo.UpdateStatus(ctx, n.ID, model.StatusSent, &resp.MessageID, nil)
	if err := msg.Ack(false); err != nil {
		logger.Error("ack failed", "error", err)
	}
	logger.Info("delivered", "provider_msg_id", resp.MessageID, "latency_ms", latency.Milliseconds())
}

func (c *Consumer) handleFailure(ctx context.Context, msg amqp.Delivery, n *model.Notification, sendErr error, logger *slog.Logger) {
	errMsg := sendErr.Error()
	retryable := true
	var pe *delivery.ProviderError
	if errors.As(sendErr, &pe) {
		retryable = pe.Retryable
	}

	_ = c.repo.IncrementRetry(ctx, n.ID, errMsg)
	currentRetry := n.RetryCount + 1

	if !retryable || currentRetry >= c.workerCfg.MaxRetries {
		reason := "permanent failure"
		if currentRetry >= c.workerCfg.MaxRetries {
			reason = "max retries exceeded"
		}
		logger.Warn("sending to DLQ", "reason", reason, "error", errMsg, "retry", currentRetry)
		_ = c.repo.UpdateStatus(ctx, n.ID, model.StatusFailed, nil, &errMsg)
		if err := msg.Nack(false, false); err != nil {
			logger.Error("nack failed", "error", err)
		}
		return
	}

	backoff := time.Duration(float64(c.workerCfg.RetryBaseDelay) * math.Pow(2, float64(currentRetry-1)))
	if backoff > 60*time.Second {
		backoff = 60 * time.Second
	}
	logger.Info("retrying", "error", errMsg, "retry", currentRetry, "backoff_sec", backoff.Seconds())
	_ = c.repo.UpdateStatus(ctx, n.ID, model.StatusQueued, nil, &errMsg)

	select {
	case <-ctx.Done():
		if err := msg.Nack(false, true); err != nil {
			logger.Error("nack failed", "error", err)
		}
	case <-time.After(backoff):
		if err := msg.Nack(false, true); err != nil {
			logger.Error("nack failed", "error", err)
		}
	}
}

func (c *Consumer) Shutdown() {
	close(c.done)
	c.wg.Wait()
	if c.channel != nil {
		_ = c.channel.Close()
	}
	if c.conn != nil {
		_ = c.conn.Close()
	}
	c.logger.Info("consumer shut down")
}
