package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"notify-engine/internal/model"
)

type TemplateRepository interface {
	Create(ctx context.Context, t *model.Template) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.Template, error)
	GetByName(ctx context.Context, name string) (*model.Template, error)
	List(ctx context.Context) ([]model.Template, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type templateRepo struct {
	db *sqlx.DB
}

func NewTemplateRepository(db *sqlx.DB) TemplateRepository {
	return &templateRepo{db: db}
}

func (r *templateRepo) Create(ctx context.Context, t *model.Template) error {
	query := `INSERT INTO templates (id, name, channel, subject, body, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW()) RETURNING created_at, updated_at`
	err := r.db.QueryRowxContext(ctx, query, t.ID, t.Name, t.Channel, t.Subject, t.Body).Scan(&t.CreatedAt, &t.UpdatedAt)
	if IsDuplicate(err) {
		return ErrDuplicate
	}
	return err
}

func (r *templateRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.Template, error) {
	var t model.Template
	if err := r.db.GetContext(ctx, &t, "SELECT * FROM templates WHERE id = $1", id); err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *templateRepo) GetByName(ctx context.Context, name string) (*model.Template, error) {
	var t model.Template
	if err := r.db.GetContext(ctx, &t, "SELECT * FROM templates WHERE name = $1", name); err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *templateRepo) List(ctx context.Context) ([]model.Template, error) {
	var templates []model.Template
	err := r.db.SelectContext(ctx, &templates, "SELECT * FROM templates ORDER BY name")
	return templates, err
}

func (r *templateRepo) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM templates WHERE id = $1", id)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("template %s not found", id)
	}
	return nil
}
