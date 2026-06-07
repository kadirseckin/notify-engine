package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"notify-engine/internal/handler"
	"notify-engine/internal/model"
)

type mockSvc struct {
	createFn    func(context.Context, model.CreateNotificationRequest) (*model.Notification, error)
	createBatFn func(context.Context, model.BatchCreateRequest) (*model.BatchCreateResponse, error)
	getByIDFn   func(context.Context, uuid.UUID) (*model.Notification, error)
	getByBatFn  func(context.Context, uuid.UUID) ([]model.Notification, error)
	cancelFn    func(context.Context, uuid.UUID) error
	listFn      func(context.Context, model.ListNotificationsRequest) ([]model.Notification, int64, error)
}

func (m *mockSvc) Create(ctx context.Context, r model.CreateNotificationRequest) (*model.Notification, error) {
	return m.createFn(ctx, r)
}
func (m *mockSvc) CreateBatch(ctx context.Context, r model.BatchCreateRequest) (*model.BatchCreateResponse, error) {
	return m.createBatFn(ctx, r)
}
func (m *mockSvc) GetByID(ctx context.Context, id uuid.UUID) (*model.Notification, error) {
	return m.getByIDFn(ctx, id)
}
func (m *mockSvc) GetByBatchID(ctx context.Context, id uuid.UUID) ([]model.Notification, error) {
	return m.getByBatFn(ctx, id)
}
func (m *mockSvc) Cancel(ctx context.Context, id uuid.UUID) error { return m.cancelFn(ctx, id) }
func (m *mockSvc) List(ctx context.Context, f model.ListNotificationsRequest) ([]model.Notification, int64, error) {
	return m.listFn(ctx, f)
}

func setup(svc *mockSvc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handler.NewNotificationHandler(svc)
	h.RegisterRoutes(r.Group("/api/v1"))
	return r
}

func TestCreate_201(t *testing.T) {
	s := &mockSvc{createFn: func(_ context.Context, r model.CreateNotificationRequest) (*model.Notification, error) {
		return &model.Notification{ID: uuid.New(), Recipient: r.Recipient, Channel: r.Channel, Content: r.Content, Priority: r.Priority, Status: model.StatusQueued, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
	}}
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/notifications", bytes.NewBufferString(`{"recipient":"+905551234567","channel":"sms","content":"Hi","priority":"high"}`))
	req.Header.Set("Content-Type", "application/json")
	setup(s).ServeHTTP(w, req)
	if w.Code != 201 {
		t.Errorf("got %d want 201", w.Code)
	}
}

func TestCreate_400(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/notifications", bytes.NewBufferString(`{bad`))
	req.Header.Set("Content-Type", "application/json")
	setup(&mockSvc{}).ServeHTTP(w, req)
	if w.Code != 400 {
		t.Errorf("got %d want 400", w.Code)
	}
}

func TestGetByID_400(t *testing.T) {
	w := httptest.NewRecorder()
	setup(&mockSvc{}).ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/notifications/not-uuid", nil))
	if w.Code != 400 {
		t.Errorf("got %d want 400", w.Code)
	}
}

func TestGetByID_200(t *testing.T) {
	id := uuid.New()
	s := &mockSvc{getByIDFn: func(_ context.Context, _ uuid.UUID) (*model.Notification, error) {
		return &model.Notification{ID: id, Status: model.StatusSent, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
	}}
	w := httptest.NewRecorder()
	setup(s).ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/notifications/"+id.String(), nil))
	if w.Code != 200 {
		t.Errorf("got %d want 200", w.Code)
	}
}

func TestCancel_200(t *testing.T) {
	s := &mockSvc{cancelFn: func(_ context.Context, _ uuid.UUID) error { return nil }}
	w := httptest.NewRecorder()
	setup(s).ServeHTTP(w, httptest.NewRequest("DELETE", "/api/v1/notifications/"+uuid.New().String(), nil))
	if w.Code != 200 {
		t.Errorf("got %d want 200", w.Code)
	}
}

func TestList_200(t *testing.T) {
	s := &mockSvc{listFn: func(_ context.Context, _ model.ListNotificationsRequest) ([]model.Notification, int64, error) {
		return []model.Notification{{ID: uuid.New(), CreatedAt: time.Now(), UpdatedAt: time.Now()}}, 1, nil
	}}
	w := httptest.NewRecorder()
	setup(s).ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/notifications?page=1&page_size=10", nil))
	if w.Code != 200 {
		t.Errorf("got %d want 200", w.Code)
	}
	var resp model.APIResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Meta == nil {
		t.Error("expected meta")
	}
}
