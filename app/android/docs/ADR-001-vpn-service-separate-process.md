# ADR-001: Move TunnelVpnService to a Separate Android Process

**Status:** Proposed
**Date:** 2026-04-04
**Author:** Architecture Review

---

## Context

The VPN app runs a React Native UI and a `TunnelVpnService` (Go tunnel via gomobile) in the same OS process. When Android is under memory pressure and the user switches to a heavy app (Instagram, games), the OOM killer sends SIGKILL to the entire process -- killing both the UI and the active VPN connection.

A foreground service with a notification should get higher OOM priority, but because it shares a process with the React Native runtime (which consumes significant memory), the entire process becomes a larger target.

### Current Architecture (Single Process)

```
+---------------------------------------------------------------+
|  Process: com.vpnapp (main)                                   |
|                                                                |
|  +------------------+    static ref    +--------------------+  |
|  | VpnTurboModule   | <-------------> | TunnelVpnService   |  |
|  | (React Native)   |                 | (VpnService)       |  |
|  |                  |  companion obj   |                    |  |
|  | - connect()      | ------------->  | - Go tunnel lib    |  |
|  | - disconnect()   |  instance ref   | - xray-core        |  |
|  | - getStatus()    |                 | - tun2socks        |  |
|  | - connectPromise | <-------------- | - TUN fd           |  |
|  +------------------+  sendStatusEvent +--------------------+  |
|                                                                |
|  +------------------+                                          |
|  | React Native     |                                          |
|  | JS Runtime       |  <-- large memory footprint              |
|  +------------------+                                          |
+---------------------------------------------------------------+
```

### Coupling Points (what breaks with separate processes)

1. **`TunnelVpnService.instance` (companion object static ref)** -- Used by VpnTurboModule to call `setKillSwitch()`, `setExcludedApps()`, `isKillSwitchActive()`, `isActive()`. Static references are per-process; this will be `null` in the UI process.

2. **`VpnTurboModule.sendStatusEvent()` (static method)** -- Called by TunnelVpnService to push status to JS. In a separate process, `reactCtx` is null, `connectPromise` is null.

3. **`VpnTurboModule.resolveConnectPromise()` (static method)** -- Called at end of `startVpn()` to resolve the JS promise. Same problem.

4. **`Tunnel.getStatus()` / `Tunnel.getTrafficStats()` / `Tunnel.probeServers()`** -- Called directly by VpnTurboModule. The Go library will be loaded in the `:vpn` process, not the UI process. These calls will either crash or return empty data.

5. **SharedPreferences** -- Already used for `kill_switch_enabled` and `excluded_apps_json`. SharedPreferences with `MODE_PRIVATE` works cross-process on Android (file-based, eventually consistent). This coupling is fine.

---

## Decision

Use **Broadcast Intents** for all IPC between the UI process and the `:vpn` process.

### Why Broadcasts (not AIDL, not Messenger, not Bound Service)

| Approach | Pros | Cons |
|---|---|---|
| **AIDL** | Type-safe, bidirectional, synchronous calls | Heavy boilerplate, connection lifecycle management, overkill for this use case |
| **Bound Service (Messenger)** | Simpler than AIDL, bidirectional | Still needs bind/unbind lifecycle, connection state tracking, handler threads |
| **Broadcast Intents** | Zero connection management, fire-and-forget, survives process death, already using Intents for start/stop | No return values (but we can broadcast back), no guaranteed delivery order |
| **ContentProvider** | Good for query/response patterns | Wrong abstraction for event streams |

**Broadcast Intents win** because:
- We already use `startService(intent)` with actions for CONNECT/DISCONNECT -- this is the same pattern.
- Status callbacks are inherently fire-and-forget events (service -> UI).
- The UI process may not exist when callbacks fire -- broadcasts handle this gracefully (just no receiver registered).
- Zero connection lifecycle to manage. No bind/unbind. No "what if the connection drops."
- `LocalBroadcastManager` is deprecated, but `Context.sendBroadcast` with explicit component or permission-protected broadcasts work fine.

### Target Architecture (Two Processes)

