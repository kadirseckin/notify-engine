DROP TRIGGER IF EXISTS update_notifications_updated_at ON notifications;
DROP FUNCTION IF EXISTS update_updated_at_column();
DROP TABLE IF EXISTS notifications;
DROP TYPE IF EXISTS notification_priority;
DROP TYPE IF EXISTS notification_status;
DROP TYPE IF EXISTS notification_channel;
