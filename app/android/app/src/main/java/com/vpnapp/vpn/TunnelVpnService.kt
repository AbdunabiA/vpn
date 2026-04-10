package com.vpnapp.vpn

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.Context
import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.os.ParcelFileDescriptor
import android.util.Log
import org.json.JSONObject
import tunnel.Tunnel
import tunnel.ProtectSocket
import tunnel.StatusCallback

/**
 * Android VpnService that manages the VPN tunnel via the Go tunnel library.
 *
 * Runs in the ":vpn" process (declared in AndroidManifest). This means a
 * native crash inside gvisor/tun2socks only kills the VPN process — the main
 * app process stays alive and receives a final broadcast before the process dies.
 *
 * Inter-process communication:
 *   Main -> VPN : explicit Intents delivered to onStartCommand (ACTION_CONNECT,
 *                 ACTION_DISCONNECT, ACTION_SET_KILL_SWITCH, ACTION_SET_EXCLUDED_APPS)
 *   VPN -> Main : sendBroadcast with setPackage(packageName) so only our app
 *                 receives the broadcast, no LocalBroadcastManager needed cross-process.
 *
 * Flow:
 *   React Native -> VpnTurboModule -> Intent -> TunnelVpnService -> Go tunnel (xray-core)
 *
 * The Go tunnel opens a local SOCKS5 proxy on localhost:10808.
 * tun2socks bridges the TUN interface to that SOCKS5 proxy.
 *
 * Kill switch behaviour:
 *   When killSwitchEnabled is true and the tunnel drops unexpectedly the TUN
 *   interface is kept alive with its routes intact.  Nothing reads from it so
 *   all packets are blackholed — no traffic can leak outside the tunnel.
 *   A manual disconnect (ACTION_DISCONNECT) always tears down everything.
 */
class TunnelVpnService : VpnService(), ProtectSocket, StatusCallback {

    private var vpnInterface: ParcelFileDescriptor? = null
    @Volatile private var isRunning = false
    @Volatile private var isStarting = false

    private val debugExecutor = java.util.concurrent.Executors.newSingleThreadExecutor()

    // True while the TUN interface is alive but the tunnel backend has stopped.
    // Traffic is effectively blackholed in this state.
    private var killSwitchActive = false

    // Loaded from SharedPreferences on start; can be updated at runtime via
    // ACTION_SET_KILL_SWITCH Intent sent from the main process.
    private var killSwitchEnabled = false

    // Package names excluded from the VPN (split tunneling).
    // Loaded from SharedPreferences on start; updated via ACTION_SET_EXCLUDED_APPS.
    private var excludedApps: List<String> = emptyList()

    // Traffic stats — read from /proc/net/dev for the TUN interface created by
    // VpnService.Builder.establish(). The TUN device sees DECRYPTED user data
    // before xray-core wraps it in VLESS/REALITY, so the byte counts reflect
    // exactly what the user's apps consumed/transmitted — no padding overhead,
    // no other-app traffic, no encryption inflation.
    //
    // We capture a baseline on connect so broadcast values represent only the
    // current session, not the lifetime kernel counters.
    private val statsHandler = android.os.Handler(android.os.Looper.getMainLooper())
    private var statsTunInterface: String? = null
    private var statsBaselineRx: Long = 0
    private var statsBaselineTx: Long = 0
    private var statsLastRx: Long = 0
    private var statsLastTx: Long = 0
    private var statsLastTime: Long = 0
    private val statsRunnable = object : Runnable {
        override fun run() {
            if (isRunning) {
                broadcastTrafficStats()
                statsHandler.postDelayed(this, STATS_INTERVAL_MS)
            }
        }
    }

