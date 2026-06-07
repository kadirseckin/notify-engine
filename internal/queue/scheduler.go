package queue

import (
	"context"
	"log/slog"
	"time"

	"notify-engine/internal/model"
	"notify-engine/internal/repository"
)

// Scheduler polls for due scheduled notifications and publishes them to the queue.
type Scheduler struct {
	repo      repository.NotificationRepository
	publisher Publisher
	logger    *slog.Logger
	interval  time.Duration
	batchSize int
	done      chan struct{}
}

func NewScheduler(repo repository.NotificationRepository, publisher Publisher, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		repo:      repo,
		publisher: publisher,
		logger:    logger,
		interval:  10 * time.Second,
		batchSize: 100,
		done:      make(chan struct{}),
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	s.logger.Info("scheduler started", "interval", s.interval)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("scheduler stopping (context cancelled)")
			return
		case <-s.done:
			s.logger.Info("scheduler stopped")
			return
		case <-ticker.C:
			s.pollAndPublish(ctx)
		}
	}
}

func (s *Scheduler) pollAndPublish(ctx context.Context) {
	notifications, err := s.repo.GetPending(ctx, s.batchSize)
	if err != nil {
		s.logger.Error("scheduler poll failed", "error", err)
		return
	}

	if len(notifications) == 0 {
		return
	}

	s.logger.Info("scheduler found due notifications", "count", len(notifications))

	for _, n := range notifications {
		n := n // capture
		if err := s.publisher.Publish(ctx, &n); err != nil {
			s.logger.Error("scheduler publish failed", "id", n.ID, "error", err)
			continue
		}
		_ = s.repo.UpdateStatus(ctx, n.ID, model.StatusQueued, nil, nil)
		s.logger.Debug("scheduler queued", "id", n.ID, "channel", n.Channel, "scheduled_at", n.ScheduledAt)
	}
}

func (s *Scheduler) Stop() {
	close(s.done)
}
