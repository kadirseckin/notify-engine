package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const CorrelationIDHeader = "X-Correlation-ID"

func CorrelationID() gin.HandlerFunc {
	return func(c *gin.Context) {
		cid := c.GetHeader(CorrelationIDHeader)
		if cid == "" {
			cid = uuid.New().String()
		}
		c.Set("correlation_id", cid)
		c.Header(CorrelationIDHeader, cid)
		c.Next()
	}
}

func RequestLogger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		logger.Info("request",
			"correlation_id", c.GetString("correlation_id"),
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency_ms", time.Since(start).Milliseconds(),
			"client_ip", c.ClientIP(),
		)
	}
}