    companion object {
        private const val TAG = "TunnelVpnService"
        private const val CHANNEL_ID = "vpn_channel"
        private const val NOTIFICATION_ID = 1

        // Intent actions for commands FROM the main process
        const val ACTION_CONNECT = "com.vpnapp.CONNECT"
        const val ACTION_DISCONNECT = "com.vpnapp.DISCONNECT"
        const val ACTION_SET_KILL_SWITCH = "com.vpnapp.SET_KILL_SWITCH"
        const val ACTION_SET_EXCLUDED_APPS = "com.vpnapp.SET_EXCLUDED_APPS"

        // Intent extras
        const val EXTRA_CONFIG_JSON = "config_json"
        const val EXTRA_KILL_SWITCH_ENABLED = "enabled"
        const val EXTRA_APPS_JSON = "apps_json"

        // Broadcast actions TO the main process
        const val BROADCAST_VPN_STATUS = "com.vpnapp.VPN_STATUS"
        const val BROADCAST_VPN_CONNECT_RESULT = "com.vpnapp.VPN_CONNECT_RESULT"
        const val BROADCAST_VPN_STATS = "com.vpnapp.VPN_STATS"
        const val EXTRA_STATUS_JSON = "status_json"
        const val EXTRA_SUCCESS = "success"
        const val EXTRA_STATS_JSON = "stats_json"

        // How often to broadcast traffic stats while the tunnel is up.
        private const val STATS_INTERVAL_MS = 1000L

        // SharedPreferences
        const val PREFS_NAME = "vpn_prefs"
        private const val PREF_KILL_SWITCH = "kill_switch_enabled"
        const val PREF_EXCLUDED_APPS = "excluded_apps_json"
    }

    // --- ProtectSocket interface (gomobile generated) ---
    // Prevents the tunnel's own sockets from being routed through the VPN.
    override fun protect(fd: Long): Boolean {
        return (this as VpnService).protect(fd.toInt())
    }

    // --- StatusCallback interface (gomobile generated) ---
    // Receives tunnel state changes from the Go library.
    override fun onStatusChanged(statusJSON: String?) {
        statusJSON?.let { json ->
            sendDebugEvent("native_status_callback", json)
            try {
                val state = JSONObject(json).optString("state", "")
                val error = JSONObject(json).optString("error", "")
                when (state) {
                    "connected" -> {
                        // Tunnel (re)connected — clear any active kill switch state
                        if (killSwitchActive) {
                            killSwitchActive = false
                            Log.i(TAG, "Kill switch deactivated — tunnel reconnected")
                        }
                        updateNotification("Connected")
                    }
                    "connecting" -> updateNotification("Connecting...")
                    "disconnecting" -> {
                        sendDebugEvent("status_disconnecting", "isRunning=$isRunning killSwitchActive=$killSwitchActive")
                        updateNotification("Disconnecting...")
                    }
                    "disconnected" -> {
                        sendDebugEvent("status_disconnected", "isRunning=$isRunning killSwitchActive=$killSwitchActive")
                        updateNotification("Disconnected")
                    }
                    "error" -> {
                        sendDebugEvent("status_error", "error=$error isRunning=$isRunning killSwitchEnabled=$killSwitchEnabled")
                        // Unexpected tunnel error from xray-core. If kill switch is
                        // enabled engage it now; otherwise just forward the event.
                        if (killSwitchEnabled && isRunning) {
                            Log.w(TAG, "Tunnel error detected — engaging kill switch")
                            stopVpn(isManual = false)
                            return // stopVpn sends its own status broadcast
                        }
                        updateNotification("Error")
                    }
                }
            } catch (_: Exception) { }
            broadcastStatus(json)
        }
    }

    override fun onCreate() {
        super.onCreate()
        createNotificationChannel()
        val prefs = getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
        // Restore persisted kill switch preference
        killSwitchEnabled = prefs.getBoolean(PREF_KILL_SWITCH, false)
        // Restore persisted excluded-apps list
        val savedApps = prefs.getString(PREF_EXCLUDED_APPS, null)
        if (!savedApps.isNullOrEmpty()) {
            try {
                val arr = org.json.JSONArray(savedApps)
                excludedApps = (0 until arr.length()).map { arr.getString(it) }
            } catch (_: Exception) { }
        }
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        sendDebugEvent("onStartCommand", "action=${intent?.action} isRunning=$isRunning")
        when (intent?.action) {
            ACTION_CONNECT -> {
                val configJson = intent.getStringExtra(EXTRA_CONFIG_JSON) ?: ""
                startVpn(configJson)
            }
            ACTION_DISCONNECT -> {
                sendDebugEvent("onStartCommand_disconnect", "manual disconnect requested")
                stopVpn(isManual = true)
            }
            ACTION_SET_KILL_SWITCH -> {
                val enabled = intent.getBooleanExtra(EXTRA_KILL_SWITCH_ENABLED, false)
                applyKillSwitchSetting(enabled)
            }
            ACTION_SET_EXCLUDED_APPS -> {
                val appsJson = intent.getStringExtra(EXTRA_APPS_JSON) ?: "[]"
                applyExcludedApps(appsJson)
            }
        }
        return START_NOT_STICKY
    }

