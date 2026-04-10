-- Devices table — tracks unique physical devices that have authenticated.
-- The device_id is the OS-issued stable identifier:
--   * Android: Settings.Secure.ANDROID_ID
--   * iOS:     UIDevice.identifierForVendor.uuidString
-- Both reset on factory reset / app reinstall (iOS) so they are stable
-- enough to defeat trivial reinstall abuse but still privacy-friendly.
--
-- A device is uniquely owned by exactly one user_id at a time. When a
-- device redeems a link code (Phase C), its user_id is updated to point
-- at the plan owner so that limit enforcement counts the device against
-- the owner's quota.

CREATE TABLE IF NOT EXISTS devices (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id     VARCHAR(255) NOT NULL,
    platform      VARCHAR(20)  NOT NULL DEFAULT '',
    model         VARCHAR(255) NOT NULL DEFAULT '',
    first_seen_at TIMESTAMP    NOT NULL DEFAULT NOW(),
    last_seen_at  TIMESTAMP    NOT NULL DEFAULT NOW()
);

-- One device → one current owner. When a link code is redeemed the row
-- is UPDATEd in place (not duplicated), so the unique index is safe.
CREATE UNIQUE INDEX IF NOT EXISTS idx_devices_device_id ON devices(device_id);

-- Lookup by user for "List my devices" admin/UI calls.
CREATE INDEX IF NOT EXISTS idx_devices_user_id ON devices(user_id);
