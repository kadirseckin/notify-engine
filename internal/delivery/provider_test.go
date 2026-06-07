package delivery_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"notify-engine/internal/config"
	"notify-engine/internal/delivery"
)

func TestSend_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(202)
		_ = json.NewEncoder(w).Encode(delivery.DeliveryResponse{MessageID: "msg-1", Status: "accepted", Timestamp: time.Now().Format(time.RFC3339)})
	}))
	defer srv.Close()
	p := delivery.NewWebhookProvider(config.ProviderConfig{WebhookURL: srv.URL, RequestTimeout: 5 * time.Second})
	resp, err := p.Send(context.Background(), delivery.DeliveryRequest{To: "+905551234567", Channel: "sms", Content: "Hi"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.MessageID != "msg-1" {
		t.Errorf("got %s", resp.MessageID)
	}
}

func TestSend_500_Retryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) }))
	defer srv.Close()
	p := delivery.NewWebhookProvider(config.ProviderConfig{WebhookURL: srv.URL, RequestTimeout: 5 * time.Second})
	_, err := p.Send(context.Background(), delivery.DeliveryRequest{To: "+905551234567", Channel: "sms", Content: "Hi"})
	pe, ok := err.(*delivery.ProviderError)
	if !ok {
		t.Fatal("expected ProviderError")
	}
	if !pe.Retryable {
		t.Error("500 should be retryable")
	}
}

func TestSend_400_NotRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(400) }))
	defer srv.Close()
	p := delivery.NewWebhookProvider(config.ProviderConfig{WebhookURL: srv.URL, RequestTimeout: 5 * time.Second})
	_, err := p.Send(context.Background(), delivery.DeliveryRequest{To: "+905551234567", Channel: "sms", Content: "Hi"})
	pe, ok := err.(*delivery.ProviderError)
	if !ok {
		t.Fatal("expected ProviderError")
	}
	if pe.Retryable {
		t.Error("400 should NOT be retryable")
	}
}

func TestSend_429_Retryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(429) }))
	defer srv.Close()
	p := delivery.NewWebhookProvider(config.ProviderConfig{WebhookURL: srv.URL, RequestTimeout: 5 * time.Second})
	_, err := p.Send(context.Background(), delivery.DeliveryRequest{To: "+905551234567", Channel: "sms", Content: "Hi"})
	pe, ok := err.(*delivery.ProviderError)
	if !ok {
		t.Fatal("expected ProviderError")
	}
	if !pe.Retryable {
		t.Error("429 should be retryable")
	}
}