    override fun onTaskRemoved(rootIntent: Intent?) {
        sendDebugEvent("onTaskRemoved", "isRunning=$isRunning — app swiped from recents, service continues")
        super.onTaskRemoved(rootIntent)
    }

    override fun onRevoke() {
        sendDebugEvent("onRevoke", "isRunning=$isRunning — VPN revoked by system/another app")
        stopVpn(isManual = false)
    }

    override fun onDestroy() {
        sendDebugEvent("onDestroy", "isRunning=$isRunning killSwitchActive=$killSwitchActive")
        // System-initiated destroy: honour kill switch (isManual = false).
        // If the service was already stopped cleanly this is a no-op.
        stopVpn(isManual = false)
        super.onDestroy()
    }

    // MARK: - Settings applied from main process

    /**
     * Enable or disable the kill switch. The preference is persisted in
     * SharedPreferences so it survives service restarts.
     * Called when ACTION_SET_KILL_SWITCH arrives from the main process.
     */
    private fun applyKillSwitchSetting(enabled: Boolean) {
        killSwitchEnabled = enabled
        getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
            .edit()
            .putBoolean(PREF_KILL_SWITCH, enabled)
            .apply()
        Log.i(TAG, "Kill switch ${if (enabled) "enabled" else "disabled"}")
    }

    /**
     * Updates the set of apps that bypass the VPN tunnel.
     * If the tunnel is currently running the TUN interface is rebuilt so the
     * new exclusion list takes effect without a full VPN reconnect.
     * Called when ACTION_SET_EXCLUDED_APPS arrives from the main process.
     */
    private fun applyExcludedApps(appsJson: String) {
        try {
            val arr = org.json.JSONArray(appsJson)
            excludedApps = (0 until arr.length()).map { arr.getString(it) }
            // Persist so we survive a service restart
            getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
                .edit()
                .putString(PREF_EXCLUDED_APPS, appsJson)
                .apply()
            if (isRunning) {
                rebuildTunInterface()
            }
        } catch (e: Exception) {
            Log.e(TAG, "applyExcludedApps parse error: ${e.message}")
        }
    }

    // MARK: - Broadcast helpers (VPN process -> Main process)

    /**
     * Send a VPN status update to the main process.
     * Uses sendBroadcast with setPackage so only our own app receives it.
     */
    private fun broadcastStatus(statusJson: String) {
        val intent = Intent(BROADCAST_VPN_STATUS).apply {
            setPackage(packageName)
            putExtra(EXTRA_STATUS_JSON, statusJson)
        }
        sendBroadcast(intent)
    }

    /**
     * Notify the main process that the tunnel is fully connected and the JS
     * connect() promise can be resolved.
     */
    private fun broadcastConnectResult(success: Boolean) {
        val intent = Intent(BROADCAST_VPN_CONNECT_RESULT).apply {
            setPackage(packageName)
            putExtra(EXTRA_SUCCESS, success)
        }
        sendBroadcast(intent)
    }

    // MARK: - Traffic stats polling

    /**
     * Find the TUN interface name created by VpnService.Builder.establish() by
     * scanning network interfaces for one bound to our 10.0.0.2 tunnel address.
     * Falls back to the first tun-prefixed or vpn-prefixed interface in /proc/net/dev.
     */
    private fun detectTunInterface(): String? {
        try {
            val interfaces = java.net.NetworkInterface.getNetworkInterfaces() ?: return null
            for (iface in interfaces) {
                val name = iface.name ?: continue
                if (!name.startsWith("tun") && !name.startsWith("vpn")) continue
                val addrs = iface.interfaceAddresses ?: continue
                for (addr in addrs) {
                    val host = addr.address?.hostAddress ?: continue
                    if (host == "10.0.0.2") return name
                }
            }
            // Fallback: any tun-prefixed or vpn-prefixed interface present in the kernel.
            val all = java.net.NetworkInterface.getNetworkInterfaces() ?: return null
            for (iface in all) {
                val name = iface.name ?: continue
                if (name.startsWith("tun") || name.startsWith("vpn")) return name
            }
        } catch (e: Throwable) {
            Log.w(TAG, "detectTunInterface error: ${e.message}")
        }
        return null
    }

