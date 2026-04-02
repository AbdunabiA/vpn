# ADR-001: Go Tunnel Library Integration via gomobile

**Status:** Proposed
**Date:** 2026-04-02
**Author:** Architect Agent

---

## Context

The VPN app has a React Native frontend (iOS + Android) and a Go-based tunnel library (`client-tunnel/`) that wraps xray-core to provide VLESS+REALITY tunneling. The Go library exposes a mobile-friendly API through package-level functions (`Connect`, `Disconnect`, `GetStatus`, `ProbeServers`, `GetTrafficStats`) and a `StatusCallback` interface.

Currently, the native layers (Android `TunnelVpnService.kt`, iOS `PacketTunnelProvider.swift`) have **placeholder comments** where the Go tunnel should be invoked. The bridge from React Native to native modules is complete (`vpnBridge.ts` -> `VpnModule` -> native). The missing piece is:

1. Compiling the Go library into platform artifacts (`.aar` / `.xcframework`)
2. Wiring those artifacts into the native VPN service layers
3. Handling the SOCKS5 proxy traffic routing at the OS level
4. Propagating status/stats back through the React Native bridge

## Decision

Integrate the Go tunnel library using gomobile bind, with a SOCKS5-to-TUN forwarding layer on each platform. The Go library runs a local SOCKS5 proxy on `localhost:10808`; native code is responsible for creating the TUN interface and forwarding packets to/from that proxy.

## Architecture Overview

```
+---------------------------------------------------------------+
|                     React Native (JS)                         |
|  vpnBridge.ts -> NativeModules.VpnModule.connect(configJSON)  |
+-----------------------------+---------------------------------+
                              |
              +---------------+---------------+
              |                               |
   +----------v-----------+      +-----------v-----------+
   |  Android (Kotlin)    |      |  iOS (Swift)          |
   |  VpnTurboModule.kt   |      |  VpnModuleImpl.swift  |
   |    |                 |      |    |                  |
   |    v                 |      |    v                  |
   |  TunnelVpnService.kt |      |  VpnManager.swift     |
   |  (VpnService)        |      |  (NETunnelProviderMgr)|
   |    |                 |      |    |                  |
   |    v                 |      |    v (IPC to ext.)    |
   |  [Go tunnel.aar]     |      |  PacketTunnelProvider |
   |  Tunnel.connect()    |      |  [Tunnel.xcframework] |
   |  Tunnel.disconnect() |      |  TunnelConnect()      |
   |    |                 |      |  TunnelDisconnect()   |
   |    v                 |      |    |                  |
   |  xray-core           |      |    v                  |
   |  SOCKS5@10808        |      |  xray-core            |
   |    |                 |      |  SOCKS5@10808          |
   |    v                 |      |    |                  |
   |  tun2socks           |      |    v                  |
   |  (TUN <-> SOCKS5)    |      |  tun2socks / NEPacket |
   +----------------------+      +-----------------------+
              |                               |
              +-------------------------------+
              |
     VLESS+REALITY over TCP:443
              |
     +--------v--------+
     | Remote Server    |
     | xray-core inbound|
     +------------------+
```

### Key Architectural Decision: tun2socks

The Go tunnel (xray-core) opens a SOCKS5 proxy on `localhost:10808`. The OS TUN interface captures all device traffic as raw IP packets. Something must bridge these two worlds. The options are:

| Approach | Pros | Cons |
|----------|------|------|
| **A) tun2socks in Go (badvpn/hev-socks5-tunnel)** | Single binary, no JNI overhead | Adds another Go dependency, increases binary size |
| **B) tun2socks as separate native lib** | Mature options exist (tun2socks-android) | Two native libraries to manage |
| **C) Read packets in native, forward to SOCKS5** | Full control, no extra deps | Complex to implement correctly (TCP reassembly) |
| **D) Go library reads TUN fd directly** | Eliminates SOCKS5 hop | Requires passing fd from native to Go, platform-specific |

