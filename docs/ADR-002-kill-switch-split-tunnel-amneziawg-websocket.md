# ADR-002: Kill Switch, Split Tunneling, AmneziaWG, and WebSocket CDN Transport

**Status:** Proposed
**Date:** 2026-04-02
**Authors:** Architecture Review

---

## Context

The VPN app currently supports a single protocol path:

```
React Native  -->  Native Module (iOS/Android)  -->  Go tunnel (gomobile)
                                                        |
                                                  xray-core (VLESS+REALITY)
                                                        |
                                                  tun2socks (TUN fd <-> SOCKS5)
```

Four features are requested:
1. **Kill Switch** -- block all internet when VPN drops unexpectedly
2. **Split Tunneling** -- exclude specific apps from the VPN tunnel
3. **AmneziaWG** -- a second protocol path using modified WireGuard with DPI obfuscation
4. **WebSocket (CDN) Transport** -- route VLESS traffic through Cloudflare CDN via WebSocket

These features have different dependency relationships and can be partially parallelized.

---

## Dependency Graph

```
                    +-------------------+
                    |  Server Config    |
                    |  Refactoring      |
                    +--------+----------+
                             |
              +--------------+--------------+
              |                             |
   +----------v----------+      +-----------v-----------+
   | AmneziaWG Protocol  |      | WebSocket Transport   |
   | (server + client)   |      | (server + client)     |
   +---------------------+      +-----------------------+

   +---------------------+      +-----------------------+
   | Kill Switch          |      | Split Tunneling       |
   | (pure client-side)   |      | (pure client-side)    |
   +---------------------+      +-----------------------+
```

- Kill Switch and Split Tunneling are **independent** of each other and of the protocol features
- AmneziaWG and WebSocket both require server-side config changes, but are independent of each other
- The `protocol_selector.go` / `ConnectConfig` / settings store need updates for both new protocols

---

## Implementation Batches

### Batch 1 (parallel, no dependencies)
- **Kill Switch** (Android + iOS + frontend wiring)
- **Split Tunneling** (Android + iOS + frontend wiring)

### Batch 2 (parallel, no dependencies between them)
- **AmneziaWG protocol** (server + client Go tunnel + frontend)
- **WebSocket CDN transport** (server + client Go tunnel + frontend)

### Batch 3 (after Batch 2)
- **Protocol Selector** enhancement -- make `auto` mode probe all available protocols
- Integration testing across all protocol paths

---

# Feature 1: Kill Switch

## Decision

Use OS-level VPN APIs to block traffic when the tunnel drops but the VPN service is still alive.

**Android:** Use `VpnService.Builder` with `addDisallowedApplication()` removed and ensure the TUN routes remain active. When the tunnel process (xray) dies but the VpnService is still running, keep the TUN interface open with routes in place -- traffic goes to the TUN but nothing reads it, effectively blocking all traffic. Add an `always_on_vpn` flag to rebuild the TUN without a working tunnel.

**iOS:** Use `NEVPNProtocol.includeAllNetworks = true` and `excludeLocalNetworks = true`. These are first-party Apple APIs (iOS 14+) that enforce system-wide kill switch behavior.

## Alternatives Considered

1. **Firewall rules (iptables on Android)** -- Requires root. Rejected.
2. **Dummy TUN with no backend** -- This is effectively what we do on Android by keeping the TUN interface alive. Acceptable.
3. **NEOnDemandRule on iOS** -- Insufficient; only handles "reconnect" scenarios, not "block traffic if tunnel fails." `includeAllNetworks` is the correct API.

## Tradeoffs

- `includeAllNetworks` on iOS may block some local network features (AirDrop, AirPlay). Mitigated by `excludeLocalNetworks = true`.
- On Android, the "block by keeping TUN alive" approach means there is a brief window during tunnel restart where traffic could leak. This is acceptable for non-enterprise use.

## Files to Modify

### Android

**`/Users/abdunabi/Desktop/vpn/app/android/app/src/main/java/com/vpnapp/vpn/TunnelVpnService.kt`**

Add a `killSwitchEnabled` field and modify the `stopVpn()` method:

```
Current stopVpn():
  1. StopTun
  2. Disconnect xray
  3. Close TUN interface
  4. stopSelf()

New stopVpn(fromUser: Boolean):
  If killSwitchEnabled AND NOT fromUser:
    1. StopTun
    2. Disconnect xray
    3. DO NOT close TUN interface -- keep routes active, traffic goes nowhere
    4. DO NOT stopSelf() -- service stays alive
    5. Update notification: "VPN disconnected -- Kill Switch active"
    6. Emit state: "kill_switch_active"
  Else:
    (existing behavior)
```

Add a `setKillSwitch(enabled: Boolean)` method:

```kotlin
fun setKillSwitch(enabled: Boolean) {
    killSwitchEnabled = enabled
    // If currently connected, rebuild TUN with/without the flag
    // (no immediate effect until next disconnect event)
}
```

Modify `onStatusChanged` to detect tunnel errors and trigger kill switch:

```kotlin
override fun onStatusChanged(statusJSON: String?) {
    // ... existing code ...
    when (state) {
        "error" -> {
            if (killSwitchEnabled && isRunning) {
                // Tunnel errored but service is alive -- activate kill switch
                activateKillSwitch()
            }
        }
    }
}

private fun activateKillSwitch() {
    // Stop tun2socks and xray but keep TUN interface alive
    Tunnel.stopTun()
    Tunnel.disconnect()
    // TUN interface stays open -> all traffic is blackholed
    updateNotification("Kill Switch Active - No Internet")
    VpnTurboModule.sendStatusEvent("""{"state":"kill_switch_active",...}""")
}
```

**`/Users/abdunabi/Desktop/vpn/app/android/app/src/main/java/com/vpnapp/vpn/VpnTurboModule.kt`**

Add `setKillSwitch` ReactMethod:

```kotlin
@ReactMethod
fun setKillSwitch(enabled: Boolean, promise: Promise) {
    TunnelVpnService.instance?.setKillSwitch(enabled)
    promise.resolve("")
}
```

### iOS

**`/Users/abdunabi/Desktop/vpn/app/ios/VpnApp/VpnManager.swift`**

Modify `setupManager()` to apply kill switch settings to the NETunnelProviderProtocol:

```swift
private func setupManager() {
    let proto = NETunnelProviderProtocol()
    proto.providerBundleIdentifier = "com.vpnapp.VpnAppNetworkExtension"
    proto.serverAddress = "VPN App"
    proto.disconnectOnSleep = false

    // Kill Switch: NEW
    if killSwitchEnabled {
        if #available(iOS 14.0, *) {
            proto.includeAllNetworks = true
            proto.excludeLocalNetworks = true
        }
    }

    manager?.protocolConfiguration = proto
    manager?.localizedDescription = "VPN App"
    manager?.isEnabled = true
}
```

Add methods:

```swift
var killSwitchEnabled: Bool = false

func setKillSwitch(enabled: Bool, completion: @escaping (Error?) -> Void) {
    killSwitchEnabled = enabled
    guard let manager = manager else {
        completion(nil)
        return
    }

    // Re-apply protocol configuration with new setting
    setupManager()
    manager.saveToPreferences { error in
        completion(error)
    }
}
```

**`/Users/abdunabi/Desktop/vpn/app/ios/VpnApp/VpnModuleImpl.swift`**

Add method:

```swift
@objc func setKillSwitch(_ enabled: Bool,
                          resolve: @escaping RCTPromiseResolveBlock,
                          reject: @escaping RCTPromiseRejectBlock) {
    VpnManager.shared.setKillSwitch(enabled: enabled) { error in
        if let error = error {
            reject("KILLSWITCH_ERROR", error.localizedDescription, error)
        } else {
            resolve("")
        }
    }
}
```

### Frontend

**`/Users/abdunabi/Desktop/vpn/app/src/types/vpn.ts`**

Add `'kill_switch_active'` to `ConnectionState` union type.

**`/Users/abdunabi/Desktop/vpn/app/src/types/native.ts`**

Already has `setKillSwitch` in the interface. No changes needed.

**`/Users/abdunabi/Desktop/vpn/app/src/services/vpnBridge.ts`**

Already has `setKillSwitch`. No changes needed.

**`/Users/abdunabi/Desktop/vpn/app/src/stores/settingsStore.ts`**

Already calls `vpnBridge.setKillSwitch`. No changes needed.

### Complexity: Low-Medium
- Android: ~80 lines of new/modified code
- iOS: ~40 lines of new/modified code
- Frontend: ~5 lines (type change only)

---

# Feature 2: Split Tunneling

## Decision

**Android:** Use `VpnService.Builder.addDisallowedApplication(packageName)` to exclude selected apps from the VPN tunnel. This is a well-supported Android API that works at the OS level.

**iOS:** iOS does NOT support per-app VPN for consumer apps (only MDM-managed devices can use per-app VPN). Instead, implement **domain-based split tunneling** using `NEPacketTunnelNetworkSettings` excluded routes or by modifying xray-core routing rules to send certain domains via the `direct` (non-VPN) outbound.

## Alternatives Considered

1. **Per-app VPN on iOS via MDM profiles** -- Requires enterprise MDM. Not feasible for a consumer app.
2. **Route-based split tunneling (IP ranges)** -- Complex for users. Could be a future extension.
3. **Domain-based on both platforms** -- Consistent UX but Android has much better per-app support. Decision: per-app on Android, domain-based on iOS with a clear UI distinction.

## Tradeoffs

- iOS cannot do per-app split tunneling. Users will see a different UI on iOS (domain-based exclusions) vs Android (app-based exclusions).
- Android `addDisallowedApplication` requires rebuilding the TUN interface when the list changes while connected. This means a brief reconnection.

## Files to Modify / Create

### Android

**`/Users/abdunabi/Desktop/vpn/app/android/app/src/main/java/com/vpnapp/vpn/TunnelVpnService.kt`**

Add an excluded apps list and modify `startVpn()`:

```kotlin
private var excludedApps: List<String> = emptyList()

fun setExcludedApps(packageNames: List<String>) {
    excludedApps = packageNames
    // If connected, need to rebuild TUN interface
    if (isRunning) {
        rebuildTunInterface()
    }
}

private fun rebuildTunInterface() {
    // 1. Stop tun2socks
    Tunnel.stopTun()
    // 2. Close old TUN
    vpnInterface?.close()
    // 3. Create new TUN with updated excluded apps
    vpnInterface = buildTunInterface()
    // 4. Restart tun2socks with new fd
    vpnInterface?.let {
        Tunnel.startTun(it.fd.toLong())
    }
}
```

Modify the TUN builder section in `startVpn()`:

```kotlin
private fun buildTunInterface(): ParcelFileDescriptor? {
    val builder = Builder()
        .setSession("VPN App")
        .addAddress("10.0.0.2", 32)
        .addRoute("0.0.0.0", 0)
        .addAddress("fd00::2", 128)
        .addRoute("::", 0)
        .addDnsServer("1.1.1.1")
        .addDnsServer("8.8.8.8")
        .setMtu(1500)
        .setBlocking(false)

    // Split tunneling: exclude selected apps
    for (pkg in excludedApps) {
        try {
            builder.addDisallowedApplication(pkg)
        } catch (e: Exception) {
            Log.w(TAG, "Could not exclude app: $pkg", e)
        }
    }

    return builder.establish()
}
```

**`/Users/abdunabi/Desktop/vpn/app/android/app/src/main/java/com/vpnapp/vpn/VpnTurboModule.kt`**

Add new methods:

```kotlin
@ReactMethod
fun setExcludedApps(packageNamesJSON: String, promise: Promise) {
    try {
        val names = JSONArray(packageNamesJSON)
        val list = (0 until names.length()).map { names.getString(it) }
        TunnelVpnService.instance?.setExcludedApps(list)
        promise.resolve("")
    } catch (e: Exception) {
        promise.reject("SPLIT_TUNNEL_ERROR", e.message, e)
    }
}

@ReactMethod
fun getInstalledApps(promise: Promise) {
    Thread {
        try {
            val pm = reactApplicationContext.packageManager
            val apps = pm.getInstalledApplications(0)
                .filter { pm.getLaunchIntentForPackage(it.packageName) != null }
                .map { app ->
                    JSONObject().apply {
                        put("package_name", app.packageName)
                        put("app_name", pm.getApplicationLabel(app).toString())
                    }
                }
            promise.resolve(JSONArray(apps).toString())
        } catch (e: Exception) {
            promise.reject("APP_LIST_ERROR", e.message, e)
        }
    }.start()
}
```