    /**
     * Read /proc/net/dev and return (rxBytes, txBytes) for the given interface.
     *
     * The /proc/net/dev format is two header lines followed by one row per
     * interface: "ifname: rxBytes rxPackets ... (16 columns total) txBytes ..."
     * The 1st numeric column is rx_bytes; the 9th is tx_bytes.
     *
     * For a TUN device managed by VpnService, the kernel convention is:
     *   - tun.RX = bytes the kernel received from userspace via the fd
     *              (i.e. tun2socks WROTE these — decrypted server responses
     *              that the kernel will route to the requesting app).
     *              => This is the user-perceived DOWNLOAD.
     *   - tun.TX = bytes the kernel transmitted to userspace via the fd
     *              (i.e. outgoing app packets queued for tun2socks to read).
     *              => This is the user-perceived UPLOAD.
     */
    private fun readInterfaceStats(iface: String): Pair<Long, Long>? {
        try {
            val file = java.io.File("/proc/net/dev")
            if (!file.canRead()) return null
            val prefix = "$iface:"
            file.bufferedReader().use { reader ->
                while (true) {
                    val line = reader.readLine() ?: return null
                    val trimmed = line.trimStart()
                    if (!trimmed.startsWith(prefix)) continue
                    val data = trimmed.substring(prefix.length).trim()
                    val parts = data.split("\\s+".toRegex())
                    if (parts.size < 16) return null
                    val rx = parts[0].toLongOrNull() ?: return null
                    val tx = parts[8].toLongOrNull() ?: return null
                    return Pair(rx, tx)
                }
            }
        } catch (e: Throwable) {
            Log.w(TAG, "readInterfaceStats($iface) error: ${e.message}")
        }
        @Suppress("UNREACHABLE_CODE")
        return null
    }

    /**
     * Start the 1-second traffic stats poller. Captures the current per-TUN
     * byte counters as the session baseline so subsequent broadcasts report
     * only what was consumed during this VPN session.
     */
    private fun startStatsPolling() {
        // Detect TUN interface name now — VpnService.Builder.establish()
        // has already brought it up.
        statsTunInterface = detectTunInterface()
        Log.i(TAG, "stats: tun interface = ${statsTunInterface ?: "<none>"}")

        val initial = statsTunInterface?.let { readInterfaceStats(it) }
        if (initial != null) {
            statsBaselineRx = initial.first
            statsBaselineTx = initial.second
            statsLastRx = initial.first
            statsLastTx = initial.second
        } else {
            statsBaselineRx = 0
            statsBaselineTx = 0
            statsLastRx = 0
            statsLastTx = 0
        }
        statsLastTime = android.os.SystemClock.elapsedRealtime()
        statsHandler.removeCallbacks(statsRunnable)
        statsHandler.postDelayed(statsRunnable, STATS_INTERVAL_MS)
    }

    /** Stop the stats poller. Idempotent. */
    private fun stopStatsPolling() {
        statsHandler.removeCallbacks(statsRunnable)
        statsTunInterface = null
    }