**Decision:** Use approach **D** for Android (pass TUN fd to Go, use xray-core's tun inbound or a lightweight Go tun2socks like `xjasonlyu/tun2socks`) and approach **A** for iOS (the Go library handles tun2socks internally, since the Network Extension process has direct access to packet flow). Both platforms converge on the same Go library doing the packet-to-SOCKS5 translation.

**Revised approach after further analysis:** The simplest correct path is to add a Go-based tun2socks component inside `client-tunnel/` that accepts a file descriptor (Android) or uses `NEPacketTunnelProvider.packetFlow` (iOS). However, gomobile cannot pass file descriptors or iOS packet flow objects directly.

**Final decision:** Keep the current SOCKS5 architecture and add **platform-native tun2socks** forwarding:
- **Android:** Use `com.github.nicosogangstar:tun2socks` or embed a small Kotlin-based socket forwarder that reads from TUN fd and proxies to `localhost:10808`
- **iOS:** Use `NEPacketTunnelProvider`'s built-in packet interception and forward to the local SOCKS5 proxy via a lightweight Swift TCP proxy

Actually, the most battle-tested approach for both platforms: use **hev-socks5-tunnel** (C library, ~50KB) which is specifically designed for this use case and is used by many VPN apps. It reads from a TUN fd and forwards to a SOCKS5 proxy.

**FINAL DECISION:** Use a two-layer approach:
1. Go tunnel (xray-core) provides SOCKS5 on `localhost:10808` -- **already implemented**
2. Native layer creates TUN interface and uses a lightweight forwarder to pipe TUN traffic to SOCKS5

For Android: the VpnService creates TUN and uses a Java/Kotlin-based SOCKS5 forwarder (or shells out to the Go tunnel for this too by adding tun handling to the Go library).

For iOS: `NEPacketTunnelProvider` reads packets via `packetFlow` and forwards them.

**SIMPLEST CORRECT PATH (what we actually do):**
- Add `github.com/xjasonlyu/tun2socks/v2` to the Go module
- Expose `StartWithFD(fd int, configJSON string) string` for Android
- Expose `StartWithPacketFlow(configJSON string) string` for iOS (the Go code opens its own tun)
- On Android, pass the TUN fd from `VpnService.Builder.establish()` to Go
- On iOS, the Network Extension creates the TUN and the Go library connects to the SOCKS5 locally

**Actually, after reviewing what works in production apps:** The cleanest architecture is to NOT add tun2socks to Go. Instead:

1. Go library: SOCKS5 proxy on `localhost:10808` (done)
2. Android: Use `VpnService` TUN + a battle-tested Java tun2socks library or configure the VPN to use the SOCKS5 proxy via `builder.addRoute()` + `builder.setHttpProxy()` (Android 10+ only)
3. iOS: NEPacketTunnelProvider + `NetworkExtension` framework handles routing natively

The problem is that neither platform has a built-in "route TUN to SOCKS5" mechanism. We need tun2socks.

---

## FINAL ARCHITECTURE DECISION

After analyzing all approaches, here is the concrete plan:

### Layer 1: Go Tunnel Library (no changes to tunnel.go API)

The existing API is clean and correct:
- `Connect(configJSON string) string` -- starts xray-core, opens SOCKS5 on 10808
- `Disconnect() string` -- tears down xray-core
- `GetStatus() string` -- returns JSON status
- `SetStatusCallback(cb StatusCallback)` -- push notifications to native
- `ProbeServers(serversJSON string) string` -- latency probing
- `GetTrafficStats() string` -- bandwidth stats

One addition needed: `SetProtectFD(cb ProtectFDCallback)` for Android socket protection.

### Layer 2: tun2socks (added to Go module)

Add a thin Go wrapper around `github.com/xjasonlyu/tun2socks` that:
- On Android: accepts a TUN file descriptor via `StartTun2Socks(fd int) string`
- On iOS: accepts a TUN file descriptor via `StartTun2Socks(fd int) string`

Both platforms pass the TUN fd to Go. On iOS, the Network Extension creates the TUN via `setTunnelNetworkSettings` and the fd is available through the utun device.

### Layer 3: Native Platform Code

Android and iOS create the TUN interface, pass the fd to Go, and manage lifecycle.

---

## Detailed Implementation Plan

### Phase 1: Go Library Compilation & Socket Protection

#### 1.1 Add tun2socks dependency to Go module

**File:** `/Users/abdunabi/Desktop/vpn/client-tunnel/go.mod`

Add dependency:
```
require (
    github.com/xtls/xray-core v1.8.24
    github.com/xjasonlyu/tun2socks/v2 v2.5.2
)
```

#### 1.2 Create tun2socks bridge in Go

**File to create:** `/Users/abdunabi/Desktop/vpn/client-tunnel/tun.go`

Purpose: Accepts a TUN file descriptor and starts tun2socks forwarding to the local SOCKS5 proxy.

Exported functions (gomobile compatible -- all params are primitive types):
```go
// StartTun(fd int) string
//   Starts tun2socks: reads IP packets from the TUN fd,
//   forwards them to localhost:10808 (the xray SOCKS5 proxy).
//   Returns empty string on success, error message on failure.
//   Must be called AFTER Connect() succeeds.

// StopTun() string
//   Stops the tun2socks forwarder.
//   Must be called BEFORE Disconnect() or during cleanup.
```

#### 1.3 Add socket protection callback for Android

**File to create:** `/Users/abdunabi/Desktop/vpn/client-tunnel/protect.go`

On Android, VPN traffic must not be routed back through the TUN interface (infinite loop). Android's `VpnService.protect(socket)` exempts a socket. The Go library needs to call back into Kotlin whenever xray-core opens a new outbound socket.

Exported interface and function:
```go
// ProtectSocket is implemented by the Android native module.
// gomobile generates the Java interface.
type ProtectSocket interface {
    Protect(fd int) bool
}

// SetProtectCallback registers the Android socket protector.
// Must be called before Connect() on Android. No-op on iOS.
func SetProtectCallback(cb ProtectSocket)
```

Implementation: Hook into xray-core's `internet.RegisterDialerController` to intercept socket creation and call `cb.Protect(fd)` before the socket connects.

#### 1.4 Update Makefile

**File:** `/Users/abdunabi/Desktop/vpn/client-tunnel/Makefile`

Add variables for output paths that point directly into the app project:
```makefile
ANDROID_OUT = ../app/android/app/libs/tunnel.aar
IOS_OUT = ../app/ios/Frameworks/Tunnel.xcframework

android:
    gomobile bind -target=android -androidapi 24 -o $(ANDROID_OUT) ./

ios:
    gomobile bind -target=ios -o $(IOS_OUT) ./
```

Add a `verify` target that checks gomobile is installed and the Go module compiles.

### Phase 2: Android Integration

#### 2.1 Add tunnel.aar to Gradle

**File to modify:** `/Users/abdunabi/Desktop/vpn/app/android/app/build.gradle`

Add to dependencies block:
```groovy
dependencies {
    implementation("com.facebook.react:react-android")
    // ... existing deps ...

    // Go tunnel library compiled via gomobile
    implementation(files("libs/tunnel.aar"))
}
```

Create the libs directory:
```
app/android/app/libs/   <-- tunnel.aar goes here
```

#### 2.2 Implement socket protection in TunnelVpnService.kt

**File to modify:** `/Users/abdunabi/Desktop/vpn/app/android/app/src/main/java/com/vpnapp/vpn/TunnelVpnService.kt`

Changes:

1. Import the gomobile-generated tunnel package:
```kotlin
import tunnel.Tunnel
import tunnel.ProtectSocket
import tunnel.StatusCallback
```

2. Implement `ProtectSocket` interface on the service:
```kotlin
class TunnelVpnService : VpnService(), ProtectSocket {
    override fun protect(fd: Int): Boolean {
        return this.protect(fd)  // VpnService.protect() exempts socket from VPN routing
    }
}
```

Note: There is a name collision between `ProtectSocket.protect(fd)` and `VpnService.protect(fd)`. Since both have the same signature, the implementation naturally delegates. However, to avoid ambiguity, reference `super.protect(fd)` explicitly:
```kotlin
override fun protect(fd: Int): Boolean {
    return (this as VpnService).protect(fd)
}
```

3. Implement `StatusCallback` interface:
```kotlin
class TunnelVpnService : VpnService(), ProtectSocket, StatusCallback {
    override fun onStatusChanged(statusJSON: String) {
        VpnTurboModule.sendStatusEvent(statusJSON)
        // Update notification based on parsed state
    }
}
```

4. Replace placeholder in `startVpn()`:
```kotlin
private fun startVpn(configJson: String) {
    if (isRunning) return
    startForeground(NOTIFICATION_ID, buildNotification("Connecting..."))

    try {
        // 1. Register socket protection BEFORE connecting
        Tunnel.setProtectCallback(this)

        // 2. Register status callback
        Tunnel.setStatusCallback(this)

        // 3. Start the Go tunnel (xray-core SOCKS5 proxy)
        val connectResult = Tunnel.connect(configJson)
        if (connectResult.isNotEmpty()) {
            Log.e(TAG, "Tunnel connect error: $connectResult")
            VpnTurboModule.sendStatusEvent("""{"state":"error","error":"$connectResult"}""")
            stopSelf()
            return
        }

        // 4. Create TUN interface AFTER xray starts (so we can protect its sockets)
        val builder = Builder()
            .setSession("VPN App")
            .addAddress("10.0.0.2", 32)
            .addRoute("0.0.0.0", 0)
            .addDnsServer("1.1.1.1")
            .addDnsServer("8.8.8.8")
            .setMtu(1500)

        vpnInterface = builder.establish()
        if (vpnInterface == null) {
            Log.e(TAG, "Failed to establish VPN interface")
            Tunnel.disconnect()
            stopSelf()
            return
        }

        // 5. Start tun2socks: pipes TUN packets to SOCKS5 proxy
        val tunFd = vpnInterface!!.fd
        val tunResult = Tunnel.startTun(tunFd)
        if (tunResult.isNotEmpty()) {
            Log.e(TAG, "Tun2socks error: $tunResult")
            Tunnel.disconnect()
            vpnInterface?.close()
            stopSelf()
            return
        }

        isRunning = true
        updateNotification("Connected")

    } catch (e: Exception) {
        Log.e(TAG, "Failed to start VPN", e)
        VpnTurboModule.sendStatusEvent("""{"state":"error","error":"${e.message}"}""")
        stopSelf()
    }
}
```

5. Replace placeholder in `stopVpn()`:
```kotlin
private fun stopVpn() {
    if (!isRunning) return
    Log.i(TAG, "Stopping VPN tunnel")

    // Order matters: stop tun2socks first, then xray, then close TUN
    Tunnel.stopTun()
    Tunnel.disconnect()

    try {
        vpnInterface?.close()
        vpnInterface = null
    } catch (e: Exception) {
        Log.e(TAG, "Error closing VPN interface", e)
    }

    isRunning = false
    stopForeground(STOP_FOREGROUND_REMOVE)
    stopSelf()
}
```

#### 2.3 Update VpnTurboModule.kt for real Go tunnel calls

**File to modify:** `/Users/abdunabi/Desktop/vpn/app/android/app/src/main/java/com/vpnapp/vpn/VpnTurboModule.kt`

Replace stub implementations:

```kotlin
// getStatus: delegate to Go tunnel
@ReactMethod
fun getStatus(promise: Promise) {
    promise.resolve(Tunnel.getStatus())
}

// probeServers: delegate to Go tunnel
@ReactMethod
fun probeServers(serversJSON: String, promise: Promise) {
    // Run on background thread to avoid blocking JS
    Thread {
        try {
            val result = Tunnel.probeServers(serversJSON)
            promise.resolve(result)
        } catch (e: Exception) {
            promise.reject("PROBE_ERROR", e.message, e)
        }
    }.start()
}

// getTrafficStats: delegate to Go tunnel
@ReactMethod
fun getTrafficStats(promise: Promise) {
    promise.resolve(Tunnel.getTrafficStats())
}
```

#### 2.4 Android Manifest -- already correct

The existing manifest at `/Users/abdunabi/Desktop/vpn/app/android/app/src/main/AndroidManifest.xml` already declares:
- `INTERNET` permission
- `FOREGROUND_SERVICE` + `FOREGROUND_SERVICE_SPECIAL_USE` permissions
- `TunnelVpnService` with `BIND_VPN_SERVICE` permission and `specialUse` foreground type

No changes needed.

### Phase 3: iOS Integration

iOS is more complex because the VPN tunnel runs in a **separate process** (Network Extension). The Go library must be linked into the Network Extension target, not the main app target.

#### 3.1 Place Tunnel.xcframework

**Directory to create:** `/Users/abdunabi/Desktop/vpn/app/ios/Frameworks/`

The `make ios` command in `client-tunnel/Makefile` outputs `Tunnel.xcframework` here.

#### 3.2 Add Tunnel.xcframework to the Network Extension target in Xcode

**File to modify:** Xcode project (manual steps, documented here)

1. In Xcode, select the `VpnAppNetworkExtension` target
2. Go to "General" -> "Frameworks, Libraries, and Embedded Content"
3. Click "+" and "Add Other" -> "Add Files" -> select `ios/Frameworks/Tunnel.xcframework`
4. Set embed to "Embed & Sign"
5. Verify the framework appears in the target's "Build Phases" -> "Link Binary With Libraries"

Alternatively, automate via Podfile (see 3.6).

#### 3.3 Add entitlements files

**File to create:** `/Users/abdunabi/Desktop/vpn/app/ios/VpnApp/VpnApp.entitlements`
```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>com.apple.developer.networking.networkextension</key>
    <array>
        <string>packet-tunnel-provider</string>
    </array>
    <key>com.apple.security.application-groups</key>
    <array>
        <string>group.com.vpnapp.shared</string>
    </array>
</dict>
</plist>
```

**File to create:** `/Users/abdunabi/Desktop/vpn/app/ios/VpnAppNetworkExtension/VpnAppNetworkExtension.entitlements`
```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>com.apple.developer.networking.networkextension</key>
    <array>
        <string>packet-tunnel-provider</string>
    </array>
    <key>com.apple.security.application-groups</key>
    <array>
        <string>group.com.vpnapp.shared</string>
    </array>
</dict>
</plist>
```

The App Group (`group.com.vpnapp.shared`) enables:
- Sharing data between the main app and the Network Extension
- UserDefaults suite for passing configuration
- Shared container for logs

These entitlements also require corresponding provisioning profiles from the Apple Developer portal with `packet-tunnel-provider` capability enabled on both the app bundle ID (`com.vpnapp`) and the extension bundle ID (`com.vpnapp.VpnAppNetworkExtension`).

#### 3.4 Modify PacketTunnelProvider.swift

**File to modify:** `/Users/abdunabi/Desktop/vpn/app/ios/VpnAppNetworkExtension/PacketTunnelProvider.swift`

Full replacement:

```swift
import NetworkExtension
import os.log
import Tunnel  // gomobile-generated module

class PacketTunnelProvider: NEPacketTunnelProvider {

    private let log = OSLog(subsystem: "com.vpnapp.tunnel", category: "PacketTunnel")

    // Implement StatusCallback protocol (generated by gomobile)
    private class TunnelStatusHandler: NSObject, TunnelStatusCallbackProtocol {
        weak var provider: PacketTunnelProvider?

        func onStatusChanged(_ statusJSON: String?) {
            guard let json = statusJSON else { return }
            // Store in shared UserDefaults for main app to read
            let defaults = UserDefaults(suiteName: "group.com.vpnapp.shared")
            defaults?.set(json, forKey: "tunnel_status")
        }
    }

    private let statusHandler = TunnelStatusHandler()

    override func startTunnel(
        options: [String : NSObject]?,
        completionHandler: @escaping (Error?) -> Void
    ) {
        os_log("Starting VPN tunnel", log: log, type: .info)

        let configJSON = options?["config"] as? String ?? "{}"

        // 1. Register status callback
        statusHandler.provider = self
        TunnelSetStatusCallback(statusHandler)

        // 2. Start the Go tunnel (xray-core SOCKS5 proxy on 10808)
        let connectResult = TunnelConnect(configJSON)
        if !connectResult.isEmpty {
            os_log("Tunnel connect error: %{public}@",
                   log: log, type: .error, connectResult)
            completionHandler(NSError(
                domain: "com.vpnapp.tunnel", code: 1,
                userInfo: [NSLocalizedDescriptionKey: connectResult]))
            return
        }

        // 3. Configure tunnel network settings
        let settings = NEPacketTunnelNetworkSettings(
            tunnelRemoteAddress: "10.0.0.1")

        let ipv4 = NEIPv4Settings(
            addresses: ["10.0.0.2"],
            subnetMasks: ["255.255.255.0"])
        ipv4.includedRoutes = [NEIPv4Route.default()]
        settings.ipv4Settings = ipv4

        settings.dnsSettings = NEDNSSettings(
            servers: ["1.1.1.1", "8.8.8.8"])
        settings.mtu = 1500

        // Configure SOCKS5 proxy settings so iOS routes
        // intercepted traffic to the Go tunnel's SOCKS5 proxy
        let proxySettings = NEProxySettings()
        proxySettings.httpEnabled = false
        proxySettings.httpsEnabled = false
        proxySettings.socksEnabled = true
        proxySettings.socksServer = NEProxyServer(
            address: "127.0.0.1", port: 10808)
        settings.proxySettings = proxySettings

        // 4. Apply settings and signal success
        setTunnelNetworkSettings(settings) { [weak self] error in
            if let error = error {
                os_log("Failed to set tunnel settings: %{public}@",
                       log: self?.log ?? .default, type: .error,
                       error.localizedDescription)
                TunnelDisconnect()
                completionHandler(error)
                return
            }

            os_log("VPN tunnel started successfully",
                   log: self?.log ?? .default, type: .info)
            completionHandler(nil)
        }
    }

    override func stopTunnel(
        with reason: NEProviderStopReason,
        completionHandler: @escaping () -> Void
    ) {
        os_log("Stopping VPN tunnel, reason: %{public}d",
               log: log, type: .info, reason.rawValue)

        TunnelStopTun()
        TunnelDisconnect()
        completionHandler()
    }

    override func handleAppMessage(
        _ messageData: Data,
        completionHandler: ((Data?) -> Void)?
    ) {
        guard let message = String(data: messageData, encoding: .utf8) else {
            completionHandler?(nil)
            return
        }

        os_log("Received app message: %{public}@",
               log: log, type: .debug, message)

        switch message {
        case "status":
            let status = TunnelGetStatus()
            completionHandler?(status.data(using: .utf8))
        case "stats":
            let stats = TunnelGetTrafficStats()
            completionHandler?(stats.data(using: .utf8))
        default:
            completionHandler?(nil)
        }
    }
}
```

**IMPORTANT NOTE on iOS SOCKS5 routing:** `NEProxySettings` with `socksEnabled` only works for HTTP/HTTPS traffic, not raw TCP/UDP. For full tunnel support (all IP traffic), we need tun2socks on iOS too. Two options:

Option A (recommended): Use `NEPacketTunnelProvider.packetFlow` to read raw packets and forward them. This requires a Swift-side packet forwarder or using the Go tun2socks via a pipe fd.

Option B: Accept that iOS only tunnels HTTP/HTTPS traffic via proxy settings. Not a true VPN.

**For a real VPN, Option A is required.** The implementation:

1. After `setTunnelNetworkSettings` succeeds, create a UNIX domain socket pair
2. Start a goroutine in Go that reads from one end of the pair (acting as a TUN fd)
3. In Swift, read packets from `self.packetFlow.readPackets` and write them to the other end
4. Similarly, read responses from Go side and write to `self.packetFlow.writePackets`

This requires adding to the Go library:
```go
// StartTunWithPipe(readFD int, writeFD int) string
//   For iOS: uses a pipe/socket pair instead of a real TUN fd.
//   The Swift side bridges NEPacketTunnelProvider.packetFlow <-> this pipe.
```

However, this adds significant complexity. A simpler production-proven approach:

**RECOMMENDED iOS APPROACH:** Use `Tunnel.startTun(fd)` where `fd` is obtained from the utun device that `NEPacketTunnelProvider` creates. After `setTunnelNetworkSettings` completes, the system creates a utun interface. The fd can be obtained by:

```swift
// The tunnel fd is not directly exposed by NEPacketTunnelProvider.
// Instead, use packetFlow to create a bridge.
```

Since iOS does not expose the raw TUN fd to the Network Extension, the correct approach is the **packet flow bridge**. I will document this as a separate component.

#### 3.5 Create iOS packet flow bridge

**File to create:** `/Users/abdunabi/Desktop/vpn/app/ios/VpnAppNetworkExtension/PacketFlowBridge.swift`

This Swift class bridges `NEPacketTunnelProvider.packetFlow` (which provides raw IP packets) to the Go tun2socks layer via a file descriptor pair.

```swift
import NetworkExtension

class PacketFlowBridge {
    private let packetFlow: NEPacketTunnelFlow
    private var readFD: Int32 = -1
    private var writeFD: Int32 = -1
    private var running = false

    init(packetFlow: NEPacketTunnelFlow) {
        self.packetFlow = packetFlow
    }

    /// Creates a socketpair and returns the Go-side fd.
    /// The Swift side reads/writes the other end, bridging to packetFlow.
    func start() -> Int32 {
        var fds: [Int32] = [0, 0]
        let result = socketpair(AF_UNIX, SOCK_DGRAM, 0, &fds)
        guard result == 0 else { return -1 }

        // fds[0] = Swift side, fds[1] = Go side
        readFD = fds[0]
        writeFD = fds[1]
        running = true

        // Start reading from packetFlow -> write to Go
        readFromPacketFlow()

        // Start reading from Go -> write to packetFlow
        readFromGo()

        return writeFD  // Give this fd to Go's tun2socks
    }

    func stop() {
        running = false
        if readFD >= 0 { close(readFD) }
        if writeFD >= 0 { close(writeFD) }
        readFD = -1
        writeFD = -1
    }

    private func readFromPacketFlow() {
        packetFlow.readPackets { [weak self] packets, protocols in
            guard let self = self, self.running else { return }
            for packet in packets {
                packet.withUnsafeBytes { ptr in
                    guard let base = ptr.baseAddress else { return }
                    write(self.readFD, base, packet.count)
                }
            }
            // Continue reading
            self.readFromPacketFlow()
        }
    }

    private func readFromGo() {
        DispatchQueue.global(qos: .userInteractive).async { [weak self] in
            let buffer = UnsafeMutablePointer<UInt8>.allocate(capacity: 1500)
            defer { buffer.deallocate() }

            while let self = self, self.running {
                let n = read(self.readFD, buffer, 1500)
                if n <= 0 { break }
                let data = Data(bytes: buffer, count: n)
                // AF_INET = 2 for IPv4
                self.packetFlow.writePackets([data], withProtocols: [2 as NSNumber])
            }
        }
    }
}
```

Usage in `PacketTunnelProvider.startTunnel()` after `setTunnelNetworkSettings` succeeds:
```swift
let bridge = PacketFlowBridge(packetFlow: self.packetFlow)
let goFD = bridge.start()
if goFD < 0 {
    // handle error
}
let tunResult = TunnelStartTun(Int(goFD))
```

#### 3.6 Update Podfile for Network Extension

**File to modify:** `/Users/abdunabi/Desktop/vpn/app/ios/Podfile`

The Network Extension target needs its own pod target for compilation settings:

```ruby
# After the VpnApp target block, add:

target 'VpnAppNetworkExtension' do
  # Network Extension does NOT need React Native pods
  # Only needs system frameworks + Go tunnel
end
```

The `Tunnel.xcframework` is added via Xcode (not CocoaPods) since it is a pre-built binary.

#### 3.7 Update VpnModuleImpl.swift for real status/stats

**File to modify:** `/Users/abdunabi/Desktop/vpn/app/ios/VpnApp/VpnModuleImpl.swift`

The main app does NOT import the Tunnel framework (it runs in the Network Extension). Communication uses `NETunnelProviderSession.sendProviderMessage`.

Replace stub implementations:

```swift
@objc func getStatus(_ resolve: @escaping RCTPromiseResolveBlock,
                     reject: @escaping RCTPromiseRejectBlock) {
    VpnManager.shared.sendMessage("status") { response in
        resolve(response ?? VpnManager.shared.getStatus())
    }
}

@objc func probeServers(_ serversJSON: String,
                        resolve: @escaping RCTPromiseResolveBlock,
                        reject: @escaping RCTPromiseRejectBlock) {
    // ProbeServers does NOT need the tunnel -- it's just TCP probing.
    // We CAN import Tunnel in the main app just for this function.
    // OR we can do probing from Swift directly.
    // DECISION: Import Tunnel.xcframework in the main app target too,
    // but only call ProbeServers (not Connect/Disconnect).
    let result = TunnelProbeServers(serversJSON)
    resolve(result)
}

@objc func getTrafficStats(_ resolve: @escaping RCTPromiseResolveBlock,
                           reject: @escaping RCTPromiseRejectBlock) {
    VpnManager.shared.sendMessage("stats") { response in
        resolve(response ?? """
            {"bytes_up":0,"bytes_down":0,"speed_up_bps":0,"speed_down_bps":0,"duration_secs":0}
            """)
    }
}
```

**Architecture note:** Importing `Tunnel.xcframework` in the main app target is fine for `ProbeServers` since it does not start xray-core. The tunnel lifecycle (`Connect`/`Disconnect`) must only be called from the Network Extension process.

### Phase 4: React Native Bridge

#### 4.1 vpnBridge.ts -- No changes needed

The existing bridge at `/Users/abdunabi/Desktop/vpn/app/src/services/vpnBridge.ts` is already correctly wired:
- `connect(config)` -> `VpnModule.connect(JSON.stringify(config))`
- `disconnect()` -> `VpnModule.disconnect()`
- `getStatus()` -> `VpnModule.getStatus()` -> parse JSON
- `onStatusChanged(cb)` -> listens to `onVpnStatusChanged` event
- `onStatsUpdated(cb)` -> listens to `onVpnStatsUpdated` event
- `probeServers(servers)` -> `VpnModule.probeServers(JSON.stringify(servers))`

The `ServerConfig` TypeScript type matches the Go `ConnectConfig` struct field-for-field.

No changes required. The bridge is platform-agnostic and delegates everything to native modules.

#### 4.2 Type definitions -- No changes needed

`/Users/abdunabi/Desktop/vpn/app/src/types/vpn.ts` and `/Users/abdunabi/Desktop/vpn/app/src/types/native.ts` already define the correct interfaces matching the Go structs.

### Phase 5: Build System

#### 5.1 Updated Makefile

**File to modify:** `/Users/abdunabi/Desktop/vpn/client-tunnel/Makefile`

```makefile
# Client Tunnel Library - gomobile build targets

.PHONY: all android ios clean setup verify

# Output paths -- directly into the app project
ANDROID_OUT = ../app/android/app/libs/tunnel.aar
IOS_OUT     = ../app/ios/Frameworks/Tunnel.xcframework

# Ensure output directories exist
$(ANDROID_OUT): | ../app/android/app/libs
$(IOS_OUT): | ../app/ios/Frameworks

../app/android/app/libs:
	mkdir -p $@

../app/ios/Frameworks:
	mkdir -p $@

# Install gomobile and initialize
setup:
	go install golang.org/x/mobile/cmd/gomobile@latest
	go install golang.org/x/mobile/cmd/gobind@latest
	gomobile init

# Verify environment before building
verify:
	@which gomobile > /dev/null || (echo "ERROR: gomobile not found. Run 'make setup'" && exit 1)
	@go build ./...
	@echo "Environment OK"

# Build for Android
android: verify | ../app/android/app/libs
	gomobile bind -target=android -androidapi 24 -o $(ANDROID_OUT) ./
	@echo "Built $(ANDROID_OUT)"

# Build for iOS (arm64 only, no simulator for xray-core CGO deps)
ios: verify | ../app/ios/Frameworks
	gomobile bind -target=ios/arm64 -o $(IOS_OUT) ./
	@echo "Built $(IOS_OUT)"

# Build for iOS including simulator (if xray-core supports it)
ios-sim: verify | ../app/ios/Frameworks
	gomobile bind -target=ios -o $(IOS_OUT) ./

# Build for both platforms
all: android ios

# Clean build artifacts
clean:
	rm -f tunnel.aar tunnel-sources.jar
	rm -rf Tunnel.xcframework
	rm -f $(ANDROID_OUT)
	rm -rf $(IOS_OUT)

# Fetch and tidy dependencies
deps:
	go mod tidy
	go mod download
```

#### 5.2 Gradle integration for .aar

**File to modify:** `/Users/abdunabi/Desktop/vpn/app/android/app/build.gradle`

Add to `dependencies` block:
```groovy
// Go tunnel library (compiled via gomobile)
implementation(fileTree(mapOf("dir" to "libs", "include" to listOf("*.aar"))))
```

No other Gradle changes needed. The `.aar` contains all native `.so` libraries for the supported ABIs.

#### 5.3 iOS Xcode project configuration (manual steps)

These steps must be performed in Xcode (or via `ruby` script modifying `project.pbxproj`):

1. **Add Tunnel.xcframework to the Xcode project:**
   - Drag `ios/Frameworks/Tunnel.xcframework` into the Xcode project navigator
   - When prompted, add to BOTH targets: `VpnApp` (for ProbeServers) and `VpnAppNetworkExtension` (for tunnel lifecycle)
   - Set "Embed & Sign" for both targets

2. **Add entitlements to both targets:**
   - `VpnApp` target -> "Signing & Capabilities" -> "+" -> "Network Extension" -> check "Packet Tunnel"
   - `VpnApp` target -> "+" -> "App Groups" -> add `group.com.vpnapp.shared`
   - `VpnAppNetworkExtension` target -> same capabilities

3. **Set Framework Search Paths:**
   - Both targets -> Build Settings -> "Framework Search Paths" -> add `$(PROJECT_DIR)/Frameworks`

4. **Bridging header (if needed):**
   - gomobile generates an Objective-C compatible framework; Swift can import it directly with `import Tunnel`

### Phase 6: Go Library Additions (Complete File Specifications)

#### 6.1 `/Users/abdunabi/Desktop/vpn/client-tunnel/tun.go`

```go
package tunnel

import (
    "fmt"
    "os"
    "sync"

    "github.com/xjasonlyu/tun2socks/v2/engine"
)

var (
    tunMu      sync.Mutex
    tunRunning bool
)

// StartTun starts the tun2socks engine that bridges a TUN file descriptor
// to the local SOCKS5 proxy opened by Connect().
//
// fd: the TUN file descriptor from the OS VPN service
//     - Android: VpnService.Builder.establish().getFd()
//     - iOS: socketpair fd bridged to NEPacketTunnelFlow
//
// Returns empty string on success, error message on failure.
// Must be called AFTER Connect() has successfully started xray-core.
func StartTun(fd int) string {
    tunMu.Lock()
    defer tunMu.Unlock()

    if tunRunning {
        return "tun2socks already running"
    }

    // Duplicate the fd so Go owns its own copy
    file := os.NewFile(uintptr(fd), "tun")
    if file == nil {
        return "invalid file descriptor"
    }

    key := new(engine.Key)
    key.Proxy = "socks5://127.0.0.1:10808"
    key.Device = fmt.Sprintf("fd://%d", fd)
    key.LogLevel = "warn"

    engine.Insert(key)

    if err := engine.Start(); err != nil {
        return fmt.Sprintf("tun2socks start error: %v", err)
    }

    tunRunning = true
    return ""
}

// StopTun stops the tun2socks engine.
// Must be called before Disconnect() to cleanly shut down.
func StopTun() string {
    tunMu.Lock()
    defer tunMu.Unlock()

    if !tunRunning {
        return ""
    }

    engine.Stop()
    tunRunning = false
    return ""
}
```

**IMPORTANT gomobile caveat:** The `tun2socks` library uses CGO and may have compatibility issues with gomobile. If `xjasonlyu/tun2socks` does not compile under gomobile, the alternative is to use `hev-socks5-tunnel` as a C library linked separately, or use the simpler `badvpn` tun2socks approach. This is a **build-time risk** that must be validated in Phase 7 (Testing).

If tun2socks integration proves problematic, the fallback plan is documented in the Alternatives section below.

#### 6.2 `/Users/abdunabi/Desktop/vpn/client-tunnel/protect.go`

```go
package tunnel

// ProtectSocket is implemented by the Android VpnService to prevent
// the tunnel's own sockets from being routed through the VPN (which
// would create an infinite loop).
//
// gomobile generates a Java interface: tunnel.ProtectSocket
// The Android TunnelVpnService implements it by calling VpnService.protect(fd).
//
// On iOS this is not needed because the Network Extension process
// is exempt from its own tunnel by default.
type ProtectSocket interface {
    Protect(fd int) bool
}

var protectCallback ProtectSocket

// SetProtectCallback registers the socket protection callback.
// Must be called before Connect() on Android.
func SetProtectCallback(cb ProtectSocket) {
    protectCallback = cb
}
```

To actually hook this into xray-core's socket creation, add to `tunnel.go` in the `init()` or before `core.New()`:

```go
import "github.com/xtls/xray-core/transport/internet"

// In runTunnel(), before creating the xray instance:
if protectCallback != nil {
    internet.RegisterDialerController(func(network, address string, fd uintptr) error {
        if !protectCallback.Protect(int(fd)) {
            return fmt.Errorf("failed to protect socket fd=%d", fd)
        }
        return nil
    })
}
```

#### 6.3 Updated `go.mod`

```
module vpnapp/client-tunnel

go 1.22.0

require (
    github.com/xtls/xray-core v1.8.24
    github.com/xjasonlyu/tun2socks/v2 v2.5.2
)
```

Run `go mod tidy` after adding the dependency to resolve transitive deps.

### Phase 7: Testing Strategy

#### 7.1 Go library unit tests

**File to create:** `/Users/abdunabi/Desktop/vpn/client-tunnel/tunnel_test.go`

Test the Go library independently before mobile integration:

```go
// TestConnectDisconnectLifecycle:
//   - Call Connect() with valid config -> verify state transitions
//   - Call GetStatus() -> verify JSON structure
//   - Call Disconnect() -> verify cleanup
//
// TestInvalidConfig:
//   - Call Connect("garbage") -> verify error return
//   - Call Connect("{}") -> verify meaningful error
//
// TestDoubleConnect:
//   - Connect() -> Connect() again -> verify "already connected" error
//
// TestStatusCallback:
//   - Register callback, Connect(), verify callback receives state changes
//
// TestProbeServers:
//   - Probe a known-reachable server (e.g., 1.1.1.1:443)
//   - Probe an unreachable server -> verify error in result
```

Run with: `cd client-tunnel && go test -v ./...`

#### 7.2 gomobile build verification

**Test before any native integration:**

```bash
cd /path/to/vpn/client-tunnel

# Verify Android build
make android
# Success = tunnel.aar exists, contains classes.jar and jni/ with .so files
unzip -l ../app/android/app/libs/tunnel.aar | head -20

# Verify iOS build
make ios
# Success = Tunnel.xcframework exists with arm64 slice
ls -la ../app/ios/Frameworks/Tunnel.xcframework/
```

If the build fails due to CGO/tun2socks, fall back to building without tun.go:
```bash
# Temporarily remove tun.go and build core tunnel only
mv tun.go tun.go.bak
make android
make ios
mv tun.go.bak tun.go
```

#### 7.3 Android integration test

1. Build and install the app on a physical Android device (emulator cannot do VPN)
2. Test sequence:
   - Tap connect -> verify VPN permission dialog appears
   - Grant permission -> verify VPN icon appears in status bar
   - Verify notification shows "Connected"
   - Open a browser -> verify pages load (traffic is tunneled)
   - Check `adb logcat | grep TunnelVpnService` for errors
   - Tap disconnect -> verify VPN icon disappears
   - Verify notification is dismissed

3. Edge cases:
   - Kill the app while connected -> verify VPN stays up (foreground service)
   - Toggle airplane mode -> verify reconnection or clean error
   - Connect with invalid config -> verify error propagates to JS

#### 7.4 iOS integration test

1. Build and run on a physical iOS device (provisioning profile required)
2. Test sequence:
   - Tap connect -> verify "Allow VPN Configuration" system dialog
   - Approve -> verify VPN icon appears in status bar
   - Open Safari -> verify pages load
   - Check Xcode console for `PacketTunnel` logs
   - Tap disconnect -> verify VPN icon disappears

3. Edge cases:
   - Background the app -> verify tunnel stays up
   - Lock the device -> verify `disconnectOnSleep = false` works
   - Force-kill the app -> verify Network Extension keeps running
   - Send IPC messages (status, stats) -> verify responses

#### 7.5 End-to-end test with real server

Requirements:
- A running server with the config from `/Users/abdunabi/Desktop/vpn/server/tunnel/`
- A `ServerConfig` JSON with valid `server_address`, `server_port`, `user_id`, and `reality` credentials

Test:
```json
{
  "server_address": "<server_ip>",
  "server_port": 443,
  "protocol": "vless-reality",
  "user_id": "<uuid>",
  "reality": {
    "public_key": "<reality_public_key>",
    "short_id": "<short_id>",
    "server_name": "www.microsoft.com",
    "fingerprint": "chrome"
  }
}
```

Verify:
- `curl ifconfig.me` from the device shows the server's IP (not the device's real IP)
- DNS queries go through the tunnel (use `1.1.1.1/cdn-cgi/trace`)
- Traffic stats update in the UI

