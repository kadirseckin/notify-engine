package model

import "github.com/google/uuid"

const (
	ErrCodeInvalidRequest = "INVALID_REQUEST"
	ErrCodeValidation     = "VALIDATION_ERROR"
	ErrCodeNotFound       = "NOT_FOUND"
	ErrCodeDuplicate      = "DUPLICATE"
	ErrCodeInvalidID      = "INVALID_ID"
	ErrCodeInvalidFilter  = "INVALID_FILTER"
	ErrCodeInternal       = "INTERNAL_ERROR"
	ErrCodeMetrics        = "METRICS_ERROR"
)

type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *APIError   `json:"error,omitempty"`
	Meta    *Meta       `json:"meta,omitempty"`
}

type APIError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Details []ValidationError `json:"details,omitempty"`
}

type Meta struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

type BatchCreateResponse struct {
	BatchID       uuid.UUID         `json:"batch_id"`
	TotalAccepted int               `json:"total_accepted"`
	TotalRejected int               `json:"total_rejected"`
	Notifications []BatchItemResult `json:"notifications"`
}

type BatchItemResult struct {
	Index  int               `json:"index"`
	ID     *uuid.UUID        `json:"id,omitempty"`
	Status string            `json:"status"`
	Errors []ValidationError `json:"errors,omitempty"`
}

type NotificationListResponse struct {
	Notifications []Notification `json:"notifications"`
}

type HealthResponse struct {
	Status   string            `json:"status"`
	Services map[string]string `json:"services"`
}

type MetricsResponse struct {
	QueueDepth   map[string]int64          `json:"queue_depth"`
	SuccessRate  map[string]float64        `json:"success_rate"`
	FailureRate  map[string]float64        `json:"failure_rate"`
	AvgLatencyMs map[string]float64        `json:"avg_latency_ms"`
	TotalSent    int64                     `json:"total_sent"`
	TotalFailed  int64                     `json:"total_failed"`
	TotalPending int64                     `json:"total_pending"`
	PerChannel   map[string]ChannelMetrics `json:"per_channel"`
}

type ChannelMetrics struct {
	Sent    int64 `json:"sent"`
	Failed  int64 `json:"failed"`
	Pending int64 `json:"pending"`
	Queued  int64 `json:"queued"`
}