### iOS

**`/Users/abdunabi/Desktop/vpn/app/ios/VpnAppNetworkExtension/PacketTunnelProvider.swift`**

For iOS, implement domain-based split tunneling by modifying the xray-core routing config:

The approach: pass excluded domains in the tunnel options, and add them as "direct" routing rules in the xray config built by the Go tunnel.

This means the Go tunnel's `ConnectConfig` needs a new field:

```go
type ConnectConfig struct {
    // ... existing fields ...
    ExcludedDomains []string `json:"excluded_domains,omitempty"`
}
```

And `buildClientXRayConfig()` adds routing rules for those domains to use the "direct" outbound.

**`/Users/abdunabi/Desktop/vpn/app/ios/VpnApp/VpnModuleImpl.swift`**

Add methods (iOS provides domain-based exclusions, not app-based):

```swift
@objc func setExcludedDomains(_ domainsJSON: String,
                               resolve: @escaping RCTPromiseResolveBlock,
                               reject: @escaping RCTPromiseRejectBlock) {
    // Store in UserDefaults for the tunnel extension to read
    let defaults = UserDefaults(suiteName: "group.com.vpnapp.shared")
    defaults?.set(domainsJSON, forKey: "excluded_domains")
    resolve("")
}
```

### Go Tunnel

**`/Users/abdunabi/Desktop/vpn/client-tunnel/config.go`**

Modify `ConnectConfig` and `buildClientXRayConfig()`:

```go
type ConnectConfig struct {
    ServerAddress   string               `json:"server_address"`
    ServerPort      int                  `json:"server_port"`
    Protocol        string               `json:"protocol"`
    UserID          string               `json:"user_id"`
    Reality         *RealityClientConfig `json:"reality,omitempty"`
    ExcludedDomains []string             `json:"excluded_domains,omitempty"` // NEW
}
```

In `buildClientXRayConfig()`, modify the routing section:

```go
rules := []map[string]interface{}{
    {
        "type":        "field",
        "inboundTag":  []string{"socks-in"},
        "outboundTag": "vless-out",
    },
}

// Split tunneling: excluded domains go direct
if len(config.ExcludedDomains) > 0 {
    // Insert BEFORE the catch-all rule
    rules = append([]map[string]interface{}{
        {
            "type":        "field",
            "domain":      config.ExcludedDomains,
            "outboundTag": "direct",
        },
    }, rules...)
}
```

### Frontend

**NEW FILE: `/Users/abdunabi/Desktop/vpn/app/src/screens/SplitTunnelScreen.tsx`**

A new screen showing:
- On Android: a list of installed apps with checkboxes to exclude
- On iOS: a text input / list to add domains to exclude
- Use `Platform.OS` to show the correct UI

**`/Users/abdunabi/Desktop/vpn/app/src/types/native.ts`**

Add to interface:

```typescript
export interface VpnNativeModule {
    // ... existing ...
    setExcludedApps(packageNamesJSON: string): Promise<void>;       // Android only
    getInstalledApps(): Promise<string>;                              // Android only
    setExcludedDomains(domainsJSON: string): Promise<void>;          // iOS only
}
```

**`/Users/abdunabi/Desktop/vpn/app/src/services/vpnBridge.ts`**

Add bridge functions for split tunneling.

**`/Users/abdunabi/Desktop/vpn/app/src/stores/settingsStore.ts`**

Add `excludedApps: string[]` and `excludedDomains: string[]` to the store. Persist to AsyncStorage.

### Complexity: Medium
- Android: ~120 lines new/modified
- iOS: ~30 lines (domain-based is simpler)
- Go tunnel: ~20 lines (routing rules)
- Frontend: ~200 lines (new screen, store updates)

---

# Feature 3: AmneziaWG Protocol

## Decision

Use `github.com/amnezia-vpn/amneziawg-go` as a Go library (not CLI). AmneziaWG is a direct replacement for WireGuard's kernel module, adding DPI obfuscation via junk packets, message padding, and custom headers.

This is a **completely separate tunnel path** from xray-core. When AmneziaWG is selected:
- xray-core does NOT run
- tun2socks does NOT run
- AmneziaWG creates its own virtual interface and handles packet encryption/routing directly

The architecture becomes:

```
Protocol: vless-reality / websocket          Protocol: amneziawg
+-----------------------------+              +-----------------------------+
| xray-core (SOCKS5 proxy)   |              | amneziawg-go (device)       |
|     |                       |              |     |                       |
| tun2socks (TUN<->SOCKS5)   |              | TUN fd (direct, no SOCKS5) |
+-----------------------------+              +-----------------------------+
```

## Alternatives Considered

1. **Wrap AmneziaWG in a SOCKS5 proxy and reuse tun2socks** -- Adds unnecessary overhead. AmneziaWG already handles TUN directly. Rejected.
2. **Use AmneziaWG as a CLI subprocess** -- gomobile cannot spawn subprocesses on iOS. Must use as a library. Rejected.
3. **Use standard WireGuard-Go** -- Does not have DPI obfuscation. The whole point is anti-censorship. Rejected.

## Tradeoffs

- Adding amneziawg-go increases the gomobile binary size (~3-5MB)
- AmneziaWG uses UDP (WireGuard protocol), which may be blocked on some networks. This is where VLESS+REALITY (TCP-based) or WebSocket (CDN-routable) complement it.
- Server needs to run a separate AmneziaWG daemon alongside xray-core
- Key management for AmneziaWG is different (X25519 keypairs, not UUIDs)

## Architecture

```
                        ConnectConfig.Protocol
                               |
                   +-----------+-----------+
                   |                       |
             "vless-reality"          "amneziawg"
             "websocket"
                   |                       |
            +------v------+        +-------v-------+
            | runTunnel()  |        | runAWGTunnel()|
            | xray-core    |        | amneziawg-go  |
            | + tun2socks  |        | device.NewDev |
            +--------------+        | + UAPI config |
                                    +---------------+
```

## Files to Modify / Create

### Go Client Tunnel

