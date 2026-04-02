-- Add AmneziaWG server configuration columns to vpn_servers.
--
-- awg_public_key   — the server's X25519 public key (Base64, 44 chars).
--                    Sent to clients so they can verify the server identity.
-- awg_endpoint     — the public UDP endpoint, e.g. "95.216.1.1:51820".
--                    Clients use this as the WireGuard Endpoint= value.
-- awg_params       — JSON object with AmneziaWG obfuscation parameters:
--                    { "jc": 5, "jmin": 50, "jmax": 1000,
--                      "s1": 0, "s2": 0, "h1": 0, "h2": 0, "h3": 0, "h4": 0 }
--                    Null means the server does not support AmneziaWG.

ALTER TABLE vpn_servers
    ADD COLUMN IF NOT EXISTS awg_public_key VARCHAR(64),
    ADD COLUMN IF NOT EXISTS awg_endpoint   VARCHAR(255),
    ADD COLUMN IF NOT EXISTS awg_params     JSONB;

-- Index on awg_public_key so the API can quickly identify AWG-capable servers.
CREATE INDEX IF NOT EXISTS idx_vpn_servers_awg ON vpn_servers(awg_public_key)
    WHERE awg_public_key IS NOT NULL;
