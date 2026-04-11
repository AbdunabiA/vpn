-- Telegram recovery binding profile fields (ADR-006 follow-up).
--
-- Adds two nullable columns to users so the Telegram binding can
-- display a human-readable identity in the mobile app's Account
-- screen and the admin panel's user detail page. Without these
-- the UI can only render the raw numeric telegram_user_id, which
-- is unfriendly and prevents the user from confirming "yes, this
-- is my Telegram account" at a glance.
--
-- Values are captured by the bot at /start link_<token> time from
-- the Telegram Update.Message.From struct:
--   telegram_username   ← from.Username (may be empty if the user
--                         has not set a public @username)
--   telegram_first_name ← from.FirstName (always non-empty per
--                         Telegram's contract)
--
-- Both are nullable because existing linked users (there is one
-- at the time of this migration) have no way to retroactively
-- report their profile without unlinking and re-linking. The UI
-- gracefully degrades to "Привязан" without an identity line for
-- those rows until the user unlinks and relinks.

ALTER TABLE users
    ADD COLUMN telegram_username   VARCHAR(32),
    ADD COLUMN telegram_first_name VARCHAR(64);
