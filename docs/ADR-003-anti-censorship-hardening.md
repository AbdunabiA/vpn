# ADR-003: Anti-Censorship Hardening Against TSPU/DPI

**Status:** Proposed
**Date:** 2026-04-03
**Author:** Architect Agent

## Context

Russia's TSPU (Technical Means of Countering Threats) is deployed at ISP level and performs deep packet inspection (DPI) to identify and block VPN protocols. Current state of the codebase:

- **VLESS+REALITY** is the default and only enabled protocol in `config.example.json`. TSPU has been actively fingerprinting REALITY handshakes since late 2024.
- **AmneziaWG** is defined but all obfuscation parameters (`s1`, `s2`, `h1`-`h4`) are zero -- meaning it behaves as standard WireGuard, which TSPU blocks trivially.
- **WebSocket CDN** transport exists but is disabled by default.
- The client has no geo-awareness -- it tries the same protocol regardless of whether the user is behind TSPU.
- Reconnection logic retries the same protocol on failure instead of falling back to a different one.
- The API returns all protocol configs but provides no guidance on which to prefer.
- REALITY `server_names` only include Microsoft domains, which may be flagged by TSPU.

## Decision

Implement a layered anti-censorship strategy across seven work items at four priority levels. The core principle: **make every protocol look like ordinary traffic for its transport layer, and give the client enough intelligence to switch protocols when one is blocked.**

---

## Architecture Overview

```
                          TSPU / DPI Wall
                               |
          +--------------------+--------------------+
          |                    |                    |
    [WebSocket+CDN]      [AmneziaWG]        [VLESS+REALITY]
    Cloudflare proxy     Obfuscated UDP     TLS fingerprint
    Looks like HTTPS     Looks like noise    Looks like HTTPS
    to CDN domain        to random port      to server_name
          |                    |                    |
          +--------------------+--------------------+
                               |
                    Client Protocol Selector
                     (geo-aware priority)
                               |
                        Mobile App
                  (auto-fallback on drop)
```

**Data flow for protocol selection:**

```
1. App starts / user taps Connect
2. App calls GET /servers/:id/config
3. API detects client IP region from X-Forwarded-For or CF-IPCountry
4. API returns ServerConfig with protocol_priority: ["websocket","amneziawg","vless-reality"]
5. App reads user setting:
   - "auto" -> use server's protocol_priority
   - specific protocol -> use that, fall back to priority list on failure
6. Go tunnel Connect() is called with chosen protocol
7. On connection drop (not manual disconnect):
   - App advances to next protocol in priority list
   - Shows "Switching protocol..." UI state
   - Retries up to MAX_RECONNECT_ATTEMPTS per protocol
```

---

## Implementation Plan

### P0 -- Critical (do first)

#### Step 1: Harden config.example.json defaults

**File:** `/Users/abdunabi/Desktop/vpn/server/tunnel/config.example.json`

**What changes:**

1. Set `websocket.enabled` to `true` (was `false`).
2. Set AWG obfuscation parameters to non-zero values:
   - `s1`: 59 (init packet minimum size padding)
   - `s2`: 59 (response packet minimum size padding)
   - `h1`: 925816387 (header obfuscation seed 1)
   - `h2`: 1586498549 (header obfuscation seed 2)
   - `h3`: 1367025694 (header obfuscation seed 3)
   - `h4`: 2013711510 (header obfuscation seed 4)
   - Keep `jc: 5`, `jmin: 50`, `jmax: 1000` as-is (already non-zero).
3. Set `awg.enabled` to `true` (was `false`).

**Rationale for specific values:** The s1/s2 values pad initial handshake packets so they no longer match WireGuard's fixed 148-byte signature. The h1-h4 seeds randomize the header structure. These values must match on client and server -- they are already transmitted via the AWGParams JSONB column in the database, so only the example/template needs updating.

**Impact:** New server deployments will have all three protocols active by default. Existing servers require a database UPDATE to their `awg_params` JSONB column.

**Migration for existing servers (SQL):**
```sql
UPDATE vpn_servers
SET awg_params = '{"jc":5,"jmin":50,"jmax":1000,"s1":59,"s2":59,"h1":925816387,"h2":1586498549,"h3":1367025694,"h4":2013711510}'
WHERE awg_params IS NOT NULL
  AND (awg_params->>'s1')::int = 0;
```

**Dependencies:** None. This is a config-only change.

