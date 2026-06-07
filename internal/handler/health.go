package handler

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"

	"notify-engine/internal/config"
	"notify-engine/internal/model"
)

type HealthHandler struct {
	db      *sqlx.DB
	rdb     *redis.Client
	amqpURL string
	cfg     *config.Config

	// Cached AMQP connection for health checks
	amqpConn *amqp.Connection
	amqpMu   sync.Mutex
}

func NewHealthHandler(db *sqlx.DB, rdb *redis.Client, amqpURL string, cfg *config.Config) *HealthHandler {
	return &HealthHandler{db: db, rdb: rdb, amqpURL: amqpURL, cfg: cfg}
}

// getAMQPConn returns a cached AMQP connection, reconnecting if needed.
func (h *HealthHandler) getAMQPConn() (*amqp.Connection, error) {
	h.amqpMu.Lock()
	defer h.amqpMu.Unlock()

	if h.amqpConn != nil && !h.amqpConn.IsClosed() {
		return h.amqpConn, nil
	}

	conn, err := amqp.DialConfig(h.amqpURL, amqp.Config{Dial: amqp.DefaultDial(2 * time.Second)})
	if err != nil {
		return nil, err
	}
	h.amqpConn = conn
	return conn, nil
}

func (h *HealthHandler) Health(c *gin.Context) {
	services := map[string]string{}

	// PostgreSQL
	if err := h.db.Ping(); err != nil {
		services["postgres"] = "unhealthy: " + err.Error()
	} else {
		services["postgres"] = "healthy"
	}

	// Redis
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if err := h.rdb.Ping(ctx).Err(); err != nil {
		services["redis"] = "unhealthy: " + err.Error()
	} else {
		services["redis"] = "healthy"
	}

	// RabbitMQ (cached connection)
	if _, err := h.getAMQPConn(); err != nil {
		services["rabbitmq"] = "unhealthy: " + err.Error()
	} else {
		services["rabbitmq"] = "healthy"
	}

	status, code := "healthy", http.StatusOK
	for _, v := range services {
		if v != "healthy" {
			status, code = "degraded", http.StatusServiceUnavailable
			break
		}
	}
	c.JSON(code, model.HealthResponse{Status: status, Services: services})
}

func (h *HealthHandler) Metrics(c *gin.Context) {
	var stats []struct {
		Channel string `db:"channel"`
		Status  string `db:"status"`
		Count   int64  `db:"count"`
	}
	if err := h.db.SelectContext(c.Request.Context(), &stats,
		"SELECT channel, status, COUNT(*) as count FROM notifications GROUP BY channel, status"); err != nil {
		c.JSON(http.StatusInternalServerError, model.APIResponse{
			Success: false, Error: &model.APIError{Code: model.ErrCodeMetrics, Message: err.Error()}})
		return
	}

	sr := map[string]float64{}
	fr := map[string]float64{}
	var totalSent, totalFailed, totalPending int64
	ct := map[string]int64{}
	cs := map[string]int64{}
	cf := map[string]int64{}

	perChannel := map[string]model.ChannelMetrics{}
	for _, s := range stats {
		ct[s.Channel] += s.Count
		cm := perChannel[s.Channel]
		switch model.Status(s.Status) {
		case model.StatusSent:
			cs[s.Channel] += s.Count
			totalSent += s.Count
			cm.Sent += s.Count
		case model.StatusFailed:
			cf[s.Channel] += s.Count
			totalFailed += s.Count
			cm.Failed += s.Count
		case model.StatusPending:
			totalPending += s.Count
			cm.Pending += s.Count
		case model.StatusQueued:
			totalPending += s.Count
			cm.Queued += s.Count
		}
		perChannel[s.Channel] = cm
	}
	for ch := range ct {
		completed := cs[ch] + cf[ch]
		if completed > 0 {
			sr[ch] = float64(cs[ch]) / float64(completed) * 100
			fr[ch] = float64(cf[ch]) / float64(completed) * 100
		}
	}

	// Queue depth from RabbitMQ (cached connection)
	queueDepth := map[string]int64{}
	conn, err := h.getAMQPConn()
	if err == nil {
		ch, err := conn.Channel()
		if err == nil {
			for _, channel := range []string{"sms", "email", "push"} {
				qName := fmt.Sprintf("%s.%s", h.cfg.RabbitMQ.QueuePrefix, channel)
				q, err := ch.QueueDeclarePassive(qName, true, false, false, false, nil)
				if err == nil {
					queueDepth[channel] = int64(q.Messages)
				}
			}
			_ = ch.Close()
		}
	}

	// Avg latency per channel
	avgLatency := map[string]float64{}
	var latencyStats []struct {
		Channel    string  `db:"channel"`
		AvgLatency float64 `db:"avg_latency_ms"`
	}
	if err := h.db.SelectContext(c.Request.Context(), &latencyStats,
		`SELECT channel, AVG(EXTRACT(EPOCH FROM (sent_at - created_at)) * 1000) as avg_latency_ms
		 FROM notifications WHERE sent_at IS NOT NULL GROUP BY channel`); err != nil {
		c.JSON(http.StatusInternalServerError, model.APIResponse{
			Success: false, Error: &model.APIError{Code: model.ErrCodeMetrics, Message: err.Error()}})
		return
	}
	for _, l := range latencyStats {
		avgLatency[l.Channel] = math.Round(l.AvgLatency*100) / 100
	}

	c.JSON(http.StatusOK, model.MetricsResponse{
		QueueDepth:   queueDepth,
		SuccessRate:  sr,
		FailureRate:  fr,
		AvgLatencyMs: avgLatency,
		TotalSent:    totalSent,
		TotalFailed:  totalFailed,
		TotalPending: totalPending,
		PerChannel:   perChannel,
	})
}
