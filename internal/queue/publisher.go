package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"notify-engine/internal/config"
	"notify-engine/internal/model"
	"notify-engine/internal/telemetry"
)

type Publisher interface {
	Publish(ctx context.Context, notification *model.Notification) error
	Close() error
}

type rabbitPublisher struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	cfg     config.RabbitMQConfig
	logger  *slog.Logger
}

func NewPublisher(cfg config.RabbitMQConfig, logger *slog.Logger) (Publisher, error) {
	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq connect: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("rabbitmq channel: %w", err)
	}
	p := &rabbitPublisher{conn: conn, channel: ch, cfg: cfg, logger: logger}
	if err := p.setup(); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, err
	}
	return p, nil
}

func (p *rabbitPublisher) setup() error {
	if err := p.channel.ExchangeDeclare(p.cfg.Exchange, "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare exchange: %w", err)
	}
	for _, ch := range []model.Channel{model.ChannelSMS, model.ChannelEmail, model.ChannelPush} {
		qName := fmt.Sprintf("%s.%s", p.cfg.QueuePrefix, ch)
		dlqName := fmt.Sprintf("%s.%s", p.cfg.DLQPrefix, ch)

		if _, err := p.channel.QueueDeclare(dlqName, true, false, false, false, nil); err != nil {
			return fmt.Errorf("declare DLQ %s: %w", dlqName, err)
		}
		if err := p.channel.QueueBind(dlqName, fmt.Sprintf("dlq.%s", ch), p.cfg.Exchange, false, nil); err != nil {
			return fmt.Errorf("bind DLQ: %w", err)
		}

		args := amqp.Table{
			"x-max-priority":            int32(3),
			"x-dead-letter-exchange":    p.cfg.Exchange,
			"x-dead-letter-routing-key": fmt.Sprintf("dlq.%s", ch),
		}
		if _, err := p.channel.QueueDeclare(qName, true, false, false, false, args); err != nil {
			return fmt.Errorf("declare queue %s: %w", qName, err)
		}
		if err := p.channel.QueueBind(qName, fmt.Sprintf("notification.%s", ch), p.cfg.Exchange, false, nil); err != nil {
			return fmt.Errorf("bind queue: %w", err)
		}
	}
	p.logger.Info("RabbitMQ topology declared")
	return nil
}

func (p *rabbitPublisher) Publish(ctx context.Context, notification *model.Notification) error {
	ctx, span := otel.Tracer(telemetry.Name).Start(ctx, "queue.publish")
	defer span.End()
	span.SetAttributes(
		attribute.String("messaging.system", "rabbitmq"),
		attribute.String("messaging.destination", fmt.Sprintf("notification.%s", notification.Channel)),
		attribute.String("notification.id", notification.ID.String()),
		attribute.String("notification.channel", string(notification.Channel)),
	)

	body, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	headers := amqp.Table{}
	otel.GetTextMapPropagator().Inject(ctx, amqpCarrier(headers))

	return p.channel.PublishWithContext(ctx, p.cfg.Exchange, fmt.Sprintf("notification.%s", notification.Channel),
		false, false, amqp.Publishing{
			DeliveryMode: amqp.Persistent, ContentType: "application/json",
			Priority: uint8(notification.Priority.Int()), MessageId: notification.ID.String(),
			Headers: headers, Body: body,
		})
}

type amqpCarrier amqp.Table

func (c amqpCarrier) Get(key string) string {
	val, ok := amqp.Table(c)[key]
	if !ok {
		return ""
	}
	s, _ := val.(string)
	return s
}

func (c amqpCarrier) Set(key, value string) {
	amqp.Table(c)[key] = value
}

func (c amqpCarrier) Keys() []string {
	keys := make([]string, 0, len(amqp.Table(c)))
	for k := range amqp.Table(c) {
		keys = append(keys, k)
	}
	return keys
}

func (p *rabbitPublisher) Close() error {
	_ = p.channel.Close()
	return p.conn.Close()
}
