CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TYPE notification_channel  AS ENUM ('sms', 'email', 'push');
CREATE TYPE notification_status   AS ENUM ('pending', 'queued', 'sending', 'sent', 'failed', 'cancelled');
CREATE TYPE notification_priority AS ENUM ('high', 'normal', 'low');

CREATE TABLE notifications (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    batch_id            UUID,
    idempotency_key     VARCHAR(255),
    recipient           VARCHAR(512)          NOT NULL,
    channel             notification_channel  NOT NULL,
    content             TEXT                  NOT NULL,
    subject             VARCHAR(512),
    priority            notification_priority NOT NULL DEFAULT 'normal',
    status              notification_status   NOT NULL DEFAULT 'pending',
    provider_message_id VARCHAR(255),
    retry_count         INT                   NOT NULL DEFAULT 0,
    max_retries         INT                   NOT NULL DEFAULT 3,
    last_error          TEXT,
    scheduled_at        TIMESTAMPTZ,
    sent_at             TIMESTAMPTZ,
    created_at          TIMESTAMPTZ           NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ           NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_notifications_status           ON notifications (status);
CREATE INDEX idx_notifications_channel_status   ON notifications (channel, status);
CREATE INDEX idx_notifications_batch_id         ON notifications (batch_id) WHERE batch_id IS NOT NULL;
CREATE UNIQUE INDEX idx_notifications_idempotency ON notifications (idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX idx_notifications_created_at       ON notifications (created_at DESC);
CREATE INDEX idx_notifications_pending           ON notifications (status, created_at) WHERE status = 'pending';
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_notifications_updated_at
    BEFORE UPDATE ON notifications
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
