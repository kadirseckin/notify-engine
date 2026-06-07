package handler

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"notify-engine/internal/model"
	repo "notify-engine/internal/repository"
)

type TemplateHandler struct {
	repo repo.TemplateRepository
}

func NewTemplateHandler(r repo.TemplateRepository) *TemplateHandler {
	return &TemplateHandler{repo: r}
}

func (h *TemplateHandler) RegisterRoutes(r *gin.RouterGroup) {
	t := r.Group("/templates")
	t.POST("", h.Create)
	t.GET("", h.List)
	t.GET("/:id", h.GetByID)
	t.DELETE("/:id", h.Delete)
}

func (h *TemplateHandler) Create(c *gin.Context) {
	var req model.CreateTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, model.ErrCodeInvalidRequest, "invalid request body", nil)
		return
	}

	if errs := req.Validate(); len(errs) > 0 {
		respondError(c, http.StatusBadRequest, model.ErrCodeValidation, "validation failed", errs)
		return
	}

	t := &model.Template{
		ID:      uuid.New(),
		Name:    req.Name,
		Channel: req.Channel,
		Subject: req.Subject,
		Body:    req.Body,
	}

	if err := h.repo.Create(c.Request.Context(), t); err != nil {
		if repo.IsDuplicate(err) {
			respondError(c, http.StatusConflict, model.ErrCodeDuplicate, "template with this name already exists", nil)
		} else {
			respondError(c, http.StatusInternalServerError, model.ErrCodeInternal, "failed to create template", nil)
		}
		return
	}

	respondSuccess(c, http.StatusCreated, t, nil)
}

func (h *TemplateHandler) List(c *gin.Context) {
	templates, err := h.repo.List(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusInternalServerError, model.ErrCodeInternal, "failed to list templates", nil)
		return
	}
	respondSuccess(c, http.StatusOK, templates, nil)
}

func (h *TemplateHandler) GetByID(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, model.ErrCodeInvalidID, "invalid UUID format", nil)
		return
	}

	t, err := h.repo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondError(c, http.StatusNotFound, model.ErrCodeNotFound, "template not found", nil)
		} else {
			respondError(c, http.StatusInternalServerError, model.ErrCodeInternal, "failed to get template", nil)
		}
		return
	}

	respondSuccess(c, http.StatusOK, t, nil)
}

func (h *TemplateHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, model.ErrCodeInvalidID, "invalid UUID format", nil)
		return
	}

	if err := h.repo.Delete(c.Request.Context(), id); err != nil {
		respondError(c, http.StatusNotFound, model.ErrCodeNotFound, "template not found", nil)
		return
	}

	respondSuccess(c, http.StatusOK, gin.H{"message": "template deleted"}, nil)
}
