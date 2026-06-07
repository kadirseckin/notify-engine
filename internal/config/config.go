package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Server    ServerConfig
	Database  DatabaseConfig
	RabbitMQ  RabbitMQConfig
	Redis     RedisConfig
	Provider  ProviderConfig
	Worker    WorkerConfig
	Telemetry TelemetryConfig
}

type TelemetryConfig struct {
	OTLPEndpoint string
	ServiceName  string
}

type ServerConfig struct {
	Host string
	Port int
}

type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.DBName, d.SSLMode)
}

type RabbitMQConfig struct {
	URL           string
	Exchange      string
	QueuePrefix   string
	DLQPrefix     string
	PrefetchCount int
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type ProviderConfig struct {
	WebhookURL     string
	RequestTimeout time.Duration
}

type WorkerConfig struct {
	Concurrency     int
	MaxRetries      int
	RetryBaseDelay  time.Duration
	RateLimitPerSec int
}

func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			Port: getEnvInt("SERVER_PORT", 8080),
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnvInt("DB_PORT", 5432),
			User:     getEnv("DB_USER", "notification"),
			Password: getEnv("DB_PASSWORD", "notification"),
			DBName:   getEnv("DB_NAME", "notification_db"),
			SSLMode:  getEnv("DB_SSL_MODE", "disable"),
		},
		RabbitMQ: RabbitMQConfig{
			URL:           getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
			Exchange:      getEnv("RABBITMQ_EXCHANGE", "notifications"),
			QueuePrefix:   getEnv("RABBITMQ_QUEUE_PREFIX", "notifications"),
			DLQPrefix:     getEnv("RABBITMQ_DLQ_PREFIX", "notifications.dlq"),
			PrefetchCount: getEnvInt("RABBITMQ_PREFETCH", 10),
		},
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "localhost:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		Provider: ProviderConfig{
			WebhookURL:     getEnv("PROVIDER_WEBHOOK_URL", "https://webhook.site/your-uuid-here"),
			RequestTimeout: getEnvDuration("PROVIDER_TIMEOUT", 10*time.Second),
		},
		Worker: WorkerConfig{
			Concurrency:     getEnvInt("WORKER_CONCURRENCY", 5),
			MaxRetries:      getEnvInt("WORKER_MAX_RETRIES", 3),
			RetryBaseDelay:  getEnvDuration("WORKER_RETRY_BASE_DELAY", 5*time.Second),
			RateLimitPerSec: getEnvInt("WORKER_RATE_LIMIT_PER_SEC", 100),
		},
		Telemetry: TelemetryConfig{
			OTLPEndpoint: getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4318"),
			ServiceName:  getEnv("OTEL_SERVICE_NAME", "notify-engine"),
		},
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
