-- Add WebSocket CDN transport fields to vpn_servers.
-- These fields enable VLESS-over-WebSocket routing through Cloudflare CDN
-- for users whose networks block direct connections to VPN servers.
--
-- ws_host:    The Cloudflare-proxied domain clients connect to (e.g. vpn.example.com).
--             NULL means WebSocket CDN is not configured for this server.
-- ws_path:    The WebSocket upgrade path to use (must match xray-core and Nginx config).
-- ws_enabled: Master toggle; clients only receive WS config when this is true AND
--             ws_host is non-null.

ALTER TABLE vpn_servers ADD COLUMN IF NOT EXISTS ws_host    VARCHAR(255);
ALTER TABLE vpn_servers ADD COLUMN IF NOT EXISTS ws_path    VARCHAR(255) DEFAULT '/ws';
ALTER TABLE vpn_servers ADD COLUMN IF NOT EXISTS ws_enabled BOOLEAN      DEFAULT false;
