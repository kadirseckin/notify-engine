package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"notify-engine/internal/model"
)

type NotificationRepository interface {
	Create(ctx context.Context, n *model.Notification) error
	CreateBatch(ctx context.Context, notifications []*model.Notification) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.Notification, error)
	GetByBatchID(ctx context.Context, batchID uuid.UUID) ([]model.Notification, error)
	GetByIdempotencyKey(ctx context.Context, key string) (*model.Notification, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status model.Status, providerMsgID *string, lastError *string) error
	IncrementRetry(ctx context.Context, id uuid.UUID, lastError string) error
	Cancel(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context, filter model.ListNotificationsRequest) ([]model.Notification, int64, error)
	GetPending(ctx context.Context, limit int) ([]model.Notification, error)
	CountByStatus(ctx context.Context) (map[model.Status]int64, error)
	CountByChannelAndStatus(ctx context.Context) (map[model.Channel]map[model.Status]int64, error)
}

type notificationRepo struct {
	db *sqlx.DB
}

func NewNotificationRepository(db *sqlx.DB) NotificationRepository {
	return &notificationRepo{db: db}
}

func (r *notificationRepo) Create(ctx context.Context, n *model.Notification) error {
	query := `
		INSERT INTO notifications (id, batch_id, idempotency_key, recipient, channel, content, subject, priority, status, max_retries, scheduled_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW(), NOW())
		RETURNING created_at, updated_at`
	return r.db.QueryRowxContext(ctx, query,
		n.ID, n.BatchID, n.IdempotencyKey, n.Recipient, n.Channel,
		n.Content, n.Subject, n.Priority, n.Status, n.MaxRetries, n.ScheduledAt,
	).Scan(&n.CreatedAt, &n.UpdatedAt)
}

func (r *notificationRepo) CreateBatch(ctx context.Context, notifications []*model.Notification) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PreparexContext(ctx, `
		INSERT INTO notifications (id, batch_id, idempotency_key, recipient, channel, content, subject, priority, status, max_retries, scheduled_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW(), NOW())
		RETURNING created_at, updated_at`)
	if err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, n := range notifications {
		if err := stmt.QueryRowxContext(ctx, n.ID, n.BatchID, n.IdempotencyKey,
			n.Recipient, n.Channel, n.Content, n.Subject, n.Priority,
			n.Status, n.MaxRetries, n.ScheduledAt,
		).Scan(&n.CreatedAt, &n.UpdatedAt); err != nil {
			return fmt.Errorf("insert %s: %w", n.ID, err)
		}
	}
	return tx.Commit()
}

func (r *notificationRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.Notification, error) {
	var n model.Notification
	if err := r.db.GetContext(ctx, &n, "SELECT * FROM notifications WHERE id = $1", id); err != nil {
		return nil, err
	}
	return &n, nil
}

func (r *notificationRepo) GetByBatchID(ctx context.Context, batchID uuid.UUID) ([]model.Notification, error) {
	ns := []model.Notification{}
	err := r.db.SelectContext(ctx, &ns, "SELECT * FROM notifications WHERE batch_id = $1 ORDER BY created_at", batchID)
	return ns, err
}

func (r *notificationRepo) GetByIdempotencyKey(ctx context.Context, key string) (*model.Notification, error) {
	var n model.Notification
	if err := r.db.GetContext(ctx, &n, "SELECT * FROM notifications WHERE idempotency_key = $1", key); err != nil {
		return nil, err
	}
	return &n, nil
}

func (r *notificationRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status model.Status, providerMsgID *string, lastError *string) error {
	query := `UPDATE notifications SET status = $2, provider_message_id = COALESCE($3, provider_message_id), last_error = $4`
	if status == model.StatusSent {
		query += `, sent_at = NOW()`
	}
	query += ` WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, id, status, providerMsgID, lastError)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("notification %s not found", id)
	}
	return nil
}

func (r *notificationRepo) IncrementRetry(ctx context.Context, id uuid.UUID, lastError string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE notifications SET retry_count = retry_count + 1, last_error = $2 WHERE id = $1`, id, lastError)
	return err
}

func (r *notificationRepo) Cancel(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE notifications SET status = 'cancelled' WHERE id = $1 AND status IN ('pending', 'queued')`, id)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("notification %s not found or not cancellable", id)
	}
	return nil
}

func (r *notificationRepo) List(ctx context.Context, filter model.ListNotificationsRequest) ([]model.Notification, int64, error) {
	filter.Normalize()
	var conditions []string
	var args []interface{}
	idx := 1

	if filter.Status != nil {
		conditions = append(conditions, fmt.Sprintf("status = $%d", idx))
		args = append(args, *filter.Status)
		idx++
	}
	if filter.Channel != nil {
		conditions = append(conditions, fmt.Sprintf("channel = $%d", idx))
		args = append(args, *filter.Channel)
		idx++
	}
	if filter.BatchID != nil {
		conditions = append(conditions, fmt.Sprintf("batch_id = $%d", idx))
		args = append(args, *filter.BatchID)
		idx++
	}
	if filter.StartDate != nil {
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", idx))
		args = append(args, *filter.StartDate)
		idx++
	}
	if filter.EndDate != nil {
		conditions = append(conditions, fmt.Sprintf("created_at <= $%d", idx))
		args = append(args, *filter.EndDate)
		idx++
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	var total int64
	if err := r.db.GetContext(ctx, &total, fmt.Sprintf("SELECT COUNT(*) FROM notifications %s", where), args...); err != nil {
		return nil, 0, err
	}

	offset := (filter.Page - 1) * filter.PageSize
	args = append(args, filter.PageSize, offset)
	dataQ := fmt.Sprintf("SELECT * FROM notifications %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d", where, idx, idx+1)

	ns := []model.Notification{}

	if err := r.db.SelectContext(ctx, &ns, dataQ, args...); err != nil {
		return nil, 0, err
	}
	return ns, total, nil
}

func (r *notificationRepo) CountByStatus(ctx context.Context) (map[model.Status]int64, error) {
	rows, err := r.db.QueryxContext(ctx, "SELECT status, COUNT(*) as count FROM notifications GROUP BY status")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make(map[model.Status]int64)
	for rows.Next() {
		var s model.Status
		var c int64
		if err := rows.Scan(&s, &c); err != nil {
			return nil, err
		}
		result[s] = c
	}
	return result, nil
}

func (r *notificationRepo) CountByChannelAndStatus(ctx context.Context) (map[model.Channel]map[model.Status]int64, error) {
	rows, err := r.db.QueryxContext(ctx, "SELECT channel, status, COUNT(*) as count FROM notifications GROUP BY channel, status")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make(map[model.Channel]map[model.Status]int64)
	for rows.Next() {
		var ch model.Channel
		var s model.Status
		var c int64
		if err := rows.Scan(&ch, &s, &c); err != nil {
			return nil, err
		}
		if result[ch] == nil {
			result[ch] = make(map[model.Status]int64)
		}
		result[ch][s] = c
	}
	return result, nil
}

func (r *notificationRepo) GetPending(ctx context.Context, limit int) ([]model.Notification, error) {
	ns := []model.Notification{}
	err := r.db.SelectContext(ctx, &ns,
		`SELECT * FROM notifications
 WHERE status = 'pending'
   AND (scheduled_at IS NULL OR scheduled_at <= NOW())
 ORDER BY created_at ASC
 LIMIT $1`, limit)
	return ns, err
}
