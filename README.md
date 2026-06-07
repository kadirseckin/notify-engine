# Notify Engine

Event-driven notification system that processes and delivers messages through SMS, Email, and Push channels with async queue processing, rate limiting, priority queues, and reliable delivery.

## Architecture

```
Client → API (Gin) → Validate + Idempotency → PostgreSQL (insert)
                                  ↓
                           RabbitMQ (priority queue per channel)
                                  ↓
                    Worker → Rate Limiter (Redis) → Deliver (webhook.site)
                                  ↓ fail
                         Retry (exp backoff) or DLQ
```

## Quick Start

**Prerequisites:** Docker & Docker Compose only. No Go installation needed to run.

### Step 1 — Get a webhook URL

Go to [webhook.site](https://webhook.site), copy the unique URL shown on the page (e.g. `https://webhook.site/abc-123`). This is where delivered notifications will appear.

### Step 2 — Clone & configure

```bash
git clone https://github.com/kadirseckin/notify-engine.git
cd notify-engine
cp .env.example .env
```

Open `.env` and replace the webhook URL:
```
PROVIDER_WEBHOOK_URL=https://webhook.site/YOUR-UUID-HERE
```

### Step 3 — Start everything

```bash
docker-compose up --build -d
```

This starts PostgreSQL, RabbitMQ, Redis, Jaeger, runs migrations, and starts the API + Worker. Wait ~15 seconds for all services to be healthy.

### Step 4 — Verify it's running

```bash
curl http://localhost:8080/health
```

Expected:
```json
{"status":"healthy","services":{"postgres":"healthy","rabbitmq":"healthy","redis":"healthy"}}
```

### Step 5 — Send a notification

```bash
curl -X POST http://localhost:8080/api/v1/notifications \
  -H "Content-Type: application/json" \
  -d '{"recipient":"+905551234567","channel":"sms","content":"Hello from notify-engine","priority":"high"}'
```

Within 1-2 seconds the status changes from `queued` → `sent` and the request appears on your webhook.site page.

### Step 6 — View the trace in Jaeger

Open **http://localhost:16686**, select service **notify-engine-api**, click **Find Traces**. You'll see the full end-to-end trace: HTTP handler → service → RabbitMQ publish → worker consume → delivery.

### Other useful endpoints

| URL | What it shows |
|-----|--------------|
| http://localhost:8080/swagger | Interactive API docs |
| http://localhost:8080/metrics | Queue depth, latency, success rates |
| http://localhost:16686 | Jaeger distributed traces |
| http://localhost:15672 | RabbitMQ management (guest/guest) |

### Run unit tests

```bash
go test ./... -v
```

### Step 7 — Run the full Postman collection

1. Open Postman
2. Import `docs/Notify_Engine.postman_collection.json`
3. Right-click **"Notify Engine API"** → **Run collection**
4. All 40 requests run in order with automated assertions

The collection auto-captures IDs between requests — just run in order. Tests cover all CRUD operations, channel-specific validation, idempotency, batch with partial reject, pagination, cancel lifecycle, and template rendering.

### Cleanup

```bash
docker-compose down -v
```

## API Endpoints

| Method   | Path                               | Description                    |
|----------|------------------------------------|--------------------------------|
| `POST`   | `/api/v1/notifications`            | Create notification            |
| `POST`   | `/api/v1/notifications/batch`      | Batch create (max 1000)        |
| `GET`    | `/api/v1/notifications/:id`        | Get by ID                      |
| `GET`    | `/api/v1/notifications/batch/:bid` | Get by batch ID                |
| `DELETE` | `/api/v1/notifications/:id`        | Cancel pending                 |
| `GET`    | `/api/v1/notifications`            | List + filter + paginate       |
| `GET`    | `/health`                          | Health check                   |
| `GET`    | `/metrics`                         | Real-time metrics              |
| `POST`   | `/api/v1/templates`                | Create template                |
| `GET`    | `/api/v1/templates`                | List all templates             |
| `GET`    | `/api/v1/templates/:id`            | Get template by ID             |
| `DELETE` | `/api/v1/templates/:id`            | Delete template                |
| `GET`    | `/swagger`                         | Interactive API documentation  |

### API Documentation

Interactive Swagger UI is available at `http://localhost:8080/swagger` when the server is running. The raw OpenAPI spec is at `/swagger/spec`.

You can also view the spec at [editor.swagger.io](https://editor.swagger.io) by pasting the contents of `docs/swagger.yaml`.

### Filtering & Pagination

```
GET /api/v1/notifications?status=sent&channel=sms&page=1&page_size=10
GET /api/v1/notifications?start_date=2026-06-01T00:00:00Z&end_date=2026-06-30T23:59:59Z
```

## API Examples

### Create SMS
```bash
curl -X POST http://localhost:8080/api/v1/notifications \
  -H "Content-Type: application/json" \
  -d '{
    "recipient": "+905551234567",
    "channel": "sms",
    "content": "Your verification code is 4821",
    "priority": "high",
    "idempotency_key": "otp-user123-20260606"
  }'
```

### Create Email
```bash
curl -X POST http://localhost:8080/api/v1/notifications \
  -H "Content-Type: application/json" \
  -d '{
    "recipient": "user@example.com",
    "channel": "email",
    "content": "<h1>Welcome!</h1>",
    "subject": "Welcome!",
    "priority": "normal"
  }'
```

### Batch Create
```bash
curl -X POST http://localhost:8080/api/v1/notifications/batch \
  -H "Content-Type: application/json" \
  -d '{
    "notifications": [
      {"recipient": "+905551111111", "channel": "sms", "content": "Flash sale!", "priority": "high"},
      {"recipient": "user@test.com", "channel": "email", "content": "Sale details", "subject": "Sale", "priority": "normal"}
    ]
  }'
```

### Scheduled Notification
```bash
curl -X POST http://localhost:8080/api/v1/notifications \
  -H "Content-Type: application/json" \
  -d '{
    "recipient": "+905559876543",
    "channel": "sms",
    "content": "Reminder: appointment tomorrow",
    "priority": "normal",
    "scheduled_at": "2026-06-07T10:00:00Z"
  }'
```

### Send with Template
```bash
# List available templates
curl http://localhost:8080/api/v1/templates

# Send using template (no content needed — template renders it)
curl -X POST http://localhost:8080/api/v1/notifications \
  -H "Content-Type: application/json" \
  -d '{
    "recipient": "+905551234567",
    "channel": "sms",
    "priority": "high",
    "template_id": "TEMPLATE-UUID-HERE",
    "variables": {"code": "9876", "minutes": "5"}
  }'
# Result content: "Your verification code is 9876. Valid for 5 minutes."
```

## Key Design Decisions

### Rate Limiting
Redis-based sliding window with Lua script for atomic INCR+PEXPIRE. 100 messages/sec/channel. Zero race conditions — single round-trip to Redis.

### Priority Queue
RabbitMQ `x-max-priority:3`. Three levels: high=3, normal=2, low=1. Higher priority messages dequeued first.

### Idempotency
Partial unique index on `idempotency_key WHERE idempotency_key IS NOT NULL`. Service layer checks before insert — returns existing notification if key matches. DB-level unique constraint prevents race conditions.

### Retry & Dead Letter Queue
- Retryable errors (429, 5xx): exponential backoff (5s → 10s → 20s), max 3 retries
- Permanent errors (4xx except 429): immediate DLQ, no retry
- Each channel has its own DLQ (`notifications.dlq.sms`, `notifications.dlq.email`, `notifications.dlq.push`)

### Graceful Shutdown
Both API and Worker handle SIGINT/SIGTERM. Worker waits for in-flight messages before closing connections.

### Scheduled Notifications
Scheduler polls DB every 10 seconds for pending notifications where `scheduled_at <= NOW()`, publishes them to the queue.

### Content Validation
Channel-specific rules enforced at API level:
- SMS: E.164 phone format, max 160 characters
- Email: valid email address, subject required, max 50000 characters  
- Push: device token required, max 256 characters

### Distributed Tracing
OpenTelemetry SDK with OTLP/HTTP exporter. Trace context is propagated across service boundaries via W3C TraceContext headers embedded in RabbitMQ message headers — so a single notification request produces one unified trace that spans both the API and the Worker process. All major operations (HTTP handler, service layer, queue publish/consume, rate limiter, delivery) are individually instrumented with span attributes.

### Template System
Reusable message templates with `{{variable}}` placeholders. Templates are channel-specific and stored in DB. When creating a notification, pass `template_id` + `variables` instead of content — the service renders the template by replacing placeholders. Channel mismatch between template and notification is validated.

## Project Structure

```
notify-engine/
├── cmd/
│   ├── api/main.go              # API server entrypoint
│   ├── worker/main.go           # Queue worker entrypoint
│   └── migrate/main.go          # Migration runner
├── internal/
│   ├── model/                   # Domain entities, DTOs, validation
│   ├── config/                  # Environment-based configuration
│   ├── repository/              # PostgreSQL data access layer
│   ├── service/                 # Business logic (idempotency, batch)
│   ├── handler/                 # HTTP handlers (Gin)
│   ├── queue/                   # RabbitMQ publisher, consumer, scheduler
│   ├── delivery/                # External provider (webhook.site)
│   ├── ratelimiter/             # Redis rate limiter (Lua script)
│   ├── telemetry/               # OpenTelemetry tracer init (Jaeger/OTLP)
│   └── middleware/              # Correlation ID, request logging
├── migrations/                  # Versioned SQL migrations
├── docs/
│   ├── swagger.yaml             # OpenAPI 3.0 spec
│   └── Notify_Engine.postman_collection.json  # 40 requests, 76 assertions
├── .github/workflows/ci.yml     # GitHub Actions (lint + test + build)
├── docker-compose.yml           # One-command infrastructure
├── Dockerfile                   # Multi-stage build
├── Makefile                     # Common commands
└── README.md
```

## Testing

### Unit Tests (24 tests)
```bash
go test ./... -v
```

| Package    | Tests | Coverage |
|------------|-------|----------|
| model      | 6     | Validation rules (SMS/Email/Push/Batch) |
| handler    | 6     | HTTP responses, error handling |
| service    | 9     | Idempotency, batch, cancel, scheduling |
| delivery   | 4     | Provider success/fail, retry decisions |

### Postman Collection (40 requests)

Import `docs/Notify_Engine.postman_collection.json` and run the full collection. Tests cover:
- All CRUD operations across SMS, Email, Push
- Channel-specific validation (7 error scenarios)
- Idempotency (duplicate prevention)
- Batch with partial reject
- Pagination and filtering (status, channel, date range, combined)
- Cancel lifecycle (pending→cancelled, already cancelled→404, sent→404)
- Template CRUD, rendering, channel mismatch validation

## Distributed Tracing

Every notification request is traced end-to-end across the API and Worker using [OpenTelemetry](https://opentelemetry.io/) with [Jaeger](https://jaegertracing.io/) as the backend.

### Open Jaeger UI

```
http://localhost:16686
```

Select service **notify-engine-api** or **notify-engine-worker**, then click **Find Traces**.

### Trace Flow

```
POST /api/v1/notifications       (API — notify-engine-api)
  └─ service.CreateNotification
       └─ queue.publish          ← trace context injected into AMQP headers

            └─ worker.process    (Worker — notify-engine-worker)
                 ├─ ratelimiter.allow
                 └─ delivery.send
```

The trace spans two services. The API injects the W3C TraceContext into the RabbitMQ message headers when publishing. The Worker extracts it when consuming — both sides appear under the **same TraceID** in Jaeger.

### Instrumented Spans

| Span | Service | Attributes |
|------|---------|-----------|
| `POST /api/v1/notifications` | api | http.method, http.route, http.status_code |
| `service.CreateNotification` | api | notification.id, channel, recipient, priority |
| `queue.publish` | api | messaging.system, messaging.destination, notification.id |
| `worker.process` | worker | notification.id, channel, recipient |
| `ratelimiter.allow` | worker | ratelimiter.channel, ratelimiter.allowed |
| `delivery.send` | worker | delivery.channel, delivery.to, http.status_code |

### Configuration

| Env Variable | Default | Description |
|---|---|---|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `localhost:4318` | Jaeger OTLP endpoint |
| `OTEL_SERVICE_NAME` | `notify-engine` | Service name shown in Jaeger |

> Tracing is non-blocking — if Jaeger is unavailable the system logs a warning and continues normally.

## Monitoring

- **Swagger UI**: http://localhost:8080/swagger
- **Jaeger UI**: http://localhost:16686 — distributed traces
- **RabbitMQ Management**: http://localhost:15672 (guest/guest)
- **Health endpoint**: `GET /health` — checks PostgreSQL, Redis, RabbitMQ
- **Metrics endpoint**: `GET /metrics` — queue depth, success/failure rates, latency per channel
- **Structured logging**: JSON format with correlation IDs

## Tech Stack

Go 1.25 · Gin · PostgreSQL 16 · RabbitMQ 3.13 · Redis 7 · OpenTelemetry · Jaeger · Docker Compose
