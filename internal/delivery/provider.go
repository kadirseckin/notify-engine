package delivery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"notify-engine/internal/config"
	"notify-engine/internal/telemetry"
)

type Provider interface {
	Send(ctx context.Context, req DeliveryRequest) (*DeliveryResponse, error)
}

type DeliveryRequest struct {
	To      string `json:"to"`
	Channel string `json:"channel"`
	Content string `json:"content"`
	Subject string `json:"subject,omitempty"`
}

type DeliveryResponse struct {
	MessageID string `json:"messageId"`
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

type webhookProvider struct {
	client     *http.Client
	webhookURL string
}

func NewWebhookProvider(cfg config.ProviderConfig) Provider {
	return &webhookProvider{client: &http.Client{Timeout: cfg.RequestTimeout}, webhookURL: cfg.WebhookURL}
}

func (p *webhookProvider) Send(ctx context.Context, req DeliveryRequest) (*DeliveryResponse, error) {
	ctx, span := otel.Tracer(telemetry.Name).Start(ctx, "delivery.send")
	defer span.End()
	span.SetAttributes(
		attribute.String("delivery.channel", req.Channel),
		attribute.String("delivery.to", req.To),
	)

	body, err := json.Marshal(req)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("marshal: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.webhookURL, bytes.NewReader(body))
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		span.SetStatus(codes.Error, fmt.Sprintf("provider status %d", resp.StatusCode))
		return nil, &ProviderError{StatusCode: resp.StatusCode, Body: string(respBody),
			Retryable: resp.StatusCode == 429 || resp.StatusCode >= 500}
	}

	var dr DeliveryResponse
	if err := json.Unmarshal(respBody, &dr); err != nil || dr.MessageID == "" {
		dr = DeliveryResponse{MessageID: fmt.Sprintf("wh_%d", time.Now().UnixNano()), Status: "accepted",
			Timestamp: time.Now().UTC().Format(time.RFC3339)}
	}
	return &dr, nil
}

type ProviderError struct {
	StatusCode int
	Body       string
	Retryable  bool
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("provider returned status %d: %s", e.StatusCode, e.Body)
}