**`/Users/abdunabi/Desktop/vpn/client-tunnel/go.mod`**

Add dependency:

```
require (
    github.com/amnezia-vpn/amneziawg-go v0.2.12  // or latest
    // ... existing
)
```

**`/Users/abdunabi/Desktop/vpn/client-tunnel/config.go`**

Extend `ConnectConfig`:

```go
type ConnectConfig struct {
    ServerAddress   string               `json:"server_address"`
    ServerPort      int                  `json:"server_port"`
    Protocol        string               `json:"protocol"`         // "vless-reality", "amneziawg", "websocket"
    UserID          string               `json:"user_id"`          // For VLESS protocols
    Reality         *RealityClientConfig `json:"reality,omitempty"`
    AmneziaWG       *AWGClientConfig     `json:"amneziawg,omitempty"` // NEW
    WebSocket       *WSClientConfig      `json:"websocket,omitempty"` // NEW (Feature 4)
    ExcludedDomains []string             `json:"excluded_domains,omitempty"`
}

// AWGClientConfig holds client-side AmneziaWG settings.
type AWGClientConfig struct {
    PrivateKey string `json:"private_key"`    // Client X25519 private key (base64)
    PublicKey  string `json:"public_key"`     // Server X25519 public key (base64)
    Endpoint   string `json:"endpoint"`       // Server address:port (UDP)
    AllowedIPs string `json:"allowed_ips"`    // Typically "0.0.0.0/0, ::/0"
    DNS        string `json:"dns"`            // DNS servers
    PresharedKey string `json:"preshared_key,omitempty"`

    // AmneziaWG obfuscation parameters
    Jc   int `json:"jc"`    // Junk packet count (before handshake)
    Jmin int `json:"jmin"`  // Min junk packet size
    Jmax int `json:"jmax"`  // Max junk packet size
    S1   int `json:"s1"`    // Init packet padding
    S2   int `json:"s2"`    // Response packet padding
    H1   int `json:"h1"`    // Init header modifier
    H2   int `json:"h2"`    // Response header modifier
    H3   int `json:"h3"`    // Cookie header modifier
    H4   int `json:"h4"`    // Transport header modifier
}
```

**NEW FILE: `/Users/abdunabi/Desktop/vpn/client-tunnel/awg.go`**

This is the core of the AmneziaWG client integration:

```go
package tunnel

import (
    "fmt"
    "strings"
    "sync"

    "github.com/amnezia-vpn/amneziawg-go/conn"
    "github.com/amnezia-vpn/amneziawg-go/device"
    "github.com/amnezia-vpn/amneziawg-go/tun"
)

var (
    awgMu     sync.Mutex
    awgDevice *device.Device
)

// startAWGTunnel creates an AmneziaWG device, configures it via UAPI,
// and starts the tunnel. The TUN fd is provided by the OS VPN service.
//
// Unlike the xray path, AmneziaWG does NOT use SOCKS5 or tun2socks.
// It directly reads/writes packets on the TUN fd.
func startAWGTunnel(fd int, config AWGClientConfig) error {
    awgMu.Lock()
    defer awgMu.Unlock()

    if awgDevice != nil {
        return fmt.Errorf("amneziawg already running")
    }

    // Create TUN device from file descriptor
    tunDev, err := tun.CreateTUNFromFile(fd)  // platform-specific
    if err != nil {
        return fmt.Errorf("creating tun device: %w", err)
    }

    // Create logger
    logger := device.NewLogger(device.LogLevelVerbose, "(awg) ")

    // Create the AmneziaWG device
    dev := device.NewDevice(tunDev, conn.NewDefaultBind(), logger)

    // Build UAPI config string
    uapiConfig := buildAWGUAPIConfig(config)

    // Apply configuration via IPC
    if err := dev.IpcSet(uapiConfig); err != nil {
        dev.Close()
        return fmt.Errorf("configuring amneziawg: %w", err)
    }

    // Bring device up
    dev.Up()

    awgDevice = dev
    return nil
}

// stopAWGTunnel stops the AmneziaWG device.
func stopAWGTunnel() {
    awgMu.Lock()
    defer awgMu.Unlock()

    if awgDevice != nil {
        awgDevice.Close()
        awgDevice = nil
    }
}

// buildAWGUAPIConfig creates a UAPI configuration string for AmneziaWG.
// Format: key=value pairs separated by newlines.
func buildAWGUAPIConfig(config AWGClientConfig) string {
    var b strings.Builder

    b.WriteString(fmt.Sprintf("private_key=%s\n", hexKey(config.PrivateKey)))

    // Obfuscation parameters (AmneziaWG extensions)
    b.WriteString(fmt.Sprintf("jc=%d\n", config.Jc))
    b.WriteString(fmt.Sprintf("jmin=%d\n", config.Jmin))
    b.WriteString(fmt.Sprintf("jmax=%d\n", config.Jmax))
    b.WriteString(fmt.Sprintf("s1=%d\n", config.S1))
    b.WriteString(fmt.Sprintf("s2=%d\n", config.S2))
    b.WriteString(fmt.Sprintf("h1=%d\n", config.H1))
    b.WriteString(fmt.Sprintf("h2=%d\n", config.H2))
    b.WriteString(fmt.Sprintf("h3=%d\n", config.H3))
    b.WriteString(fmt.Sprintf("h4=%d\n", config.H4))

    // Peer configuration
    b.WriteString(fmt.Sprintf("public_key=%s\n", hexKey(config.PublicKey)))
    if config.PresharedKey != "" {
        b.WriteString(fmt.Sprintf("preshared_key=%s\n", hexKey(config.PresharedKey)))
    }
    b.WriteString(fmt.Sprintf("endpoint=%s\n", config.Endpoint))
    b.WriteString(fmt.Sprintf("persistent_keepalive_interval=25\n"))

    // Allowed IPs
    for _, ip := range strings.Split(config.AllowedIPs, ",") {
        ip = strings.TrimSpace(ip)
        if ip != "" {
            b.WriteString(fmt.Sprintf("allowed_ip=%s\n", ip))
        }
    }

    return b.String()
}
```

**`/Users/abdunabi/Desktop/vpn/client-tunnel/tunnel.go`**

Major changes to `Connect()` and `runTunnel()` to dispatch based on protocol:

