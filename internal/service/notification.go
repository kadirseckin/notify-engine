package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"notify-engine/internal/model"
	"notify-engine/internal/queue"
	"notify-engine/internal/repository"
	"notify-engine/internal/telemetry"
)

type NotificationService interface {
	Create(ctx context.Context, req model.CreateNotificationRequest) (*model.Notification, error)
	CreateBatch(ctx context.Context, req model.BatchCreateRequest) (*model.BatchCreateResponse, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.Notification, error)
	GetByBatchID(ctx context.Context, batchID uuid.UUID) ([]model.Notification, error)
	Cancel(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context, filter model.ListNotificationsRequest) ([]model.Notification, int64, error)
}

type notificationService struct {
	repo      repository.NotificationRepository
	tmplRepo  repository.TemplateRepository
	publisher queue.Publisher
	logger    *slog.Logger
}

func NewNotificationService(repo repository.NotificationRepository, tmplRepo repository.TemplateRepository, publisher queue.Publisher, logger *slog.Logger) NotificationService {
	return &notificationService{repo: repo, tmplRepo: tmplRepo, publisher: publisher, logger: logger}
}

func (s *notificationService) Create(ctx context.Context, req model.CreateNotificationRequest) (*model.Notification, error) {
	ctx, span := otel.Tracer(telemetry.Name).Start(ctx, "service.CreateNotification")
	defer span.End()
	span.SetAttributes(
		attribute.String("notification.channel", string(req.Channel)),
		attribute.String("notification.recipient", req.Recipient),
		attribute.String("notification.priority", string(req.Priority)),
	)

	if errs := req.Validate(); len(errs) > 0 {
		span.SetStatus(codes.Error, "validation failed")
		return nil, &ValidationErr{Errors: errs}
	}

	if req.IdempotencyKey != nil {
		existing, err := s.repo.GetByIdempotencyKey(ctx, *req.IdempotencyKey)
		if err == nil && existing != nil {
			s.logger.Info("idempotent hit", "key", *req.IdempotencyKey, "id", existing.ID)
			return existing, nil
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("idempotency check: %w", err)
		}
	}

	// Resolve template if provided
	if req.TemplateID != nil {
		tmpl, err := s.tmplRepo.GetByID(ctx, *req.TemplateID)
		if err != nil {
			return nil, &ValidationErr{Errors: []model.ValidationError{{Field: "template_id", Message: "template not found"}}}
		}
		if tmpl.Channel != req.Channel {
			return nil, &ValidationErr{Errors: []model.ValidationError{{Field: "template_id", Message: fmt.Sprintf("template channel (%s) does not match request channel (%s)", tmpl.Channel, req.Channel)}}}
		}
		content, subject := tmpl.Render(req.Variables)
		req.Content = content
		if subject != nil {
			req.Subject = subject
		}
	}

	n := &model.Notification{
		ID:             uuid.New(),
		Recipient:      req.Recipient,
		Channel:        req.Channel,
		Content:        req.Content,
		Subject:        req.Subject,
		Priority:       req.Priority,
		Status:         model.StatusPending,
		IdempotencyKey: req.IdempotencyKey,
		MaxRetries:     3,
	}

	if req.ScheduledAt != nil {
		t := req.ScheduledAt.Time()
		n.ScheduledAt = &t
	}

	if err := s.repo.Create(ctx, n); err != nil {
		return nil, fmt.Errorf("create: %w", err)
	}

	if n.ScheduledAt == nil || n.ScheduledAt.Before(time.Now()) {
		if err := s.publisher.Publish(ctx, n); err != nil {
			s.logger.Error("publish failed", "id", n.ID, "error", err)
		} else {
			_ = s.repo.UpdateStatus(ctx, n.ID, model.StatusQueued, nil, nil)
			n.Status = model.StatusQueued
		}
	}

	span.SetAttributes(attribute.String("notification.id", n.ID.String()))
	s.logger.Info("created", "id", n.ID, "channel", n.Channel, "priority", n.Priority)
	return n, nil
}