---

## Risk Register

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| tun2socks library incompatible with gomobile | Medium | High | Fallback: use separate native tun2socks lib per platform |
| xray-core binary size too large (~30MB per ABI) | High | Medium | Use `-ldflags="-s -w"`, strip debug symbols, consider `-trimpath` |
| iOS Network Extension memory limit (6MB on older devices, 50MB on newer) | Medium | High | Profile memory; if too large, strip unused xray-core transports |
| Socket protection race condition on Android | Low | High | Register `DialerController` before `core.New()`, not after |
| iOS PacketFlowBridge performance | Medium | Medium | Use larger buffer (64KB), batch writes, profile with Instruments |
| Go version compatibility | High | High | **Go 1.23 required.** Go 1.22 has a macOS dyld linker bug (missing LC_UUID). Go 1.26 breaks `sagernet/sing` (invalid reference to `net.errNoSuchInterface`). Pinned to `go 1.23.0` in `go.mod`. |

## Alternatives Considered

### Alternative A: Embed V2Ray/Xray mobile SDKs instead of gomobile

Projects like `AioXray` and `v2rayNG` have pre-built Android/iOS libraries. We rejected this because:
- Less control over the SOCKS5 port and configuration
- Harder to customize for REALITY-only (they bundle all protocols)
- License compatibility concerns

