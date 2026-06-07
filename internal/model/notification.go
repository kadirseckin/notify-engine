package model

import (
	"time"

	"github.com/google/uuid"
)

type Channel string

const (
	ChannelSMS   Channel = "sms"
	ChannelEmail Channel = "email"
	ChannelPush  Channel = "push"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusQueued    Status = "queued"
	StatusSending   Status = "sending"
	StatusSent      Status = "sent"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

type Priority string

const (
	PriorityHigh   Priority = "high"
	PriorityNormal Priority = "normal"
	PriorityLow    Priority = "low"
)

func (p Priority) Int() int {
	switch p {
	case PriorityHigh:
		return 3
	case PriorityNormal:
		return 2
	case PriorityLow:
		return 1
	default:
		return 2
	}
}

type Notification struct {
	ID             uuid.UUID  `json:"id" db:"id"`
	BatchID        *uuid.UUID `json:"batch_id,omitempty" db:"batch_id"`
	IdempotencyKey *string    `json:"idempotency_key,omitempty" db:"idempotency_key"`
	Recipient      string     `json:"recipient" db:"recipient"`
	Channel        Channel    `json:"channel" db:"channel"`
	Content        string     `json:"content" db:"content"`
	Subject        *string    `json:"subject,omitempty" db:"subject"`
	Priority       Priority   `json:"priority" db:"priority"`
	Status         Status     `json:"status" db:"status"`
	ProviderMsgID  *string    `json:"provider_message_id,omitempty" db:"provider_message_id"`
	RetryCount     int        `json:"retry_count" db:"retry_count"`
	MaxRetries     int        `json:"max_retries" db:"max_retries"`
	LastError      *string    `json:"last_error,omitempty" db:"last_error"`
	ScheduledAt    *time.Time `json:"scheduled_at,omitempty" db:"scheduled_at"`
	SentAt         *time.Time `json:"sent_at,omitempty" db:"sent_at"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
}
