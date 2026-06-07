package service_test

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"notify-engine/internal/model"
	"notify-engine/internal/service"
)

// ==================== Mock Repository ====================

type mockRepo struct {
	createFn           func(ctx context.Context, n *model.Notification) error
	createBatchFn      func(ctx context.Context, ns []*model.Notification) error
	getByIDFn          func(ctx context.Context, id uuid.UUID) (*model.Notification, error)
	getByBatchIDFn     func(ctx context.Context, id uuid.UUID) ([]model.Notification, error)
	getByIdempotencyFn func(ctx context.Context, key string) (*model.Notification, error)
	updateStatusFn     func(ctx context.Context, id uuid.UUID, status model.Status, pid *string, err *string) error
	cancelFn           func(ctx context.Context, id uuid.UUID) error
	listFn             func(ctx context.Context, f model.ListNotificationsRequest) ([]model.Notification, int64, error)
}

func (m *mockRepo) Create(ctx context.Context, n *model.Notification) error {
	if m.createFn != nil {
		return m.createFn(ctx, n)
	}
	n.CreatedAt = time.Now()
	n.UpdatedAt = time.Now()
	return nil
}
func (m *mockRepo) CreateBatch(ctx context.Context, ns []*model.Notification) error {
	if m.createBatchFn != nil {
		return m.createBatchFn(ctx, ns)
	}
	for _, n := range ns {
		n.CreatedAt = time.Now()
		n.UpdatedAt = time.Now()
	}
	return nil
}
func (m *mockRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.Notification, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, sql.ErrNoRows
}
func (m *mockRepo) GetByBatchID(ctx context.Context, id uuid.UUID) ([]model.Notification, error) {
	if m.getByBatchIDFn != nil {
		return m.getByBatchIDFn(ctx, id)
	}
	return nil, nil
}
func (m *mockRepo) GetByIdempotencyKey(ctx context.Context, key string) (*model.Notification, error) {
	if m.getByIdempotencyFn != nil {
		return m.getByIdempotencyFn(ctx, key)
	}
	return nil, sql.ErrNoRows
}
func (m *mockRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status model.Status, pid *string, e *string) error {
	if m.updateStatusFn != nil {
		return m.updateStatusFn(ctx, id, status, pid, e)
	}
	return nil
}
func (m *mockRepo) IncrementRetry(ctx context.Context, id uuid.UUID, lastError string) error {
	return nil
}
func (m *mockRepo) Cancel(ctx context.Context, id uuid.UUID) error {
	if m.cancelFn != nil {
		return m.cancelFn(ctx, id)
	}
	return nil
}
func (m *mockRepo) List(ctx context.Context, f model.ListNotificationsRequest) ([]model.Notification, int64, error) {
	if m.listFn != nil {
		return m.listFn(ctx, f)
	}
	return nil, 0, nil
}
func (m *mockRepo) CountByStatus(ctx context.Context) (map[model.Status]int64, error) {
	return nil, nil
}
func (m *mockRepo) CountByChannelAndStatus(ctx context.Context) (map[model.Channel]map[model.Status]int64, error) {
	return nil, nil
}
func (m *mockRepo) GetPending(ctx context.Context, limit int) ([]model.Notification, error) {
	return nil, nil
}

// ==================== Mock Publisher ====================

type mockPublisher struct {
	published []*model.Notification
}

func (m *mockPublisher) Publish(ctx context.Context, n *model.Notification) error {
	m.published = append(m.published, n)
	return nil
}

func (m *mockPublisher) Close() error { return nil }

// ==================== Tests ====================

func newService(repo *mockRepo, pub *mockPublisher) service.NotificationService {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	return service.NewNotificationService(repo, nil, pub, logger)
}