```go
// In Connect(), after parsing config:
// Validate based on protocol
switch config.Protocol {
case "vless-reality", "websocket":
    // Existing validation for VLESS-based protocols
    if config.UserID == "" {
        // ... existing error handling
    }
case "amneziawg":
    if config.AmneziaWG == nil {
        return "amneziawg config is required"
    }
    if config.AmneziaWG.PrivateKey == "" || config.AmneziaWG.PublicKey == "" {
        return "amneziawg keys are required"
    }
default:
    return fmt.Sprintf("unsupported protocol: %s", config.Protocol)
}
```

Modify `runTunnel()`:

```go
func (m *tunnelManager) runTunnel(config ConnectConfig, readyCh chan<- string) {
    switch config.Protocol {
    case "vless-reality":
        m.runXRayTunnel(config, readyCh)    // Existing code, renamed
    case "websocket":
        m.runXRayTunnel(config, readyCh)    // Same xray path, different config builder
    case "amneziawg":
        m.runAWGTunnel(config, readyCh)     // NEW path
    }
}
```

Rename existing `runTunnel` logic to `runXRayTunnel` (no functional change, just rename).

Add `runAWGTunnel`:

```go
func (m *tunnelManager) runAWGTunnel(config ConnectConfig, readyCh chan<- string) {
    serverAddr := config.AmneziaWG.Endpoint
    protocol := "amneziawg"

    m.mu.Lock()
    m.setStatus(TunnelStatus{
        State:      StateConnecting,
        ServerAddr: serverAddr,
        Protocol:   protocol,
    })
    m.mu.Unlock()

    // AmneziaWG doesn't need xray or tun2socks.
    // It will be started AFTER the native layer provides the TUN fd.
    // Signal readiness -- the native layer will call StartTun() next.
    //
    // But wait -- AmneziaWG needs the TUN fd to start. The current flow is:
    //   Connect() -> creates xray -> native creates TUN -> StartTun(fd)
    //
    // For AmneziaWG, we need to defer actual tunnel creation to StartTun().
    // So here we just validate config and store it for StartTun to use.

    m.mu.Lock()
    m.awgConfig = config.AmneziaWG  // Store for StartTun to pick up
    m.connectedAt = time.Now()
    m.setStatus(TunnelStatus{
        State:       StateConnected,
        ServerAddr:  serverAddr,
        Protocol:    protocol,
        ConnectedAt: m.connectedAt.Unix(),
    })
    stopCh := m.stopCh
    m.mu.Unlock()

    readyCh <- ""  // Signal: ready for TUN fd

    <-stopCh
}
```

**`/Users/abdunabi/Desktop/vpn/client-tunnel/tun.go`**

Modify `StartTun()` to branch on protocol:

```go
func StartTun(fd int) string {
    tunMu.Lock()
    defer tunMu.Unlock()

    if tunRunning {
        return "tun already running"
    }

    mgr := getManager()
    mgr.mu.Lock()
    awgConfig := mgr.awgConfig
    protocol := mgr.status.Protocol
    mgr.mu.Unlock()

    if protocol == "amneziawg" && awgConfig != nil {
        // AmneziaWG path: device reads/writes TUN directly
        if err := startAWGTunnel(fd, *awgConfig); err != nil {
            return err.Error()
        }
    } else {
        // xray path: tun2socks bridges TUN <-> SOCKS5
        key := new(engine.Key)
        key.Proxy = socksProxyURL()
        key.Device = fmt.Sprintf("fd://%d", fd)
        key.LogLevel = "warn"
        key.MTU = 1500
        engine.Insert(key)
        engine.Start()
    }

    tunRunning = true
    return ""
}

func StopTun() string {
    tunMu.Lock()
    defer tunMu.Unlock()

    if !tunRunning {
        return ""
    }

    // Stop both -- only the active one will do anything
    stopAWGTunnel()
    engine.Stop()

    tunRunning = false
    return ""
}
```

**`/Users/abdunabi/Desktop/vpn/client-tunnel/tunnel.go`** (tunnelManager struct)

Add `awgConfig` field:

```go
type tunnelManager struct {
    mu           sync.Mutex
    status       TunnelStatus
    callback     StatusCallback
    xrayInstance *core.Instance
    awgConfig    *AWGClientConfig   // NEW: stored between Connect() and StartTun()
    stats        *TrafficStats
    stopCh       chan struct{}
    connectedAt  time.Time
}
```

In `Disconnect()`, also clear `awgConfig`:

```go
mgr.awgConfig = nil
stopAWGTunnel()
```

### Server Side

**`/Users/abdunabi/Desktop/vpn/server/tunnel/internal/config.go`**

Extend the config to support AmneziaWG:

```go
type Config struct {
    Port       int            `json:"port"`
    Protocol   string         `json:"protocol"`
    Reality    RealityConfig  `json:"reality"`
    AmneziaWG  AWGServerConfig `json:"amneziawg"`   // NEW
    HealthPort int            `json:"health_port"`
}

type AWGServerConfig struct {
    PrivateKey string `json:"private_key"`    // Server X25519 private key
    PublicKey  string `json:"public_key"`     // Server X25519 public key (for clients)
    ListenPort int    `json:"listen_port"`    // UDP port (e.g., 51820)
    Address    string `json:"address"`        // Server tunnel address (e.g., "10.8.0.1/24")

    // Obfuscation params (must match all clients)
    Jc   int `json:"jc"`
    Jmin int `json:"jmin"`
    Jmax int `json:"jmax"`
    S1   int `json:"s1"`
    S2   int `json:"s2"`
    H1   int `json:"h1"`
    H2   int `json:"h2"`
    H3   int `json:"h3"`
    H4   int `json:"h4"`
}
```

Update `validate()` to accept `"amneziawg"` protocol.

**NEW FILE: `/Users/abdunabi/Desktop/vpn/server/tunnel/internal/awg_server.go`**

The server-side AmneziaWG is best run as a system service using the `amneziawg-go` binary directly (not embedded in the Go server). The Go server should:
1. Generate/manage the AmneziaWG config file
2. Manage peer additions/removals via the UAPI socket
3. Start/stop the amneziawg-go process

However, for a simpler initial implementation, run the AmneziaWG server in-process using the library:

```go
package internal

import (
    "fmt"
    "net"

    "github.com/amnezia-vpn/amneziawg-go/conn"
    "github.com/amnezia-vpn/amneziawg-go/device"
    "github.com/amnezia-vpn/amneziawg-go/tun"
)

type AWGServer struct {
    device *device.Device
    config *AWGServerConfig
    logger *device.Logger
}

func NewAWGServer(config *AWGServerConfig) (*AWGServer, error) {
    return &AWGServer{config: config}, nil
}

func (s *AWGServer) Start() error {
    // Create TUN device
    tunDev, err := tun.CreateTUN("awg0", device.DefaultMTU)
    if err != nil {
        return fmt.Errorf("creating tun: %w", err)
    }

    logger := device.NewLogger(device.LogLevelVerbose, "(awg-server) ")
    dev := device.NewDevice(tunDev, conn.NewDefaultBind(), logger)

    // Configure via UAPI
    uapiConfig := s.buildServerUAPI()
    if err := dev.IpcSet(uapiConfig); err != nil {
        dev.Close()
        return fmt.Errorf("configuring awg server: %w", err)
    }

    dev.Up()
    s.device = dev

    // Configure IP address and routing on the TUN interface
    // This requires OS-level commands (ip addr add, ip route, sysctl)
    if err := s.configureRouting(); err != nil {
        dev.Close()
        return fmt.Errorf("configuring routing: %w", err)
    }

    return nil
}

func (s *AWGServer) Stop() error {
    if s.device != nil {
        s.device.Close()
    }
    return nil
}

func (s *AWGServer) AddPeer(publicKey string, allowedIPs string) error {
    // Add a peer via UAPI IpcSet
    config := fmt.Sprintf("public_key=%s\nallowed_ip=%s\n", publicKey, allowedIPs)
    return s.device.IpcSet(config)
}
```

### API Server

**`/Users/abdunabi/Desktop/vpn/server/api/`**

The API server's `/servers/:id/config` endpoint needs to return AmneziaWG config when the server's protocol is `amneziawg`. This means the `ServerConfig` response type needs to include the `amneziawg` field:

```go
// Response for GET /servers/:id/config
type ServerConfigResponse struct {
    ServerAddress string               `json:"server_address"`
    ServerPort    int                  `json:"server_port"`
    Protocol      string               `json:"protocol"`
    UserID        string               `json:"user_id,omitempty"`
    Reality       *RealityClientConfig `json:"reality,omitempty"`
    AmneziaWG     *AWGClientConfig     `json:"amneziawg,omitempty"` // NEW
    WebSocket     *WSClientConfig      `json:"websocket,omitempty"` // NEW
}
```

When `protocol == "amneziawg"`:
- Generate a client keypair per-device (or per-user)
- Return server public key, endpoint, obfuscation params
- Register client public key as a peer on the AmneziaWG server

### Frontend

**`/Users/abdunabi/Desktop/vpn/app/src/types/api.ts`**

Extend `ServerConfig`:

```typescript
export interface ServerConfig {
    server_address: string;
    server_port: number;
    protocol: string;
    user_id?: string;
    reality?: { /* existing */ };
    amneziawg?: {
        private_key: string;
        public_key: string;
        endpoint: string;
        allowed_ips: string;
        dns: string;
        preshared_key?: string;
        jc: number;
        jmin: number;
        jmax: number;
        s1: number;
        s2: number;
        h1: number;
        h2: number;
        h3: number;
        h4: number;
    };
    websocket?: { /* Feature 4 */ };
}
```

No changes needed in `vpnBridge.ts` -- it already passes the full `ServerConfig` as JSON to the native module, which passes it to the Go tunnel. The Go tunnel parses whatever fields are present.

### Complexity: High
- Go client tunnel: ~200 lines new code (awg.go + modifications to tunnel.go, tun.go)
- Go server: ~150 lines new code (awg_server.go + config changes)
- API server: ~50 lines (config response extension)
- Frontend: ~20 lines (type extension only)
- gomobile rebuild required

---

# Feature 4: WebSocket (CDN) Transport

## Decision

Use xray-core's built-in WebSocket transport (network: "ws") to route VLESS traffic through Cloudflare CDN. This reuses the existing xray-core + tun2socks pipeline -- only the xray config changes.

**NOTE:** xray-core's WebSocket transport is deprecated in favor of XHTTP. However, WebSocket is currently stable and widely supported by CDNs. We will use WebSocket now and plan migration to XHTTP in a future iteration.

Traffic flow:
```
Client -> xray-core (SOCKS5) -> VLESS over WebSocket -> Cloudflare CDN -> Server (WebSocket inbound) -> Internet
```

## Prerequisites

- A domain (e.g., `vpn.example.com`) pointed to Cloudflare
- Cloudflare proxy enabled (orange cloud) for that domain
- Cloudflare configured to forward WebSocket traffic to the origin server
- The origin server runs Nginx or xray directly with TLS + WebSocket

## Alternatives Considered

1. **XHTTP (H2/H3)** -- Newer, but less mature. Planned for future.
2. **gRPC transport** -- Also CDN-compatible but WebSocket has broader CDN support.
3. **Custom WebSocket wrapper around xray** -- Unnecessary; xray-core has native WS support.

## Tradeoffs

- WebSocket adds overhead vs raw TCP (HTTP upgrade, framing). Typically 5-15% slower.
- Requires a domain + Cloudflare setup (operational complexity).
- Cloudflare free tier has limits on WebSocket connections.
- `xtls-rprx-vision` flow is NOT compatible with WebSocket. Must use flow: "" (empty).

## Files to Modify / Create

### Go Client Tunnel

**`/Users/abdunabi/Desktop/vpn/client-tunnel/config.go`**

Add `WSClientConfig`:

```go
type WSClientConfig struct {
    Host string `json:"host"`   // Domain name (e.g., "vpn.example.com")
    Path string `json:"path"`   // WebSocket path (e.g., "/ws")
}
```

Add a new config builder function. The key difference from VLESS+REALITY:
- `network` is `"ws"` instead of `"tcp"`
- `security` is `"tls"` instead of `"reality"`
- `flow` must be `""` (empty, not "xtls-rprx-vision")
- `wsSettings` with `host` and `path`
- The server address points to the Cloudflare CDN domain, not the server IP