    /**
     * Read current TUN interface byte counters and broadcast a TrafficStats payload.
     * Session bytes = current - baseline.
     * Speed = (current - last_tick) / seconds_since_last_tick.
     */
    private fun broadcastTrafficStats() {
        try {
            val iface = statsTunInterface ?: detectTunInterface()?.also { statsTunInterface = it }
            if (iface == null) return

            val current = readInterfaceStats(iface) ?: return
            val rx = current.first
            val tx = current.second
            val now = android.os.SystemClock.elapsedRealtime()

            val sessionRx = (rx - statsBaselineRx).coerceAtLeast(0)
            val sessionTx = (tx - statsBaselineTx).coerceAtLeast(0)

            // Guard against zero-divide and the kernel resetting counters when
            // the TUN device gets recreated (rebuildTunInterface for split tunnel).
            val deltaMs = (now - statsLastTime).coerceAtLeast(1)
            val deltaSec = deltaMs / 1000.0
            val speedDownBps = ((rx - statsLastRx).coerceAtLeast(0)).toDouble() / deltaSec
            val speedUpBps = ((tx - statsLastTx).coerceAtLeast(0)).toDouble() / deltaSec

            statsLastRx = rx
            statsLastTx = tx
            statsLastTime = now

            val json = JSONObject().apply {
                put("bytes_up", sessionTx)
                put("bytes_down", sessionRx)
                put("speed_up_bps", speedUpBps)
                put("speed_down_bps", speedDownBps)
            }.toString()

            val intent = Intent(BROADCAST_VPN_STATS).apply {
                setPackage(packageName)
                putExtra(EXTRA_STATS_JSON, json)
            }
            sendBroadcast(intent)
        } catch (e: Throwable) {
            Log.w(TAG, "broadcastTrafficStats error: ${e.message}")
        }
    }

    // MARK: - VPN lifecycle