**Testing:**
- Deploy a test server with new config.
- Connect from a Russian IP using AmneziaWG -- verify handshake is not blocked.
- Packet capture: confirm init packets are no longer 148 bytes.

---

#### Step 2: Geo-aware protocol selection in Go tunnel

**File:** `/Users/abdunabi/Desktop/vpn/client-tunnel/protocol_selector.go`

**What changes:**

1. **Add a new struct `ProtocolPriority`:**
   ```
   type ProtocolPriority struct {
       Protocols []string `json:"protocols"`
       Region    string   `json:"region"`
   }
   ```

2. **Add a function `IsRussianIP(ip string) bool`:**
   - Embed a compact list of Russian IP ranges (RIPE NCC RU allocations). Approximately 30 CIDR supernets cover 95%+ of Russian address space.
   - Use `net.ParseCIDR` and `net.IP.Contains` for matching.
   - This runs on the client device, not the server. It checks the device's outbound IP.
   - The list is a `var russianCIDRs []net.IPNet` initialized in an `init()` function.
   - Place the CIDR data in a new file: `/Users/abdunabi/Desktop/vpn/client-tunnel/geoip_ru.go`

3. **Add a function `DefaultProtocolPriority(clientIP string) []string`:**
   - If `IsRussianIP(clientIP)` returns true: `["vless-ws", "amneziawg", "vless-reality"]`
   - Otherwise: `["vless-reality", "amneziawg", "vless-ws"]`
   - This is the client-side fallback when the server does not provide `protocol_priority`.

4. **Add a function `GetRecommendedProtocol(clientIP string, serverProtocols []string) string`:**
   - Takes the client's public IP and the list of protocols the server supports.
   - Returns the first protocol from `DefaultProtocolPriority(clientIP)` that appears in `serverProtocols`.
   - Exported via gomobile for the React Native app to call.

5. **Modify `ProbeServers` to be protocol-aware:**
   - Change `probeServer` to accept a `protocol` parameter.
   - For `"vless-ws"`: probe via HTTPS GET to the WebSocket host on port 443 (HTTP 101 upgrade or 200 OK both count as success).
   - For `"amneziawg"`: probe via UDP to the AWG endpoint (send a single WG handshake init, expect any response within timeout).
   - For `"vless-reality"`: keep existing TCP probe.
   - Update `ProbeResult.Protocol` to reflect the actual protocol probed.

**New file:** `/Users/abdunabi/Desktop/vpn/client-tunnel/geoip_ru.go`
- Contains `var russianCIDRStrings = []string{...}` with ~30 CIDR supernets.
- `init()` parses them into `[]net.IPNet`.
- Exports `IsRussianIP(ip string) bool`.

**Dependencies:** None. Pure Go, no new external dependencies.

**Testing:**
- Unit test `IsRussianIP` with known Russian IPs (e.g., 5.3.0.1, 77.88.55.1 -- Yandex) and non-Russian IPs.
- Unit test `DefaultProtocolPriority` for both branches.
- Unit test `GetRecommendedProtocol` with various combinations.
- Integration: on a Russian VPS, verify `IsRussianIP` returns true for the device's outbound IP.

---

### P1 -- Important

#### Step 3: Protocol fallback in useVpnConnection.ts

**File:** `/Users/abdunabi/Desktop/vpn/app/src/hooks/useVpnConnection.ts`

**What changes:**

1. **Add new state to track fallback:**
   - `protocolQueue: string[]` -- ordered list of protocols to try.
   - `currentProtocolIndex: number` -- index into the queue.
   - `fallbackStatus: string | null` -- e.g., "Switching to WebSocket..." for UI display.

   These go into a new ref: `protocolFallbackRef = useRef({queue: [], index: 0})`.

2. **New helper `buildProtocolQueue(config: ServerConfig, priority?: string[]): string[]`:**
   - Takes the server config and optional priority hints.
   - Returns an ordered list of protocols the server actually supports.
   - Logic: filter `priority` list to only include protocols present in the config:
     - `"vless-reality"` always available (server always has Reality keys).
     - `"vless-ws"` available if `config.websocket` is present.
     - `"amneziawg"` available if `config.awg` is present.

3. **Modify the auto-reconnect block (lines 107-133):**
   - Current behavior: retry same protocol with exponential backoff.
   - New behavior: after `MAX_RECONNECT_ATTEMPTS` failures on current protocol, advance `currentProtocolIndex` and reset attempt counter.
   - When advancing protocol:
     - Fetch fresh config from API (it may have updated priority hints).
     - Set `connectionState` to a new state value: `'switching_protocol'`.
     - Log which protocol is being tried.
   - When all protocols exhausted: set state to `'error'` with message "All protocols blocked".