```
+-----------------------------------+     +-----------------------------------+
| Process: com.vpnapp (main/UI)     |     | Process: com.vpnapp:vpn           |
|                                    |     |                                   |
| +------------------+               |     | +--------------------+            |
| | VpnTurboModule   |               |     | | TunnelVpnService   |            |
| | (React Native)   |               |     | | (VpnService)       |            |
| |                  |  startService  |     | |                    |            |
| | - connect() ---- | --Intent-----> |---->| | - Go tunnel lib    |            |
| | - disconnect() - | --Intent-----> |---->| | - xray-core        |            |
| |                  |               |     | | - tun2socks        |            |
| | - getStatus() -- | --Intent-----> |---->| | - TUN fd           |            |
| |                  |               |     | |                    |            |
| | BroadcastReceiver| <--Broadcast-- |<----| | sendBroadcast()    |            |
| | - onReceive()    |  (status JSON) |     | +--------------------+            |
| +------------------+               |     |                                   |
|                                    |     | SharedPreferences (file-based)    |
| +------------------+               |     | - kill_switch_enabled             |
| | React Native     |               |     | - excluded_apps_json              |
| | JS Runtime       |               |     +-----------------------------------+
| +------------------+               |
+-----------------------------------+
```

### IPC Contract

**UI -> VPN Service (commands via Intent extras):**

| Action | Intent Action | Extras |
|---|---|---|
| Connect | `com.vpnapp.CONNECT` | `config_json: String` |
| Disconnect | `com.vpnapp.DISCONNECT` | -- |
| Get Status | `com.vpnapp.GET_STATUS` | -- (response comes via broadcast) |
| Get Traffic Stats | `com.vpnapp.GET_STATS` | -- (response comes via broadcast) |

**VPN Service -> UI (broadcasts):**

| Event | Broadcast Action | Extras |
|---|---|---|
| Status changed | `com.vpnapp.STATUS_UPDATE` | `status_json: String` |
| Traffic stats | `com.vpnapp.STATS_UPDATE` | `stats_json: String` |
| Connect result | `com.vpnapp.CONNECT_RESULT` | `success: Boolean, error: String?` |

**Settings (via SharedPreferences -- no IPC needed):**

Kill switch and excluded apps are already persisted in SharedPreferences. The service reads them on `onCreate()`. For runtime updates, send a lightweight intent:

| Action | Intent Action | Extras |
|---|---|---|
| Update kill switch | `com.vpnapp.SET_KILL_SWITCH` | `enabled: Boolean` |
| Update excluded apps | `com.vpnapp.SET_EXCLUDED_APPS` | `apps_json: String` |

---

## Implementation Plan

### Step 1: AndroidManifest.xml -- one line change

Add `android:process=":vpn"` to the service declaration:

```xml
<service
    android:name=".vpn.TunnelVpnService"
    android:process=":vpn"
    android:permission="android.permission.BIND_VPN_SERVICE"
    android:foregroundServiceType="specialUse"
    android:exported="false">
```

### Step 2: TunnelVpnService.kt -- replace static calls with broadcasts

Remove all calls to `VpnTurboModule.sendStatusEvent()` and `VpnTurboModule.resolveConnectPromise()`. Replace with:

```kotlin
// In TunnelVpnService:
private fun broadcastStatus(statusJson: String) {
    val intent = Intent("com.vpnapp.STATUS_UPDATE").apply {
        setPackage(packageName)  // restrict to own app
        putExtra("status_json", statusJson)
    }
    sendBroadcast(intent)
}

private fun broadcastConnectResult(success: Boolean, error: String? = null) {
    val intent = Intent("com.vpnapp.CONNECT_RESULT").apply {
        setPackage(packageName)
        putExtra("success", success)
        error?.let { putExtra("error", it) }
    }
    sendBroadcast(intent)
}
```

Remove the `companion object { var instance }` pattern entirely. The UI process cannot hold a reference to a service in another process.

