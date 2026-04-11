-- Telegram-based account recovery (ADR-006, Phase TG-1).
--
-- Adds two nullable columns to the users table so a user can
-- optionally bind their Telegram account as a stable recovery
-- identifier. Survives iOS IDFV resets, Android factory resets,
-- phone swaps, and cross-platform switches — Telegram user IDs
-- don't change for any of those events.
--
-- The binding is entirely opt-in. Free users can stay anonymous
-- forever; only users who pay for premium have a reason to link
-- their Telegram, and they do it voluntarily from the mobile
-- app's Account screen.
--
-- UNIQUE on telegram_user_id means one Telegram account can recover
-- exactly one VPN user. The share-code flow handles the "multiple
-- devices on one plan" case separately: the Telegram-linked account
-- is the owner, and friends who redeem share codes don't need their
-- own Telegram link.

ALTER TABLE users
    ADD COLUMN telegram_user_id BIGINT,
    ADD COLUMN telegram_linked_at TIMESTAMPTZ;

-- Partial UNIQUE index — only rows where telegram_user_id IS NOT
-- NULL are constrained. Keeps the constraint enforceable even when
-- most users haven't linked anything.
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_telegram_user_id
    ON users(telegram_user_id)
    WHERE telegram_user_id IS NOT NULL;