func TestCreate_Success(t *testing.T) {
	repo := &mockRepo{}
	pub := &mockPublisher{}
	svc := newService(repo, pub)

	n, err := svc.Create(context.Background(), model.CreateNotificationRequest{
		Recipient: "+905551234567",
		Channel:   model.ChannelSMS,
		Content:   "Hello",
		Priority:  model.PriorityHigh,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n.Status != model.StatusQueued {
		t.Errorf("status: got %s, want queued", n.Status)
	}
	if len(pub.published) != 1 {
		t.Errorf("published: got %d, want 1", len(pub.published))
	}
}

func TestCreate_ValidationError(t *testing.T) {
	svc := newService(&mockRepo{}, &mockPublisher{})

	_, err := svc.Create(context.Background(), model.CreateNotificationRequest{
		Recipient: "invalid",
		Channel:   model.ChannelSMS,
		Content:   "Hello",
		Priority:  model.PriorityNormal,
	})

	if err == nil {
		t.Fatal("expected validation error")
	}
	if _, ok := err.(*service.ValidationErr); !ok {
		t.Errorf("expected ValidationErr, got %T", err)
	}
}

func TestCreate_Idempotency(t *testing.T) {
	existingID := uuid.New()
	repo := &mockRepo{
		getByIdempotencyFn: func(_ context.Context, key string) (*model.Notification, error) {
			return &model.Notification{
				ID:       existingID,
				Status:   model.StatusSent,
				Channel:  model.ChannelSMS,
				Priority: model.PriorityHigh,
			}, nil
		},
	}
	pub := &mockPublisher{}
	svc := newService(repo, pub)

	key := "test-key-123"
	n, err := svc.Create(context.Background(), model.CreateNotificationRequest{
		Recipient:      "+905551234567",
		Channel:        model.ChannelSMS,
		Content:        "Hello",
		Priority:       model.PriorityHigh,
		IdempotencyKey: &key,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n.ID != existingID {
		t.Errorf("should return existing notification, got different ID")
	}
	if len(pub.published) != 0 {
		t.Error("should NOT publish again for idempotent request")
	}
}

func TestCreate_ScheduledFuture_NotPublished(t *testing.T) {
	repo := &mockRepo{}
	pub := &mockPublisher{}
	svc := newService(repo, pub)

	future := model.JSONTime(time.Now().Add(1 * time.Hour))
	n, err := svc.Create(context.Background(), model.CreateNotificationRequest{
		Recipient:   "+905551234567",
		Channel:     model.ChannelSMS,
		Content:     "Scheduled",
		Priority:    model.PriorityNormal,
		ScheduledAt: &future,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n.Status != model.StatusPending {
		t.Errorf("scheduled notification should stay pending, got %s", n.Status)
	}
	if len(pub.published) != 0 {
		t.Error("scheduled notification should NOT be published immediately")
	}
}

func TestCreateBatch_PartialReject(t *testing.T) {
	repo := &mockRepo{}
	pub := &mockPublisher{}
	svc := newService(repo, pub)

	resp, err := svc.CreateBatch(context.Background(), model.BatchCreateRequest{
		Notifications: []model.CreateNotificationRequest{
			{Recipient: "+905551234567", Channel: model.ChannelSMS, Content: "Valid", Priority: model.PriorityHigh},
			{Recipient: "bad-phone", Channel: model.ChannelSMS, Content: "Invalid", Priority: model.PriorityNormal},
			{Recipient: "+905559876543", Channel: model.ChannelSMS, Content: "Also valid", Priority: model.PriorityLow},
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.TotalAccepted != 2 {
		t.Errorf("accepted: got %d, want 2", resp.TotalAccepted)
	}
	if resp.TotalRejected != 1 {
		t.Errorf("rejected: got %d, want 1", resp.TotalRejected)
	}
	if len(pub.published) != 2 {
		t.Errorf("published: got %d, want 2", len(pub.published))
	}
}

func TestGetByID_NotFound(t *testing.T) {
	repo := &mockRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*model.Notification, error) {
			return nil, sql.ErrNoRows
		},
	}
	svc := newService(repo, &mockPublisher{})

	_, err := svc.GetByID(context.Background(), uuid.New())

	if err == nil {
		t.Fatal("expected not found error")
	}
	if _, ok := err.(*service.NotFoundErr); !ok {
		t.Errorf("expected NotFoundErr, got %T", err)
	}
}

func TestCancel_Success(t *testing.T) {
	repo := &mockRepo{
		cancelFn: func(_ context.Context, _ uuid.UUID) error { return nil },
	}
	svc := newService(repo, &mockPublisher{})

	err := svc.Cancel(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCancel_NotCancellable(t *testing.T) {
	repo := &mockRepo{
		cancelFn: func(_ context.Context, id uuid.UUID) error {
			return fmt.Errorf("notification %s not found or not cancellable", id)
		},
	}
	svc := newService(repo, &mockPublisher{})

	err := svc.Cancel(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error for non-cancellable notification")
	}
}

func TestCreateBatch_Idempotency(t *testing.T) {
	key := "batch-idemp-001"
	existingID := uuid.New()
	repo := &mockRepo{
		getByIdempotencyFn: func(_ context.Context, k string) (*model.Notification, error) {
			if k == key {
				return &model.Notification{ID: existingID, Status: model.StatusSent}, nil
			}
			return nil, sql.ErrNoRows
		},
	}
	pub := &mockPublisher{}
	svc := newService(repo, pub)

	resp, err := svc.CreateBatch(context.Background(), model.BatchCreateRequest{
		Notifications: []model.CreateNotificationRequest{
			{Recipient: "+905551234567", Channel: model.ChannelSMS, Content: "New", Priority: model.PriorityHigh},
			{Recipient: "+905559876543", Channel: model.ChannelSMS, Content: "Duplicate", Priority: model.PriorityNormal, IdempotencyKey: &key},
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.TotalAccepted != 2 {
		t.Errorf("accepted: got %d, want 2", resp.TotalAccepted)
	}
	if len(pub.published) != 1 {
		t.Errorf("published: got %d, want 1 (duplicate should not publish)", len(pub.published))
	}

	// İkinci item mevcut ID'yi dönmeli
	if resp.Notifications[1].ID == nil || *resp.Notifications[1].ID != existingID {
		t.Error("duplicate item should return existing ID")
	}
}
