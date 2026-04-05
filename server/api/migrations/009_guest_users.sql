-- Allow guest users: email_hash and password_hash are nullable
-- Guest accounts have no email or password — identified by UUID name only
ALTER TABLE users ALTER COLUMN email_hash DROP NOT NULL;
ALTER TABLE users ALTER COLUMN password_hash DROP NOT NULL;