### Alternative B: Use WireGuard instead of VLESS+REALITY

WireGuard has excellent native mobile SDKs. We rejected this because:
- WireGuard traffic is easily fingerprinted and blocked by DPI
- REALITY is specifically designed to evade censorship by impersonating TLS traffic
- The server is already built for VLESS+REALITY

### Alternative C: Run tun2socks as a separate native library

Instead of embedding tun2socks in the Go binary, use platform-specific solutions:
- Android: `tun2socks-android` (Java library)
- iOS: `hev-socks5-tunnel` (C library)

This was rejected as the primary approach because it doubles the native library management. However, it remains the **fallback plan** if gomobile + tun2socks compilation fails.

### Alternative D: Skip tun2socks, use HTTP proxy only

Configure each platform to use `localhost:10808` as a SOCKS5 proxy in system settings. This would:
- Work for browser traffic
- NOT work for non-HTTP apps, DNS, or raw TCP/UDP
- NOT be a true VPN

Rejected because users expect all traffic to be tunneled.

## Consequences

### Positive
- Single Go codebase for tunnel logic, shared across Android and iOS
- xray-core gives us battle-tested VLESS+REALITY implementation
- Status callback pattern provides real-time state updates to the UI
- Traffic stats are collected at the tunnel level, accurate for all protocols