```go
func buildWebSocketXRayConfig(config ConnectConfig) map[string]interface{} {
    user, pass := getSocksAuth()

    wsHost := config.WebSocket.Host
    wsPath := config.WebSocket.Path
    if wsPath == "" {
        wsPath = "/ws"
    }

    return map[string]interface{}{
        "log": map[string]interface{}{
            "loglevel": "warning",
        },
        "dns": map[string]interface{}{
            "servers": []string{"1.1.1.1", "8.8.8.8"},
        },
        "inbounds": []map[string]interface{}{
            {
                "listen":   "127.0.0.1",
                "port":     localSocksPort,
                "protocol": "socks",
                "settings": map[string]interface{}{
                    "auth": "password",
                    "accounts": []map[string]interface{}{
                        {"user": user, "pass": pass},
                    },
                    "udp": true,
                },
                "sniffing": map[string]interface{}{
                    "enabled":      true,
                    "destOverride": []string{"http", "tls", "quic"},
                },
                "tag": "socks-in",
            },
        },
        "outbounds": []map[string]interface{}{
            {
                "protocol": "vless",
                "settings": map[string]interface{}{
                    "vnext": []map[string]interface{}{
                        {
                            "address": wsHost,            // CDN domain, not IP
                            "port":    443,               // Always 443 for CDN
                            "users": []map[string]interface{}{
                                {
                                    "id":         config.UserID,
                                    "flow":       "",            // NO flow for WS!
                                    "encryption": "none",
                                },
                            },
                        },
                    },
                },
                "streamSettings": map[string]interface{}{
                    "network":  "ws",                    // WebSocket
                    "security": "tls",                   // Standard TLS (Cloudflare terminates)
                    "tlsSettings": map[string]interface{}{
                        "serverName":  wsHost,
                        "fingerprint": "chrome",
                    },
                    "wsSettings": map[string]interface{}{
                        "path": wsPath,
                        "headers": map[string]interface{}{
                            "Host": wsHost,
                        },
                    },
                },
                "tag": "vless-out",
            },
            {
                "protocol": "freedom",
                "tag":      "direct",
            },
        },
        "routing": map[string]interface{}{
            "rules": []map[string]interface{}{
                {
                    "type":        "field",
                    "inboundTag":  []string{"socks-in"},
                    "outboundTag": "vless-out",
                },
            },
        },
    }
}
```

**`/Users/abdunabi/Desktop/vpn/client-tunnel/tunnel.go`** (imports)

Add WebSocket transport import:

```go
import (
    // ... existing imports ...
    _ "github.com/xtls/xray-core/transport/internet/websocket"  // NEW
    _ "github.com/xtls/xray-core/transport/internet/tls"        // NEW (standard TLS, not REALITY)
)
```

Modify `runXRayTunnel` to use the correct config builder:

```go
func (m *tunnelManager) runXRayTunnel(config ConnectConfig, readyCh chan<- string) {
    // ... existing setup code ...

    var xrayConfig map[string]interface{}
    switch config.Protocol {
    case "websocket":
        xrayConfig = buildWebSocketXRayConfig(config)
    default:
        xrayConfig = buildClientXRayConfig(config)  // existing VLESS+REALITY builder
    }

    // ... rest of existing code (json.Marshal, core.LoadConfig, etc.) ...
}
```

### Go Server Tunnel

**`/Users/abdunabi/Desktop/vpn/server/tunnel/internal/config.go`**

Add WebSocket config:

```go
type Config struct {
    Port       int             `json:"port"`
    Protocol   string          `json:"protocol"`
    Reality    RealityConfig   `json:"reality"`
    AmneziaWG  AWGServerConfig `json:"amneziawg"`
    WebSocket  WSServerConfig  `json:"websocket"`     // NEW
    HealthPort int             `json:"health_port"`
}

type WSServerConfig struct {
    Path   string `json:"path"`    // WebSocket path (e.g., "/ws")
    // TLS is handled by Cloudflare/Nginx, not by xray
}
```

Update `validate()` to accept `"websocket"` and `"vless-ws"`.

**`/Users/abdunabi/Desktop/vpn/server/tunnel/internal/server.go`**

Add WebSocket transport import:

```go
import (
    // ... existing ...
    _ "github.com/xtls/xray-core/transport/internet/websocket"
)
```

Modify `buildXRayConfig()` to handle WebSocket protocol:

```go
func (s *TunnelServer) buildXRayConfig() map[string]interface{} {
    switch s.config.Protocol {
    case "vless-reality":
        return s.buildRealityConfig()    // existing code extracted
    case "websocket", "vless-ws":
        return s.buildWebSocketConfig()  // NEW
    default:
        return s.buildRealityConfig()
    }
}

func (s *TunnelServer) buildWebSocketConfig() map[string]interface{} {
    return map[string]interface{}{
        "log": map[string]interface{}{
            "loglevel": "warning",
        },
        "inbounds": []map[string]interface{}{
            {
                "port":     s.config.Port,    // e.g., 8443 (Nginx proxies 443 -> 8443)
                "protocol": "vless",
                "settings": map[string]interface{}{
                    "clients": []map[string]interface{}{
                        {
                            "id":   "00000000-0000-0000-0000-000000000000",
                            "flow": "",  // No flow for WebSocket
                        },
                    },
                    "decryption": "none",
                },
                "streamSettings": map[string]interface{}{
                    "network": "ws",
                    "wsSettings": map[string]interface{}{
                        "path": s.config.WebSocket.Path,
                    },
                    // No TLS here -- Cloudflare/Nginx handles TLS termination
                },
            },
        },
        "outbounds": []map[string]interface{}{
            {
                "protocol": "freedom",
                "tag":      "direct",
            },
            {
                "protocol": "blackhole",
                "tag":      "block",
            },
        },
    }
}
```

### Nginx / Cloudflare Configuration (Documentation)

Create a setup guide. The server needs:

1. **Nginx config** (`/etc/nginx/conf.d/vpn-ws.conf`):
```nginx
server {
    listen 443 ssl http2;
    server_name vpn.example.com;

    ssl_certificate     /etc/letsencrypt/live/vpn.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/vpn.example.com/privkey.pem;

    location /ws {
        proxy_pass http://127.0.0.1:8443;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }

    # Fallback: serve a normal website
    location / {
        root /var/www/html;
    }
}
```