Add new actions in `onStartCommand`:
- `ACTION_GET_STATUS` -> reads `Tunnel.getStatus()`, broadcasts it back
- `ACTION_GET_STATS` -> reads `Tunnel.getTrafficStats()`, broadcasts it back
- `ACTION_SET_KILL_SWITCH` -> calls `setKillSwitch(intent.getBooleanExtra(...))`
- `ACTION_SET_EXCLUDED_APPS` -> calls `setExcludedApps(intent.getStringExtra(...))`

### Step 3: VpnTurboModule.kt -- register BroadcastReceiver, remove static methods

Replace `companion object` static methods with a `BroadcastReceiver`:

```kotlin
private val vpnReceiver = object : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        when (intent.action) {
            "com.vpnapp.STATUS_UPDATE" -> {
                val json = intent.getStringExtra("status_json") ?: return
                emitToJS("onVpnStatusChanged", json)
                handleStatusForPromise(json)  // reject connectPromise on error
            }
            "com.vpnapp.CONNECT_RESULT" -> {
                val success = intent.getBooleanExtra("success", false)
                if (success) {
                    connectPromise?.resolve("")
                } else {
                    connectPromise?.reject("TUNNEL_ERROR",
                        intent.getStringExtra("error") ?: "Connection failed")
                }
                connectPromise = null
            }
            "com.vpnapp.STATS_UPDATE" -> {
                val json = intent.getStringExtra("stats_json") ?: return
                emitToJS("onVpnStatsUpdated", json)
            }
        }
    }
}
```

Register in `init {}`, unregister in a lifecycle callback or `onCatalystInstanceDestroy()`.

Change `getStatus()` and `getTrafficStats()`:
- Send intent to service requesting data
- Store promise, resolve when broadcast arrives

Or simpler: keep `Tunnel.probeServers()` in the UI process (it does not need the VPN service, it is just network I/O). For `getStatus()`, maintain a cached last-known-status in VpnTurboModule updated by every STATUS_UPDATE broadcast.

### Step 4: Handle `probeServers()` -- keep in UI process

`Tunnel.probeServers()` is independent of the VPN connection. Load the Go tunnel library in both processes (it is an AAR, costs ~5MB). Alternatively, call it from the service process via intent. The simpler path: load in UI process too. The Go library init is idempotent.

**Recommendation:** Load tunnel AAR in both processes. `probeServers()` stays as a direct call in VpnTurboModule. No IPC needed for this.

---

## Risks and Edge Cases

### 1. Race: UI sends CONNECT, gets killed before CONNECT_RESULT arrives
- **Impact:** Promise is lost. User reopens app, VPN is actually connected.
- **Mitigation:** On app startup, VpnTurboModule sends `GET_STATUS` to the service. The broadcast response updates the UI. The JS layer already handles "already connected" state.

### 2. Broadcast ordering
- **Risk:** STATUS_UPDATE with "connected" arrives before CONNECT_RESULT.
- **Mitigation:** Handle both. `CONNECT_RESULT` is the definitive promise resolution. `STATUS_UPDATE` is a UI update. If the promise is already resolved, ignore CONNECT_RESULT.

### 3. SharedPreferences cross-process consistency
- **Risk:** SharedPreferences file writes are not atomic cross-process. Two processes writing simultaneously can corrupt.
- **Mitigation:** Only the UI process writes preferences (kill switch, excluded apps). The VPN process only reads on `onCreate()`. For runtime changes, use intents (Step 2 actions), not SharedPreferences reads.

### 4. Go library loaded in two processes
- **Risk:** Double memory for the Go runtime (~15-20MB per process).
- **Mitigation:** Acceptable tradeoff. The whole point is to separate memory footprints. The VPN process will be lean (~20-30MB Go + tunnel). The UI process loads Go only for `probeServers()`, which is optional. Could defer to avoid loading in UI process at all.
- **Alternative:** Move `probeServers()` to the VPN service process via intent. Then UI process never loads the Go library.

### 5. `VpnService.prepare()` must be called from an Activity
- **Status:** No change needed. `prepare()` is already called in VpnTurboModule (UI process) before starting the service. This works fine regardless of which process the service runs in.

