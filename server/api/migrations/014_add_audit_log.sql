-- Audit log — records every mutating action performed by an admin via
-- the /api/v1/admin/* endpoints. Populated by middleware.AuditLog which
-- wraps the write handlers and commits a row on success.
--
-- Why this exists:
--   Phase B-2 replaced the curl workflow for activating Telegram-paid
--   subscriptions. Without an audit trail, there is no record of who
--   upgraded which user, when, or for how many days. This is a
--   compliance-grade gap for anything that interacts with payments.
--
-- Why JSONB for details:
--   Different actions carry different shapes (update_user has tier +
--   extend_days, delete_device has device_id, update_server has
--   whatever the PATCH body contained). A schemaless blob is the
--   simplest way to capture all of them without a new migration every
--   time a new handler is audited.

CREATE TABLE IF NOT EXISTS audit_log (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    admin_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    action      VARCHAR(64) NOT NULL,
    target_id   UUID,
    details     JSONB,
    ip          VARCHAR(45),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes cover the three query shapes the UI needs:
--   * recent activity for the dashboard / audit log page (created_at)
--   * "what has this admin done?"     (admin_id)
--   * "who touched this user?"        (target_id)
CREATE INDEX IF NOT EXISTS idx_audit_log_created_at ON audit_log(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_log_admin_id   ON audit_log(admin_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_target_id  ON audit_log(target_id);
