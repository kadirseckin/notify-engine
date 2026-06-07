package model

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

type Template struct {
	ID        uuid.UUID `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	Channel   Channel   `json:"channel" db:"channel"`
	Subject   *string   `json:"subject,omitempty" db:"subject"`
	Body      string    `json:"body" db:"body"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// Render replaces {{key}} placeholders with variable values.
func (t *Template) Render(variables map[string]string) (content string, subject *string) {
	content = t.Body
	for key, val := range variables {
		content = strings.ReplaceAll(content, "{{"+key+"}}", val)
	}

	if t.Subject != nil {
		s := *t.Subject
		for key, val := range variables {
			s = strings.ReplaceAll(s, "{{"+key+"}}", val)
		}
		subject = &s
	}

	return content, subject
}

type CreateTemplateRequest struct {
	Name    string  `json:"name" binding:"required"`
	Channel Channel `json:"channel" binding:"required"`
	Subject *string `json:"subject,omitempty"`
	Body    string  `json:"body" binding:"required"`
}

func (r *CreateTemplateRequest) Validate() []ValidationError {
	var errs []ValidationError
	if r.Name == "" {
		errs = append(errs, ValidationError{Field: "name", Message: "name is required"})
	}
	switch r.Channel {
	case ChannelSMS, ChannelEmail, ChannelPush:
	default:
		errs = append(errs, ValidationError{Field: "channel", Message: "must be one of [sms, email, push]"})
	}
	if r.Body == "" {
		errs = append(errs, ValidationError{Field: "body", Message: "body is required"})
	}
	if r.Channel == ChannelEmail && (r.Subject == nil || *r.Subject == "") {
		errs = append(errs, ValidationError{Field: "subject", Message: "subject is required for email templates"})
	}
	return errs
}
