ALTER TABLE connections ADD COLUMN IF NOT EXISTS status VARCHAR(20) NOT NULL DEFAULT 'connected';
ALTER TABLE connections ADD COLUMN IF NOT EXISTS last_heartbeat_at TIMESTAMPTZ;
UPDATE connections SET last_heartbeat_at = connected_at WHERE last_heartbeat_at IS NULL AND disconnected_at IS NULL;