4. **Modify the `connect` callback (lines 159-176):**
   - After fetching `ServerConfig`, read `protocol_priority` from the response (Step 4 adds this field).
   - Call `buildProtocolQueue(config, protocolPriority)`.
   - If user's `settingsStore.protocol` is not `'auto'`, move that protocol to front of queue.
   - Store queue in `protocolFallbackRef`.
   - Connect with the first protocol in the queue.

**File:** `/Users/abdunabi/Desktop/vpn/app/src/types/vpn.ts`

**What changes:**
- Add `'switching_protocol'` to `ConnectionState` union type.

**File:** `/Users/abdunabi/Desktop/vpn/app/src/types/api.ts`

**What changes:**
- Add `websocket?` and `awg?` fields to `ServerConfig` interface (they already exist in the Go handler response but are missing from the TS type).
- Add `protocol_priority?: string[]` field to `ServerConfig`.

**Dependencies:** Step 4 (API returns `protocol_priority`). However, the fallback logic works without it -- it will use the client-side default from Step 2's `DefaultProtocolPriority`.

**Testing:**
- Mock a server that returns all three protocol configs.
- Simulate connection failure on first protocol -- verify automatic switch to second.
- Verify `'switching_protocol'` state is emitted and visible in UI.
- Verify manual protocol selection overrides the priority queue.
- Verify manual disconnect cancels all fallback attempts.

---

#### Step 4: Server-side protocol priority hints

**File:** `/Users/abdunabi/Desktop/vpn/server/api/internal/handler/servers.go`

**What changes:**

1. **Add `ProtocolPriority` field to `ServerConfig` struct:**
   ```
   ProtocolPriority []string `json:"protocol_priority,omitempty"`
   ```

2. **Add a function `detectClientRegion(c *fiber.Ctx) string`:**
   - Check `CF-IPCountry` header first (set by Cloudflare, most reliable).
   - Fallback: check `X-Real-IP` or `X-Forwarded-For`, then do a lightweight GeoIP lookup.
   - For the GeoIP lookup, use the existing `oschwald/maxminddb-golang` library (or add it if not present) with the free GeoLite2-Country database.
   - Returns ISO 3166-1 alpha-2 country code (e.g., "RU", "US").
   - **New file:** `/Users/abdunabi/Desktop/vpn/server/api/internal/geoip/geoip.go` -- thin wrapper around MaxMind reader, initialized once at startup.

3. **Modify `GetServerConfig` handler (line 181 area):**
   - After building the `config` struct, call `detectClientRegion(c)`.
   - Set `config.ProtocolPriority` based on region:
     - `"RU"`: `["vless-ws", "amneziawg", "vless-reality"]`
     - `"IR"` (Iran): `["amneziawg", "vless-ws", "vless-reality"]`
     - `"CN"` (China): `["vless-ws", "vless-reality", "amneziawg"]`
     - Default: `["vless-reality", "amneziawg", "vless-ws"]`
   - Only include protocols the server actually supports (filter by what fields are non-nil in the response).

4. **Add `RealityServerNames` field to `ServerConfig`:**
   ```
   RealityServerNames []string `json:"reality_server_names,omitempty"`
   ```
   - Populated from a new DB column or from a config table.
   - Allows the client to rotate server_name on reconnect attempts (different fingerprint each time).

**File:** `/Users/abdunabi/Desktop/vpn/server/api/internal/model/server.go`

**What changes:**
- No schema change needed for protocol_priority -- it is computed at request time, not stored.

**New file:** `/Users/abdunabi/Desktop/vpn/server/api/internal/geoip/geoip.go`
- `type Reader struct { db *maxminddb.Reader }`
- `func New(dbPath string) (*Reader, error)`
- `func (r *Reader) Country(ip string) string`
- `func (r *Reader) Close() error`

**New dependency:** `github.com/oschwald/maxminddb-golang` (only if not already present). Justified because: (a) GeoIP is a core requirement for censorship evasion, (b) MaxMind GeoLite2 is the industry standard free GeoIP database, (c) the Go library is zero-dependency and read-only (no network calls).

**Infrastructure requirement:** Download GeoLite2-Country.mmdb and place it on the API server. Add to deployment scripts. Auto-update weekly via MaxMind's download API.