### Negative
- Binary size: xray-core + tun2socks adds ~20-40MB per platform (before stripping)
- Build complexity: gomobile adds a toolchain dependency
- iOS memory constraints may require careful profiling
- Two-process architecture on iOS (main app + Network Extension) adds IPC complexity

### Neutral
- The React Native bridge layer requires zero changes
- The existing Android Manifest and service declarations are already correct
- The existing iOS Podfile needs minimal changes (one new target block)

---

## Implementation Task Checklist

Ordered by dependency. Each task should be a separate commit.

### Batch 1: Go Library (no native dependencies)
- [ ] **T1:** Create `/Users/abdunabi/Desktop/vpn/client-tunnel/protect.go` with `ProtectSocket` interface and `SetProtectCallback()`
- [ ] **T2:** Add `internet.RegisterDialerController` call in `tunnel.go` `runTunnel()` method before `core.New()`
- [ ] **T3:** Update `go.mod` to add tun2socks dependency, run `go mod tidy`
- [ ] **T4:** Create `/Users/abdunabi/Desktop/vpn/client-tunnel/tun.go` with `StartTun()` / `StopTun()`
- [ ] **T5:** Create `/Users/abdunabi/Desktop/vpn/client-tunnel/tunnel_test.go` with unit tests
- [ ] **T6:** Update `/Users/abdunabi/Desktop/vpn/client-tunnel/Makefile` with new targets and output paths
- [ ] **T7:** Run `make android` and `make ios` -- verify builds succeed. If tun2socks fails, implement fallback.

