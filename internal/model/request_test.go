package model_test

import (
	"testing"
	"notify-engine/internal/model"
)

func TestValidate_SMS(t *testing.T) {
	tests := []struct {
		name string
		req  model.CreateNotificationRequest
		want int
	}{
		{"valid", model.CreateNotificationRequest{Recipient: "+905551234567", Channel: model.ChannelSMS, Content: "Hello", Priority: model.PriorityNormal}, 0},
		{"bad phone", model.CreateNotificationRequest{Recipient: "05551234567", Channel: model.ChannelSMS, Content: "Hello", Priority: model.PriorityNormal}, 1},
		{"too long", model.CreateNotificationRequest{Recipient: "+905551234567", Channel: model.ChannelSMS, Content: string(make([]byte, 161)), Priority: model.PriorityNormal}, 1},
		{"bad channel", model.CreateNotificationRequest{Recipient: "+905551234567", Channel: "telegram", Content: "Hi", Priority: model.PriorityNormal}, 1},
		{"bad priority", model.CreateNotificationRequest{Recipient: "+905551234567", Channel: model.ChannelSMS, Content: "Hi", Priority: "urgent"}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := len(tt.req.Validate()); got != tt.want {
				t.Errorf("got %d errors, want %d", got, tt.want)
			}
		})
	}
}

func TestValidate_Email(t *testing.T) {
	subj := "Test"
	tests := []struct {
		name string
		req  model.CreateNotificationRequest
		want int
	}{
		{"valid", model.CreateNotificationRequest{Recipient: "a@b.com", Channel: model.ChannelEmail, Content: "Hi", Subject: &subj, Priority: model.PriorityHigh}, 0},
		{"bad email", model.CreateNotificationRequest{Recipient: "notmail", Channel: model.ChannelEmail, Content: "Hi", Subject: &subj, Priority: model.PriorityNormal}, 1},
		{"no subject", model.CreateNotificationRequest{Recipient: "a@b.com", Channel: model.ChannelEmail, Content: "Hi", Priority: model.PriorityNormal}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := len(tt.req.Validate()); got != tt.want {
				t.Errorf("got %d errors, want %d", got, tt.want)
			}
		})
	}
}

func TestValidate_Push(t *testing.T) {
	tests := []struct {
		name string
		req  model.CreateNotificationRequest
		want int
	}{
		{"valid", model.CreateNotificationRequest{Recipient: "token123", Channel: model.ChannelPush, Content: "New msg!", Priority: model.PriorityLow}, 0},
		{"too long", model.CreateNotificationRequest{Recipient: "token123", Channel: model.ChannelPush, Content: string(make([]byte, 257)), Priority: model.PriorityNormal}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := len(tt.req.Validate()); got != tt.want {
				t.Errorf("got %d errors, want %d", got, tt.want)
			}
		})
	}
}

func TestBatchValidate(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		r := model.BatchCreateRequest{}
		if len(r.Validate()) == 0 {
			t.Error("expected error")
		}
	})
	t.Run("valid", func(t *testing.T) {
		r := model.BatchCreateRequest{Notifications: []model.CreateNotificationRequest{
			{Recipient: "+905551234567", Channel: model.ChannelSMS, Content: "Hi", Priority: model.PriorityNormal},
		}}
		if errs := r.Validate(); len(errs) != 0 {
			t.Errorf("got %d errors", len(errs))
		}
	})
}

func TestNormalize(t *testing.T) {
	r := model.ListNotificationsRequest{Page: -1, PageSize: 500}
	r.Normalize()
	if r.Page != 1 || r.PageSize != 100 {
		t.Errorf("page=%d size=%d", r.Page, r.PageSize)
	}
}

func TestPriorityInt(t *testing.T) {
	if model.PriorityHigh.Int() != 3 || model.PriorityNormal.Int() != 2 || model.PriorityLow.Int() != 1 {
		t.Error("priority int mismatch")
	}
}