    /**
     * Starts the VPN tunnel:
     * 1. Register socket protection (prevents routing loop)
     * 2. Start Go tunnel (xray-core SOCKS5 proxy on localhost:10808)
     * 3. Create TUN interface
     * 4. Start tun2socks (bridges TUN <-> SOCKS5)
     */
    private fun startVpn(configJson: String) {
        if (isRunning || isStarting) {
            sendDebugEvent("startVpn_skipped", "isRunning=$isRunning isStarting=$isStarting")
            return
        }

        // Check Go tunnel state — if already connected, skip.
        // This prevents the race where JS fires a second connect
        // before the first startVpn finishes setting isRunning=true.
        try {
            val goStatus = Tunnel.getStatus()
            val goState = JSONObject(goStatus).optString("state", "")
            if (goState == "connected" || goState == "connecting") {
                sendDebugEvent("startVpn_skipped_go_state", "goState=$goState")
                return
            }
        } catch (_: Throwable) { }

        isStarting = true

        // If the kill switch was active (TUN alive, tunnel down), close the
        // stale TUN fd before establishing a fresh one for the new connection.
        if (killSwitchActive) {
            Log.i(TAG, "Clearing kill switch state before reconnect")
            try {
                vpnInterface?.close()
                vpnInterface = null
            } catch (e: Exception) {
                Log.e(TAG, "Error closing stale VPN interface during reconnect", e)
            }
            killSwitchActive = false
        }

        Log.i(TAG, "Starting VPN tunnel, config length=${configJson.length}")
        writeCrashBreadcrumb("startVpn entered, config length=${configJson.length}")
        startForeground(NOTIFICATION_ID, buildNotification("Connecting..."))

        try {
            // 1. Register socket protection BEFORE connecting
            writeCrashBreadcrumb("step1: setProtectCallback")
            Tunnel.setProtectCallback(this)

            // 2. Register status callback
            writeCrashBreadcrumb("step2: setStatusCallback")
            Tunnel.setStatusCallback(this)

            // 3. Start the Go tunnel (xray-core SOCKS5 proxy)
            writeCrashBreadcrumb("step3: calling Tunnel.connect()")
            val connectResult = Tunnel.connect(configJson)
            writeCrashBreadcrumb("step4: Tunnel.connect() returned: '$connectResult'")
            if (connectResult.isNotEmpty()) {
                Log.e(TAG, "Tunnel connect error: $connectResult")
                broadcastStatus(JSONObject().put("state", "error").put("error", connectResult).toString())
                broadcastConnectResult(success = false)
                isStarting = false
                stopForeground(STOP_FOREGROUND_REMOVE)
                stopSelf()
                return
            }

            // 4. Create TUN interface AFTER xray starts (so its sockets are already protected)
            writeCrashBreadcrumb("step5: building TUN interface")
            val builder = Builder()
                .setSession("VPN App")
                .addAddress("10.0.0.2", 32)           // IPv4 tunnel address
                .addRoute("0.0.0.0", 0)               // Route all IPv4
                .addAddress("fd00::2", 128)            // IPv6 tunnel address
                .addRoute("::", 0)                     // Route all IPv6 (prevents IPv6 leak)
                .addDnsServer("1.1.1.1")
                .addDnsServer("8.8.8.8")
                .setMtu(1400)
                // setBlocking(true) makes the TUN read block in the kernel. When kill
                // switch is active and nothing is reading from the fd, packets stall in
                // the kernel buffer instead of being dropped immediately, which is the
                // safer blackhole behaviour we want.
                .setBlocking(killSwitchEnabled)

            // Exclude our own app so API calls to vpnapi.mydayai.uz bypass the tunnel.
            // Without this, the app's HTTP traffic gets routed through xray, which
            // breaks connectivity to the API server (same host as the VPN endpoint).
            try {
                builder.addDisallowedApplication(packageName)
            } catch (e: Exception) {
                Log.w(TAG, "Failed to exclude own package from VPN", e)
            }

            // Apply split tunneling exclusions — these apps will bypass the VPN.
            for (pkg in excludedApps) {
                try {
                    builder.addDisallowedApplication(pkg)
                } catch (e: Exception) {
                    Log.w(TAG, "Skipping unknown package for split tunnel exclusion: $pkg")
                }
            }

            writeCrashBreadcrumb("step6: calling builder.establish()")
            vpnInterface = builder.establish()
            writeCrashBreadcrumb("step7: establish() returned, fd=${vpnInterface?.fd}")

            if (vpnInterface == null) {
                Log.e(TAG, "Failed to establish VPN interface")
                Tunnel.disconnect()
                broadcastConnectResult(success = false)
                isStarting = false
                stopForeground(STOP_FOREGROUND_REMOVE)
                stopSelf()
                return
            }

            // 5. Start tun2socks: pipes TUN packets to SOCKS5 proxy
            val tunFd = vpnInterface!!.fd
            writeCrashBreadcrumb("step8: calling Tunnel.startTun(fd=$tunFd)")
            val tunResult = Tunnel.startTun(tunFd.toLong())
            writeCrashBreadcrumb("step9: startTun returned: '$tunResult'")
            if (tunResult.isNotEmpty()) {
                Log.e(TAG, "tun2socks error: $tunResult")
                Tunnel.disconnect()
                vpnInterface?.close()
                vpnInterface = null
                broadcastConnectResult(success = false)
                isStarting = false
                stopForeground(STOP_FOREGROUND_REMOVE)
                stopSelf()
                return
            }

            isRunning = true
            isStarting = false
            writeCrashBreadcrumb("step10: VPN FULLY CONNECTED")
            updateNotification("Connected")
            // Begin broadcasting traffic stats every second so the UI counter
            // updates while the tunnel is up.
            startStatsPolling()
            // Notify the main process that TUN + tun2socks are fully ready so the
            // JS connect() promise can be resolved.
            broadcastConnectResult(success = true)
            Log.i(TAG, "VPN tunnel established")

        } catch (e: Throwable) {
            isStarting = false
            Log.e(TAG, "Failed to start VPN: ${e.javaClass.name}: ${e.message}", e)
            try {
                broadcastStatus(
                    JSONObject().put("state", "error").put("error", "${e.javaClass.name}: ${e.message}").toString()
                )
                broadcastConnectResult(success = false)
            } catch (_: Throwable) { }
            stopForeground(STOP_FOREGROUND_REMOVE)
            stopSelf()
        }
    }