### Batch 2: Android Integration
- [ ] **T8:** Create directory `/Users/abdunabi/Desktop/vpn/app/android/app/libs/` and place `tunnel.aar`
- [ ] **T9:** Add `fileTree` dependency to `/Users/abdunabi/Desktop/vpn/app/android/app/build.gradle`
- [ ] **T10:** Modify `/Users/abdunabi/Desktop/vpn/app/android/app/src/main/java/com/vpnapp/vpn/TunnelVpnService.kt` -- implement `ProtectSocket`, `StatusCallback`, replace `startVpn()` and `stopVpn()` placeholder code
- [ ] **T11:** Modify `/Users/abdunabi/Desktop/vpn/app/android/app/src/main/java/com/vpnapp/vpn/VpnTurboModule.kt` -- replace stub `getStatus()`, `probeServers()`, `getTrafficStats()` with Go tunnel calls
- [ ] **T12:** Test on physical Android device

### Batch 3: iOS Integration
- [ ] **T13:** Create directory `/Users/abdunabi/Desktop/vpn/app/ios/Frameworks/` and place `Tunnel.xcframework`
- [ ] **T14:** Create entitlements files for both targets
- [ ] **T15:** Add `Tunnel.xcframework` to both Xcode targets (manual Xcode configuration)
- [ ] **T16:** Enable Network Extension + App Groups capabilities in Xcode
- [ ] **T17:** Create `/Users/abdunabi/Desktop/vpn/app/ios/VpnAppNetworkExtension/PacketFlowBridge.swift`
- [ ] **T18:** Modify `/Users/abdunabi/Desktop/vpn/app/ios/VpnAppNetworkExtension/PacketTunnelProvider.swift` -- import Tunnel, call Connect/Disconnect, wire PacketFlowBridge
- [ ] **T19:** Modify `/Users/abdunabi/Desktop/vpn/app/ios/VpnApp/VpnModuleImpl.swift` -- replace stubs with IPC calls and TunnelProbeServers
- [ ] **T20:** Update `/Users/abdunabi/Desktop/vpn/app/ios/Podfile` with Network Extension target
- [ ] **T21:** Test on physical iOS device

