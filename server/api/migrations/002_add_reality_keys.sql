-- Add REALITY key columns to vpn_servers
-- Each server has its own X25519 key pair for REALITY

ALTER TABLE vpn_servers
    ADD COLUMN IF NOT EXISTS reality_public_key VARCHAR(64),
    ADD COLUMN IF NOT EXISTS reality_short_id VARCHAR(16) DEFAULT 'abcd1234';

-- Update the seeded servers with the generated key
UPDATE vpn_servers SET
    reality_public_key = 'OAmaJn5JqNlYdNIulgafHAwZs8MLLuU8MXs9rt26sl0',
    reality_short_id = 'abcd1234';
