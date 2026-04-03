-- Migration: Enable AmneziaWG obfuscation on existing servers.
--
-- Sets real obfuscation parameters so AmneziaWG no longer behaves as
-- standard WireGuard (which TSPU blocks trivially).
-- s1/s2 pad handshake packets beyond WireGuard's fixed 148-byte signature.
-- h1-h4 randomize the packet header structure.

-- 1. Set AWG obfuscation params on servers that have AWG configured but zero obfuscation.
UPDATE vpn_servers
SET awg_params = jsonb_set(
    jsonb_set(
        jsonb_set(
            jsonb_set(
                jsonb_set(
                    jsonb_set(
                        COALESCE(awg_params, '{}'::jsonb),
                        '{s1}', '59'
                    ),
                    '{s2}', '59'
                ),
                '{h1}', '925816387'
            ),
            '{h2}', '1586498549'
        ),
        '{h3}', '1367025694'
    ),
    '{h4}', '2013711510'
)
WHERE is_active = true
  AND awg_params IS NOT NULL
  AND (awg_params->>'s1')::int = 0;

-- 2. Enable WebSocket only on servers that have ws_host configured.
UPDATE vpn_servers
SET ws_enabled = true
WHERE is_active = true
  AND ws_host IS NOT NULL
  AND ws_host != '';