2. **Cloudflare DNS**: A record for `vpn.example.com` -> server IP, proxy ON (orange cloud)
3. **Cloudflare SSL**: Full (Strict) mode

### Frontend

**`/Users/abdunabi/Desktop/vpn/app/src/types/api.ts`**

Add to `ServerConfig`:

```typescript
websocket?: {
    host: string;
    path: string;
};
```

No other frontend changes needed -- the protocol selection UI already has the `websocket` option in `SettingsScreen.tsx`, and the config flows through unchanged via `vpnBridge.connect()`.

### Complexity: Medium
- Go client tunnel: ~80 lines new (config builder + imports)
- Go server: ~60 lines new (config builder + imports)
- Server ops: Nginx config + Cloudflare setup (documented, not coded)
- Frontend: ~5 lines (type extension)

---

## Summary: File Change Map

### Files to CREATE

| File | Feature | Lines (est.) |
|------|---------|-------------|
| `/Users/abdunabi/Desktop/vpn/client-tunnel/awg.go` | AmneziaWG | ~120 |
| `/Users/abdunabi/Desktop/vpn/server/tunnel/internal/awg_server.go` | AmneziaWG | ~100 |
| `/Users/abdunabi/Desktop/vpn/app/src/screens/SplitTunnelScreen.tsx` | Split Tunneling | ~200 |
| `/Users/abdunabi/Desktop/vpn/docs/websocket-cloudflare-setup.md` | WebSocket | ~80 |

### Files to MODIFY

| File | Features | Changes |
|------|----------|---------|
| `client-tunnel/tunnel.go` | AWG, WS | Protocol dispatch in Connect/runTunnel, add awgConfig field, WS imports |
| `client-tunnel/config.go` | AWG, WS, Split | New config types, WebSocket config builder, excluded domains routing |
| `client-tunnel/tun.go` | AWG | Branch on protocol in StartTun/StopTun |
| `client-tunnel/go.mod` | AWG | Add amneziawg-go dependency |
| `server/tunnel/internal/config.go` | AWG, WS | New config types, validate() update |
| `server/tunnel/internal/server.go` | WS | WebSocket config builder, WS import |
| `android/.../TunnelVpnService.kt` | Kill, Split | Kill switch logic, excluded apps, TUN rebuild |
| `android/.../VpnTurboModule.kt` | Kill, Split | setKillSwitch, setExcludedApps, getInstalledApps methods |
| `ios/.../PacketTunnelProvider.swift` | (minor) | Pass excluded domains to Go config |
| `ios/.../VpnManager.swift` | Kill | includeAllNetworks, setKillSwitch method |
| `ios/.../VpnModuleImpl.swift` | Kill, Split | setKillSwitch, setExcludedDomains methods |
| `app/src/types/vpn.ts` | Kill | Add kill_switch_active state |
| `app/src/types/api.ts` | AWG, WS | Extend ServerConfig |
| `app/src/types/native.ts` | Split | Add split tunneling methods |
| `app/src/stores/settingsStore.ts` | Split | Add excluded apps/domains state |
| `app/src/services/vpnBridge.ts` | Split | Add split tunneling bridge functions |
| `app/src/screens/SettingsScreen.tsx` | Split | Link to SplitTunnelScreen |

---

## Complexity and Effort Estimates

| Feature | Complexity | Effort (dev-days) | Risk |
|---------|-----------|-------------------|------|
| Kill Switch | Low-Medium | 2-3 | Low -- uses standard OS APIs |
| Split Tunneling | Medium | 3-4 | Medium -- iOS limitations, TUN rebuild |
| AmneziaWG | High | 5-7 | High -- new protocol path, gomobile compat unknown |
| WebSocket CDN | Medium | 3-4 | Low -- reuses xray-core, config-only change |

**Total: 13-18 dev-days**

---

## Risks and Mitigations

### AmneziaWG + gomobile compatibility
**Risk:** amneziawg-go may not compile cleanly with gomobile bind. The library uses `tun.CreateTUN()` which may depend on OS-specific APIs not available in gomobile.
**Mitigation:** Spike task first -- try `gomobile bind` with just the amneziawg-go import before writing integration code. If it fails, we may need to use amneziawg-go's TUN-from-fd approach (which is what we need anyway since the TUN fd comes from the OS VPN service).

### WebSocket deprecation in xray-core
**Risk:** xray-core may remove WebSocket transport in a future release.
**Mitigation:** WebSocket is currently stable in v1.8.x. Plan XHTTP migration as a separate future task. Pin xray-core version.

### Split Tunneling TUN rebuild on Android
**Risk:** Rebuilding the TUN interface while connected causes a brief traffic interruption.
**Mitigation:** Show a brief "Reconnecting" state in UI. The interruption is <1 second in practice.

### Kill Switch + Split Tunneling interaction on Android
**Risk:** If kill switch is active (TUN alive, no backend) and split tunneling is enabled, excluded apps should still have internet. The current "keep TUN alive" approach would also block excluded apps.
**Mitigation:** When kill switch activates, rebuild TUN WITHOUT the excluded apps filter. This way all traffic (including from excluded apps) goes to the dead TUN. This is correct behavior -- kill switch means "block everything."

---

## Rollback Strategy

Each feature is independently deployable:
- **Kill Switch:** Remove the `killSwitchEnabled` check in `stopVpn()`. Behavior reverts to always closing TUN.
- **Split Tunneling:** Remove `addDisallowedApplication` calls and excluded domain routing rules. All traffic goes through VPN.
- **AmneziaWG:** Remove the `"amneziawg"` case from protocol dispatch. Server can keep running but clients will not connect to it.
- **WebSocket:** Remove the `"websocket"` case from protocol dispatch and the WebSocket config builder. Server-side Nginx config can remain harmlessly.

---

## Implementation Order Recommendation

```
Week 1:  Kill Switch (Android + iOS)  ||  Split Tunneling (Android + iOS)
Week 2:  WebSocket CDN (client + server)  ||  AmneziaWG spike (gomobile compat test)
Week 3:  AmneziaWG (client + server, if spike succeeds)
Week 4:  Protocol selector enhancement + integration testing
```