    /**
     * Stops the VPN tunnel.
     *
     * @param isManual true when triggered by an explicit user disconnect action.
     *                 false when triggered by a system event / unexpected crash.
     *
     * Kill switch behaviour (isManual = false, killSwitchEnabled = true):
     *   - Stop tun2socks and xray-core so no traffic can reach the outside world.
     *   - Keep the TUN interface and its routes alive.  No process reads from the
     *     fd, so all packets are blackholed inside the kernel buffer.
     *   - The service stays in the foreground so Android does not reclaim it.
     *
     * Manual disconnect or kill switch disabled:
     *   - Tear down everything: tun2socks -> xray -> TUN -> stopSelf().
     */
    private fun stopVpn(isManual: Boolean = false) {
        sendDebugEvent("stopVpn_called", "isManual=$isManual isRunning=$isRunning killSwitchActive=$killSwitchActive stackTrace=${Thread.currentThread().stackTrace.take(8).joinToString(" <- ") { "${it.className}.${it.methodName}:${it.lineNumber}" }}")

        // For manual disconnect, always attempt full cleanup even if isRunning
        // is false — the Go tunnel or TUN fd may still be alive.
        if (!isManual && !isRunning && !killSwitchActive) return

        // Stop stats polling immediately so the JS UI sees the counter freeze
        // (and reset to 0 once the disconnected status broadcast lands).
        stopStatsPolling()

        val applyKillSwitch = killSwitchEnabled && !isManual

        Log.i(TAG, "Stopping VPN tunnel (isManual=$isManual, killSwitch=$applyKillSwitch)")

        // Always stop the tunnel backend — no traffic must pass through xray.
        // Wrapped in try/catch because tun2socks engine.Stop() can panic
        // (via logrus.Fatalf -> our panic handler).
        try {
            Tunnel.stopTun()
        } catch (e: Throwable) {
            Log.e(TAG, "stopTun error: ${e.message}")
        }
        try {
            Tunnel.disconnect()
        } catch (e: Throwable) {
            Log.e(TAG, "disconnect error: ${e.message}")
        }

        if (applyKillSwitch) {
            // Keep TUN alive to blackhole all traffic. The fd remains open and
            // the kernel routes are still in place, but nothing reads from the fd.
            isRunning = false
            killSwitchActive = true
            broadcastStatus(
                """{"state":"kill_switch","server_addr":"","protocol":"","connected_at":0,"bytes_up":0,"bytes_down":0}"""
            )
            updateNotification("Kill Switch Active")
            Log.i(TAG, "Kill switch engaged — TUN kept alive, traffic blackholed")
            // Do NOT call stopSelf() — the service must stay alive to hold the TUN fd.
        } else {
            // Full teardown
            try {
                vpnInterface?.close()
                vpnInterface = null
            } catch (e: Exception) {
                Log.e(TAG, "Error closing VPN interface", e)
            }

            isRunning = false
            killSwitchActive = false
            broadcastStatus(
                """{"state":"disconnected","server_addr":"","protocol":"","connected_at":0,"bytes_up":0,"bytes_down":0}"""
            )

            stopForeground(STOP_FOREGROUND_REMOVE)
            stopSelf()
        }
    }

    /**
     * Rebuilds the TUN interface with the current excludedApps list while the
     * tunnel backend (xray + tun2socks) keeps running.
     *
     * Steps:
     *  1. Stop tun2socks (releases the old TUN fd reference inside Go).
     *  2. Close the old TUN interface.
     *  3. Create a new TUN interface with the updated exclusion list.
     *  4. Re-attach tun2socks to the new fd.
     */
    private fun rebuildTunInterface() {
        if (!isRunning) return
        Log.i(TAG, "Rebuilding TUN interface for updated split tunnel exclusions")

        try {
            // 1. Detach tun2socks from the current TUN fd
            Tunnel.stopTun()

            // 2. Close the old TUN interface
            try {
                vpnInterface?.close()
                vpnInterface = null
            } catch (e: Exception) {
                Log.e(TAG, "Error closing old VPN interface during rebuild", e)
            }

            // 3. Build a new TUN interface with updated exclusions
            val builder = Builder()
                .setSession("VPN App")
                .addAddress("10.0.0.2", 32)
                .addRoute("0.0.0.0", 0)
                .addAddress("fd00::2", 128)
                .addRoute("::", 0)
                .addDnsServer("1.1.1.1")
                .addDnsServer("8.8.8.8")
                .setMtu(1400)
                .setBlocking(killSwitchEnabled)

            // Exclude own app from VPN (same as in startVpn)
            try {
                builder.addDisallowedApplication(packageName)
            } catch (_: Exception) { }

            for (pkg in excludedApps) {
                try {
                    builder.addDisallowedApplication(pkg)
                } catch (e: Exception) {
                    Log.w(TAG, "Skipping unknown package during TUN rebuild: $pkg")
                }
            }

            val newInterface = builder.establish()
            if (newInterface == null) {
                Log.e(TAG, "Failed to establish new TUN interface during rebuild")
                // Fallback: perform a full stop so we don't leave things in a broken state
                stopVpn(isManual = false)
                return
            }

            vpnInterface = newInterface

            // 4. Re-attach tun2socks to the new TUN fd
            val tunResult = Tunnel.startTun(newInterface.fd.toLong())
            if (tunResult.isNotEmpty()) {
                Log.e(TAG, "tun2socks re-attach error: $tunResult")
                stopVpn(isManual = false)
                return
            }

            // 5. Re-baseline traffic stats — the new TUN device starts fresh
            //    counters and may even have a new interface name. Without this
            //    the next stats tick would compute a huge negative delta which
            //    coerceAtLeast(0) would clamp to 0 indefinitely.
            statsTunInterface = detectTunInterface()
            val rebased = statsTunInterface?.let { readInterfaceStats(it) }
            if (rebased != null) {
                statsBaselineRx = rebased.first - (statsLastRx - statsBaselineRx)
                statsBaselineTx = rebased.second - (statsLastTx - statsBaselineTx)
                statsLastRx = rebased.first
                statsLastTx = rebased.second
            }

            Log.i(TAG, "TUN interface rebuilt successfully with ${excludedApps.size} excluded apps")
        } catch (e: Exception) {
            Log.e(TAG, "Failed to rebuild TUN interface", e)
            stopVpn(isManual = false)
        }
    }