### 6. Service restart after process death
- **Status:** `START_STICKY` already ensures Android restarts the service. Since the service is in its own process, a UI process death does not affect it at all. This is the entire point.

### 7. Broadcast receiver not registered (app not running)
- **Impact:** Status broadcasts are lost. No one is listening.
- **Mitigation:** This is fine. When the user opens the app again, VpnTurboModule registers the receiver and queries current status. Broadcasts are informational, not transactional.

### 8. Intent size limits
- **Risk:** `config_json` can be large (xray config). Intent extras have a ~500KB limit (Binder transaction).
- **Mitigation:** Config JSON is typically 1-5KB. No risk. If it ever grows, write to a shared file and pass the path.

---

## Alternatives Considered

### A. Do nothing -- tune OOM priority instead
- Use `startForeground()` with higher-priority notification channel.
- Already doing this. Not sufficient when RN runtime bloats memory.
- **Rejected:** Does not solve the root cause (shared process memory).

### B. Bound service with Messenger
- Cleaner request/response pattern for `getStatus()`.
- Requires bind/unbind lifecycle management.
- If the service process dies (shouldn't with foreground), the ServiceConnection needs reconnection logic.
- **Rejected:** More complexity for marginal benefit over broadcasts.

### C. AIDL
- Full type-safe IPC.
- Massive boilerplate for 5 methods.
- **Rejected:** Overkill. We have 3 commands and 3 event types.

### D. ContentProvider as IPC bridge
- Good for query patterns (`getStatus()`, `getTrafficStats()`).
- Awkward for event streaming (status callbacks).
- Would need broadcasts anyway for events.
- **Rejected:** Mixed metaphor, adds complexity.

---

## Consequences

### Positive
- VPN survives UI process death (the whole point)
- Smaller memory footprint per process improves OOM survival
- Cleaner separation of concerns (UI vs networking)
- SharedPreferences already handle most config persistence

### Negative
- `getStatus()` and `getTrafficStats()` become asynchronous (broadcast round-trip) instead of synchronous
- Slightly more complex debugging (two processes in logcat)
- Go library memory doubled if loaded in both processes (mitigated by not loading in UI process)
- Broadcast-based promise resolution is less deterministic than direct method calls

### Neutral
- No changes to the Go tunnel library
- No changes to the React Native JS layer (same events, same methods)
- `probeServers()` decision (UI vs VPN process) can be deferred

---

## Implementation Tasks (for developer)

1. [ ] Add `android:process=":vpn"` to AndroidManifest.xml service declaration
2. [ ] TunnelVpnService: Replace `VpnTurboModule.sendStatusEvent()` calls with `broadcastStatus()`
3. [ ] TunnelVpnService: Replace `VpnTurboModule.resolveConnectPromise()` with `broadcastConnectResult(true)`
4. [ ] TunnelVpnService: Remove `companion object { instance }` pattern
5. [ ] TunnelVpnService: Add `onStartCommand` handlers for GET_STATUS, GET_STATS, SET_KILL_SWITCH, SET_EXCLUDED_APPS
6. [ ] VpnTurboModule: Register BroadcastReceiver for STATUS_UPDATE, CONNECT_RESULT, STATS_UPDATE
7. [ ] VpnTurboModule: Remove `sendStatusEvent()`, `resolveConnectPromise()`, `sendStatsEvent()` static methods
8. [ ] VpnTurboModule: Change `getStatus()` to send intent + resolve from broadcast (or use cached status)
9. [ ] VpnTurboModule: Change `getTrafficStats()` to send intent + resolve from broadcast
10. [ ] VpnTurboModule: Change `setKillSwitch()` and `setExcludedApps()` to send intents instead of direct method calls
11. [ ] VpnTurboModule: Unregister receiver in `onCatalystInstanceDestroy()`
12. [ ] Test: Connect VPN, switch to heavy app, verify VPN survives
13. [ ] Test: Connect VPN, force-stop app from settings, verify VPN stays connected
14. [ ] Test: Open app with VPN already connected (service in :vpn), verify UI shows correct state
15. [ ] Test: Kill switch behavior across process boundary
