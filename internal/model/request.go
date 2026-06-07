package model

import (
	"fmt"
	"net/mail"
	"regexp"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	SMSMaxLength   = 160
	EmailMaxLength = 50000
	PushMaxLength  = 256
)

var phoneRegex = regexp.MustCompile(`^\+[1-9]\d{6,14}$`)

type CreateNotificationRequest struct {
	Recipient      string            `json:"recipient" binding:"required"`
	Channel        Channel           `json:"channel" binding:"required"`
	Content        string            `json:"content"`
	Subject        *string           `json:"subject,omitempty"`
	Priority       Priority          `json:"priority" binding:"required"`
	IdempotencyKey *string           `json:"idempotency_key,omitempty"`
	ScheduledAt    *JSONTime         `json:"scheduled_at,omitempty"`
	TemplateID     *uuid.UUID        `json:"template_id,omitempty"`
	Variables      map[string]string `json:"variables,omitempty"`
}

func (r *CreateNotificationRequest) Validate() []ValidationError {
	var errs []ValidationError

	switch r.Channel {
	case ChannelSMS, ChannelEmail, ChannelPush:
	default:
		errs = append(errs, ValidationError{Field: "channel", Message: "must be one of [sms, email, push]"})
	}

	switch r.Priority {
	case PriorityHigh, PriorityNormal, PriorityLow:
	default:
		errs = append(errs, ValidationError{Field: "priority", Message: "must be one of [high, normal, low]"})
	}

	switch r.Channel {
	case ChannelSMS:
		if !phoneRegex.MatchString(r.Recipient) {
			errs = append(errs, ValidationError{Field: "recipient", Message: "SMS recipient must be E.164 format (e.g. +905551234567)"})
		}
	case ChannelEmail:
		if _, err := mail.ParseAddress(r.Recipient); err != nil {
			errs = append(errs, ValidationError{Field: "recipient", Message: "invalid email address"})
		}
	case ChannelPush:
		if r.Recipient == "" {
			errs = append(errs, ValidationError{Field: "recipient", Message: "push device token is required"})
		}
	}

	// Content is required unless template_id is provided
	if r.Content == "" && r.TemplateID == nil {
		errs = append(errs, ValidationError{Field: "content", Message: "content is required (or provide template_id)"})
	}

	if r.Content != "" {
		contentLen := utf8.RuneCountInString(r.Content)
		switch r.Channel {
		case ChannelSMS:
			if contentLen > SMSMaxLength {
				errs = append(errs, ValidationError{Field: "content", Message: fmt.Sprintf("SMS content exceeds %d characters", SMSMaxLength)})
			}
		case ChannelEmail:
			if contentLen > EmailMaxLength {
				errs = append(errs, ValidationError{Field: "content", Message: fmt.Sprintf("email content exceeds %d characters", EmailMaxLength)})
			}
		case ChannelPush:
			if contentLen > PushMaxLength {
				errs = append(errs, ValidationError{Field: "content", Message: fmt.Sprintf("push content exceeds %d characters", PushMaxLength)})
			}
		}
	}

	// Subject required for email unless template provides it
	if r.Channel == ChannelEmail && r.TemplateID == nil && (r.Subject == nil || *r.Subject == "") {
		errs = append(errs, ValidationError{Field: "subject", Message: "subject is required for email notifications"})
	}

	if r.ScheduledAt != nil && time.Time(*r.ScheduledAt).Before(time.Now()) {
		errs = append(errs, ValidationError{Field: "scheduled_at", Message: "scheduled_at must be in the future"})
	}

	return errs
}

type BatchCreateRequest struct {
	Notifications []CreateNotificationRequest `json:"notifications" binding:"required,min=1,max=1000"`
}

func (r *BatchCreateRequest) Validate() []ValidationError {
	var errs []ValidationError
	if len(r.Notifications) == 0 {
		errs = append(errs, ValidationError{Field: "notifications", Message: "at least 1 notification required"})
		return errs
	}
	if len(r.Notifications) > 1000 {
		errs = append(errs, ValidationError{Field: "notifications", Message: "maximum 1000 notifications per batch"})
		return errs
	}
	for i, n := range r.Notifications {
		for _, e := range n.Validate() {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("notifications[%d].%s", i, e.Field),
				Message: e.Message,
			})
		}
	}
	return errs
}

type ListNotificationsRequest struct {
	Status    *Status    `form:"status"`
	Channel   *Channel   `form:"channel"`
	BatchID   *uuid.UUID `form:"batch_id"`
	StartDate *time.Time `form:"start_date" time_format:"2006-01-02T15:04:05Z07:00"`
	EndDate   *time.Time `form:"end_date" time_format:"2006-01-02T15:04:05Z07:00"`
	Page      int        `form:"page,default=1"`
	PageSize  int        `form:"page_size,default=20"`
}

func (r *ListNotificationsRequest) Normalize() {
	if r.Page < 1 {
		r.Page = 1
	}
	if r.PageSize < 1 {
		r.PageSize = 20
	}
	if r.PageSize > 100 {
		r.PageSize = 100
	}
}

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type JSONTime time.Time

func (t *JSONTime) UnmarshalJSON(b []byte) error {
	s := string(b)
	s = s[1 : len(s)-1]
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return fmt.Errorf("invalid time format, use RFC3339 (e.g. 2026-06-07T10:00:00Z)")
	}
	*t = JSONTime(parsed)
	return nil
}

func (t JSONTime) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, time.Time(t).Format(time.RFC3339))), nil
}

func (t JSONTime) Time() time.Time {
	return time.Time(t)
}
