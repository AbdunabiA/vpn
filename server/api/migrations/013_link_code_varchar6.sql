-- Tightens link_codes.code from VARCHAR(10) to VARCHAR(6) to match the
-- actual code format produced by generateLinkCode (6 zero-padded digits).
-- The schema-as-documentation now matches reality.

ALTER TABLE link_codes ALTER COLUMN code TYPE VARCHAR(6);