**Dependencies:** None for the basic version (hardcoded country-to-priority map). GeoIP database download is an ops task.

**Testing:**
- Unit test `detectClientRegion` with mock `CF-IPCountry` header.
- Unit test priority generation for RU, IR, CN, and default regions.
- Integration: call `GET /servers/:id/config` from a Russian IP and verify `protocol_priority` starts with `"vless-ws"`.
- Verify the priority list only includes protocols the server supports.

---

### P2 -- Nice to have

#### Step 5: Multi-server deployment strategy (documentation only)

**File:** `/Users/abdunabi/Desktop/vpn/docs/deployment-anti-censorship.md` (new)

**Content to document:**

```
Architecture: Protocol Isolation per IP

Server A (IP: a.b.c.d)          Server B (IP: e.f.g.h)
+------------------+            +------------------+
| VLESS+REALITY    |            | AmneziaWG        |
| Port 443         |            | Port 51820 (UDP) |
+------------------+            +------------------+

Server C (no direct IP exposure)
+------------------+
| VLESS+WebSocket  |
| Behind Cloudflare|
| CDN domain       |
+------------------+
```

**Key points to document:**
- Why separate IPs: if TSPU blocks one IP, only one protocol is affected.
- Server C has no publicly known IP -- Cloudflare proxies all traffic.
- Database stores all three as separate `vpn_servers` rows but they can share the same physical machine (different IPs via secondary interfaces or VMs).
- The API's `ListServers` response groups them by location; the client sees "Netherlands" with three available protocols.
- Deployment automation: Ansible/Terraform templates for spinning up protocol-specific servers.
- IP rotation strategy: when an IP is blocked, provision a new one and update DNS/DB.

**Dependencies:** None.

---

#### Step 6: Multiple CDN domain support and Cloudflare Workers

**Files:**
- `/Users/abdunabi/Desktop/vpn/server/api/internal/model/server.go` -- schema change
- `/Users/abdunabi/Desktop/vpn/server/api/internal/handler/servers.go` -- response change
- `/Users/abdunabi/Desktop/vpn/server/tunnel/nginx-ws.conf.example` -- multi-domain config

**What changes:**

1. **Database migration (007_add_ws_domains.sql):**
   - Add `ws_domains TEXT[]` column to `vpn_servers` (PostgreSQL array).
   - Migrate existing `ws_host` value into `ws_domains[0]`.
   - Keep `ws_host` as primary, `ws_domains` as rotation pool.

2. **Model change in `server.go`:**
   - Add `WSDomains pq.StringArray` field with `gorm:"column:ws_domains;type:text[]"`.

3. **Handler change in `servers.go`:**
   - `WebSocketClientConfig` gets a new field: `AlternateHosts []string json:"alternate_hosts,omitempty"`.
   - Populated from `server.WSDomains` minus the primary `server.WSHost`.
   - Client can rotate domains on reconnect if the primary is blocked.

4. **Cloudflare Workers concept (document only):**
   - A Worker at `worker.example.com` receives HTTPS requests and forwards them to the real WebSocket backend.
   - The Worker domain looks like an ordinary SaaS app to TSPU.
   - Document the Worker script template and deployment steps.
   - This is a documentation deliverable, not code.

5. **Nginx multi-domain config:**
   - Update `nginx-ws.conf.example` to show `server_name` with multiple domains.
   - Add a comment block explaining the Cloudflare Workers alternative.

**Dependencies:** Step 1 (WebSocket enabled by default).

**Testing:**
- Verify migration runs cleanly on existing database.
- Verify API returns `alternate_hosts` when multiple domains are configured.
- Manual test: block primary domain in local DNS, verify client falls back to alternate.

---

### P3 -- Quick win

#### Step 7: Russian server_names for REALITY

**File:** `/Users/abdunabi/Desktop/vpn/server/tunnel/config.example.json`

**What changes:**
- Add Russian popular sites to `reality.server_names`:
  ```
  "server_names": [
      "www.microsoft.com",
      "microsoft.com",
      "yandex.ru",
      "www.yandex.ru",
      "mail.ru",
      "www.mail.ru",
      "vk.com",
      "www.vk.com",
      "ok.ru",
      "www.ok.ru",
      "sberbank.ru",
      "www.sberbank.ru",
      "gosuslugi.ru",
      "www.gosuslugi.ru"
  ]
  ```