### Batch 4: Validation
- [ ] **T22:** End-to-end test with real VLESS+REALITY server on both platforms
- [ ] **T23:** Measure binary size, profile memory usage on iOS Network Extension
- [ ] **T24:** Test edge cases: airplane mode, app kill, sleep/wake, permission revocation

---

## File Summary

### Files to CREATE
| File | Purpose |
|------|---------|
| `client-tunnel/protect.go` | Android socket protection callback interface |
| `client-tunnel/tun.go` | tun2socks bridge (TUN fd to SOCKS5 forwarding) |
| `client-tunnel/tunnel_test.go` | Unit tests for Go library |
| `app/ios/Frameworks/` (directory) | Container for Tunnel.xcframework |
| `app/android/app/libs/` (directory) | Container for tunnel.aar |
| `app/ios/VpnApp/VpnApp.entitlements` | Network Extension + App Groups entitlements |
| `app/ios/VpnAppNetworkExtension/VpnAppNetworkExtension.entitlements` | Same, for extension target |
| `app/ios/VpnAppNetworkExtension/PacketFlowBridge.swift` | Bridges NEPacketTunnelFlow to Go tun fd |

### Files to MODIFY
| File | Changes |
|------|---------|
| `client-tunnel/go.mod` | Add tun2socks dependency |
| `client-tunnel/tunnel.go` | Add DialerController registration in runTunnel() |
| `client-tunnel/Makefile` | Output paths, verify target, iOS arm64-only option |
| `app/android/app/build.gradle` | Add libs/ fileTree dependency for .aar |
| `app/android/.../TunnelVpnService.kt` | Full Go tunnel integration (connect, disconnect, protect, status) |
| `app/android/.../VpnTurboModule.kt` | Replace stubs with Tunnel.* calls |
| `app/ios/.../PacketTunnelProvider.swift` | Full Go tunnel integration (connect, disconnect, packetflow bridge) |
| `app/ios/.../VpnModuleImpl.swift` | Replace stubs with IPC and TunnelProbeServers |
| `app/ios/Podfile` | Add VpnAppNetworkExtension target |

### Files that need NO changes
| File | Reason |
|------|--------|
| `app/src/services/vpnBridge.ts` | Bridge is already complete and correct |
| `app/src/types/vpn.ts` | Types match Go structs |
| `app/src/types/native.ts` | Event names and interface correct |
| `app/src/types/api.ts` | ServerConfig matches ConnectConfig |
| `app/ios/VpnApp/VpnManager.swift` | Already handles NETunnelProviderManager lifecycle |
| `app/ios/VpnApp/VpnModule.m` | ObjC bridge already exposes all methods |
| `app/android/.../AndroidManifest.xml` | Service and permissions already declared |
| `app/android/.../VpnPackage.kt` | Package registration already correct |