    // MARK: - Notifications

    private fun createNotificationChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val channel = NotificationChannel(
                CHANNEL_ID,
                "VPN Service",
                NotificationManager.IMPORTANCE_DEFAULT
            ).apply {
                description = "Shows VPN connection status"
                setShowBadge(false)
            }
            val manager = getSystemService(NotificationManager::class.java)
            manager.createNotificationChannel(channel)
        }
    }

    private fun buildNotification(status: String): Notification {
        return if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            Notification.Builder(this, CHANNEL_ID)
                .setContentTitle("VPN App")
                .setContentText(status)
                .setSmallIcon(android.R.drawable.ic_lock_lock)
                .setOngoing(true)
                .build()
        } else {
            @Suppress("DEPRECATION")
            Notification.Builder(this)
                .setContentTitle("VPN App")
                .setContentText(status)
                .setSmallIcon(android.R.drawable.ic_lock_lock)
                .setOngoing(true)
                .build()
        }
    }

    private fun updateNotification(status: String) {
        val manager = getSystemService(NotificationManager::class.java)
        manager.notify(NOTIFICATION_ID, buildNotification(status))
    }

    // MARK: - Diagnostics

    /**
     * Write a breadcrumb to a file so we can see the last step before a native crash.
     * The file is in filesDir which is shared across processes within the same app,
     * so the main process can read it on next startup.
     */
    private fun writeCrashBreadcrumb(msg: String) {
        Log.i(TAG, "BREADCRUMB: $msg")
        try {
            val file = java.io.File(filesDir, "crash_breadcrumb.txt")
            if (file.length() > 50_000) file.writeText("")
            file.appendText("${System.currentTimeMillis()} $msg\n")
        } catch (_: Throwable) { }
    }

    /**
     * Send a debug event to the API in real-time (fire-and-forget).
     * HTTP calls work fine from the VPN process since the tunnel's own sockets
     * are protected via the ProtectSocket callback.
     */
    private fun sendDebugEvent(action: String, detail: String) {
        Log.i(TAG, "DEBUG_EVENT: $action — $detail")
        debugExecutor.execute {
            try {
                val body = org.json.JSONObject().apply {
                    put("error", detail)
                    put("action", action)
                }
                val url = java.net.URL("https://vpnapi.mydayai.uz:9443/api/v1/debug/error")
                val conn = url.openConnection() as java.net.HttpURLConnection
                conn.requestMethod = "POST"
                conn.setRequestProperty("Content-Type", "application/json")
                conn.connectTimeout = 3000
                conn.readTimeout = 3000
                conn.doOutput = true
                conn.outputStream.write(body.toString().toByteArray())
                conn.responseCode
                conn.disconnect()
            } catch (_: Throwable) { }
        }
    }
}
