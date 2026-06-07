CREATE TABLE templates (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name        VARCHAR(255)          NOT NULL UNIQUE,
    channel     notification_channel  NOT NULL,
    subject     VARCHAR(512),
    body        TEXT                  NOT NULL,
    created_at  TIMESTAMPTZ           NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ           NOT NULL DEFAULT NOW()
);

CREATE TRIGGER update_templates_updated_at
    BEFORE UPDATE ON templates
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Seed example templates
INSERT INTO templates (id, name, channel, subject, body) VALUES
    (uuid_generate_v4(), 'otp_sms', 'sms', NULL, 'Your verification code is {{code}}. Valid for {{minutes}} minutes.'),
    (uuid_generate_v4(), 'welcome_email', 'email', 'Welcome to {{company}}!', '<h1>Welcome, {{name}}!</h1><p>Thanks for joining {{company}}.</p>'),
    (uuid_generate_v4(), 'order_push', 'push', NULL, 'Your order #{{order_id}} has been {{status}}.');
