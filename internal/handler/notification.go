package handler

import (
	"errors"
	"math"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"notify-engine/internal/model"
	"notify-engine/internal/service"
)

type NotificationHandler struct {
	svc service.NotificationService
}

func NewNotificationHandler(svc service.NotificationService) *NotificationHandler {
	return &NotificationHandler{svc: svc}
}

func (h *NotificationHandler) RegisterRoutes(r *gin.RouterGroup) {
	n := r.Group("/notifications")
	n.POST("", h.Create)
	n.POST("/batch", h.CreateBatch)
	n.GET("/:id", h.GetByID)
	n.GET("/batch/:batch_id", h.GetByBatchID)
	n.DELETE("/:id", h.Cancel)
	n.GET("", h.List)
}

func (h *NotificationHandler) Create(c *gin.Context) {
	var req model.CreateNotificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, model.ErrCodeInvalidRequest, "invalid request body", nil)
		return
	}
	notification, err := h.svc.Create(c.Request.Context(), req)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	respondSuccess(c, http.StatusCreated, notification, nil)
}

func (h *NotificationHandler) CreateBatch(c *gin.Context) {
	var req model.BatchCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, model.ErrCodeInvalidRequest, "invalid request body", nil)
		return
	}
	resp, err := h.svc.CreateBatch(c.Request.Context(), req)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	respondSuccess(c, http.StatusCreated, resp, nil)
}

func (h *NotificationHandler) GetByID(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, model.ErrCodeInvalidID, "invalid UUID format", nil)
		return
	}
	notification, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	respondSuccess(c, http.StatusOK, notification, nil)
}

func (h *NotificationHandler) GetByBatchID(c *gin.Context) {
	batchID, err := uuid.Parse(c.Param("batch_id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, model.ErrCodeInvalidID, "invalid batch UUID format", nil)
		return
	}
	notifications, err := h.svc.GetByBatchID(c.Request.Context(), batchID)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	respondSuccess(c, http.StatusOK, model.NotificationListResponse{Notifications: notifications}, nil)
}

func (h *NotificationHandler) Cancel(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, model.ErrCodeInvalidID, "invalid UUID format", nil)
		return
	}
	if err := h.svc.Cancel(c.Request.Context(), id); err != nil {
		handleServiceError(c, err)
		return
	}
	respondSuccess(c, http.StatusOK, gin.H{"message": "notification cancelled"}, nil)
}

func (h *NotificationHandler) List(c *gin.Context) {
	var filter model.ListNotificationsRequest
	if err := c.ShouldBindQuery(&filter); err != nil {
		respondError(c, http.StatusBadRequest, model.ErrCodeInvalidFilter, "invalid filter parameters", nil)
		return
	}
	notifications, total, err := h.svc.List(c.Request.Context(), filter)
	if err != nil {
		respondError(c, http.StatusInternalServerError, model.ErrCodeInternal, "failed to list notifications", nil)
		return
	}
	filter.Normalize()
	totalPages := int(math.Ceil(float64(total) / float64(filter.PageSize)))
	respondSuccess(c, http.StatusOK, model.NotificationListResponse{Notifications: notifications},
		&model.Meta{Page: filter.Page, PageSize: filter.PageSize, Total: total, TotalPages: totalPages})
}

func respondSuccess(c *gin.Context, status int, data interface{}, meta *model.Meta) {
	c.JSON(status, model.APIResponse{Success: true, Data: data, Meta: meta})
}

func respondError(c *gin.Context, status int, code, message string, details []model.ValidationError) {
	c.JSON(status, model.APIResponse{Success: false, Error: &model.APIError{Code: code, Message: message, Details: details}})
}

func handleServiceError(c *gin.Context, err error) {
	var ve *service.ValidationErr
	var nf *service.NotFoundErr
	switch {
	case errors.As(err, &ve):
		respondError(c, http.StatusBadRequest, model.ErrCodeValidation, "validation failed", ve.Errors)
	case errors.As(err, &nf):
		respondError(c, http.StatusNotFound, model.ErrCodeNotFound, err.Error(), nil)
	default:
		respondError(c, http.StatusInternalServerError, model.ErrCodeInternal, "unexpected error", nil)
	}
}