func (s *notificationService) CreateBatch(ctx context.Context, req model.BatchCreateRequest) (*model.BatchCreateResponse, error) {
	if len(req.Notifications) == 0 {
		return nil, &ValidationErr{Errors: []model.ValidationError{{Field: "notifications", Message: "at least 1 notification required"}}}
	}
	if len(req.Notifications) > 1000 {
		return nil, &ValidationErr{Errors: []model.ValidationError{{Field: "notifications", Message: "maximum 1000 notifications per batch"}}}
	}

	batchID := uuid.New()
	resp := &model.BatchCreateResponse{BatchID: batchID, Notifications: make([]model.BatchItemResult, 0, len(req.Notifications))}
	var accepted []*model.Notification

	for i, item := range req.Notifications {
		if itemErrs := item.Validate(); len(itemErrs) > 0 {
			resp.TotalRejected++
			resp.Notifications = append(resp.Notifications, model.BatchItemResult{Index: i, Status: "rejected", Errors: itemErrs})
			continue
		}

		if item.IdempotencyKey != nil {
			existing, err := s.repo.GetByIdempotencyKey(ctx, *item.IdempotencyKey)
			if err == nil && existing != nil {
				id := existing.ID
				resp.TotalAccepted++
				resp.Notifications = append(resp.Notifications, model.BatchItemResult{Index: i, ID: &id, Status: "accepted"})
				continue
			}
		}

		n := &model.Notification{
			ID: uuid.New(), BatchID: &batchID, Recipient: item.Recipient, Channel: item.Channel,
			Content: item.Content, Subject: item.Subject, Priority: item.Priority,
			Status: model.StatusPending, IdempotencyKey: item.IdempotencyKey, MaxRetries: 3,
		}
		if item.ScheduledAt != nil {
			t := item.ScheduledAt.Time()
			n.ScheduledAt = &t
		}
		accepted = append(accepted, n)
		id := n.ID
		resp.TotalAccepted++
		resp.Notifications = append(resp.Notifications, model.BatchItemResult{Index: i, ID: &id, Status: "accepted"})
	}

	if len(accepted) > 0 {
		if err := s.repo.CreateBatch(ctx, accepted); err != nil {
			return nil, fmt.Errorf("batch insert: %w", err)
		}
		for _, n := range accepted {
			if n.ScheduledAt == nil || n.ScheduledAt.Before(time.Now()) {
				if err := s.publisher.Publish(ctx, n); err != nil {
					s.logger.Error("publish batch item failed", "id", n.ID, "error", err)
				} else {
					_ = s.repo.UpdateStatus(ctx, n.ID, model.StatusQueued, nil, nil)
				}
			}
		}
	}

	s.logger.Info("batch created", "batch_id", batchID, "accepted", resp.TotalAccepted, "rejected", resp.TotalRejected)
	return resp, nil
}

func (s *notificationService) GetByID(ctx context.Context, id uuid.UUID) (*model.Notification, error) {
	n, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &NotFoundErr{Entity: "notification", ID: id.String()}
		}
		return nil, err
	}
	return n, nil
}

func (s *notificationService) GetByBatchID(ctx context.Context, batchID uuid.UUID) ([]model.Notification, error) {
	return s.repo.GetByBatchID(ctx, batchID)
}

func (s *notificationService) Cancel(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.Cancel(ctx, id); err != nil {
		return &NotFoundErr{Entity: "notification", ID: id.String()}
	}
	s.logger.Info("cancelled", "id", id)
	return nil
}

func (s *notificationService) List(ctx context.Context, filter model.ListNotificationsRequest) ([]model.Notification, int64, error) {
	return s.repo.List(ctx, filter)
}

type ValidationErr struct{ Errors []model.ValidationError }

func (e *ValidationErr) Error() string {
	return fmt.Sprintf("validation failed: %d errors", len(e.Errors))
}

type NotFoundErr struct{ Entity, ID string }

func (e *NotFoundErr) Error() string { return fmt.Sprintf("%s %s not found", e.Entity, e.ID) }