- Add corresponding `dest` rotation: `"dest": "yandex.ru:443"` (or keep microsoft.com -- the dest must support TLS 1.3 and HTTP/2).

**File:** `/Users/abdunabi/Desktop/vpn/server/api/internal/handler/servers.go`

**What changes:**
- In `GetServerConfig`, line 189: instead of hardcoding `"www.microsoft.com"`, select a random `server_name` from a list.
- Add a `var realityServerNames = []string{...}` in the handler file.
- Use `math/rand` to pick one per request, ensuring each client session uses a different SNI.
- Better yet: store server_names in the database (new column or in a JSON config column) and randomize from there.

**File:** `/Users/abdunabi/Desktop/vpn/client-tunnel/config.go`

**What changes:**
- In `buildClientXRayConfig`, the `serverName` variable (line 63) already reads from `config.Reality.ServerName`. No change needed -- the server already sends the randomized name.

**Dependencies:** None.

**Testing:**
- Verify each `GET /servers/:id/config` call returns a different `server_name`.
- Verify the server_name domains actually support TLS 1.3 (test with `openssl s_client`).
- From Russia: verify REALITY handshake using `yandex.ru` SNI is not flagged.

---

## Dependency Graph

```
Step 1 (config defaults) -----> Step 6 (multi-CDN domains)
                          \
Step 2 (geo-aware Go)      \
         |                  +--> Step 5 (deployment docs)
         v
Step 3 (client fallback)
         |
         v
Step 4 (API priority) -------> Step 3 (uses protocol_priority)

Step 7 (server_names) -------> independent
```

**Recommended execution order:**
1. Step 1 + Step 7 (both are config changes, can be done in parallel)
2. Step 2 (Go code, no server-side dependency)
3. Step 4 (API change, needs GeoIP setup)
4. Step 3 (client code, consumes Step 4's output)
5. Step 5 + Step 6 (lower priority, can be done in parallel)

---

## Consequences

### Positive
- Three independent protocol paths means TSPU must block all three simultaneously to fully deny service.
- WebSocket through Cloudflare CDN is the hardest to block -- Russia would need to block Cloudflare entirely.
- Geo-aware selection means Russian users automatically get the most censorship-resistant protocol without manual configuration.
- Protocol fallback means temporary blocks cause a brief interruption (seconds) rather than a full outage.

### Negative
- WebSocket transport adds ~15-20% latency overhead compared to direct REALITY (extra hop through Cloudflare).
- AmneziaWG with obfuscation adds ~5-10% bandwidth overhead from junk packets.
- GeoIP database requires periodic updates (weekly cron job).
- Multiple CDN domains increase operational complexity and cost.
- The Russian IP range list in the client binary is static and needs periodic updates via app releases.

### Risks
- **TSPU evolution:** Russia could start blocking Cloudflare CDN ranges. Mitigation: Cloudflare Workers on custom domains, domain fronting.
- **IP range staleness:** The embedded Russian CIDR list may miss new allocations. Mitigation: the server-side GeoIP (Step 4) is the primary mechanism; client-side (Step 2) is a fallback only.
- **Server_name blocking:** TSPU could block REALITY connections to specific SNIs. Mitigation: randomization across many domains (Step 7) and the fact that blocking yandex.ru/vk.com would break legitimate Russian internet traffic.

## Alternatives Considered

1. **ShadowTLS v3 instead of WebSocket CDN:** Better performance but less battle-tested. Cloudflare CDN is nearly impossible to block wholesale. ShadowTLS could be added later as a fourth protocol option.

2. **Domain fronting instead of Cloudflare Workers:** Most CDNs have disabled domain fronting. Cloudflare Workers achieve a similar effect through a supported mechanism.

3. **Client-side GeoIP database (MaxMind) instead of embedded IP ranges:** Would add ~5MB to the app binary. The embedded CIDR list is ~2KB. Server-side GeoIP handles the accurate case; client-side only needs to be "good enough" for the initial connection attempt.

4. **QUIC/HTTP3 transport:** xray-core supports QUIC but it is easily identified by TSPU due to the distinctive UDP pattern. AmneziaWG is a better choice for UDP-based obfuscation.

5. **Pluggable transports (obfs4, meek):** Battle-tested in Tor ecosystem but would require significant integration work with xray-core. The three current protocols cover the main evasion vectors (TLS mimicry, CDN tunneling, WireGuard obfuscation) without introducing a new dependency.
