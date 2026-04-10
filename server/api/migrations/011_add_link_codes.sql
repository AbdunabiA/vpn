-- Link codes table — short-lived 6-digit codes that allow a friend's device
-- to attach to an existing plan owner's account.
--
-- Flow:
--   1. Plan owner calls POST /devices/share-code → row inserted, code returned.
--   2. Friend's device calls POST /auth/link with code + device_id → row
--      consumed (deleted) and tokens for the owner's user_id are returned.
--      The friend's device row is reassigned to the owner.
--
-- Codes expire after a few minutes regardless of redemption.

CREATE TABLE IF NOT EXISTS link_codes (
    code        VARCHAR(10) PRIMARY KEY,
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  TIMESTAMP   NOT NULL DEFAULT NOW(),
    expires_at  TIMESTAMP   NOT NULL
);

-- Cleanup index — the scheduler will periodically delete expired codes
-- and the share-code endpoint refuses to issue more than one code per minute
-- per user (rate-limited via existing per-user middleware).
CREATE INDEX IF NOT EXISTS idx_link_codes_expires_at ON link_codes(expires_at);
CREATE INDEX IF NOT EXISTS idx_link_codes_user_id    ON link_codes(user_id);
