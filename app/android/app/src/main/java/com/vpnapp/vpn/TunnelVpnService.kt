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
 * Flow:
 *   React Native -> VpnTurboModule -> TunnelVpnService -> Go tunnel (xray-core)
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
    private var isRunning = false

    // True while the TUN interface is alive but the tunnel backend has stopped.
    // Traffic is effectively blackholed in this state.
    private var killSwitchActive = false

    // Loaded from SharedPreferences on start; can be updated at runtime via
    // setKillSwitch() called from VpnTurboModule.
    private var killSwitchEnabled = false

    // Package names excluded from the VPN (split tunneling).
    // Loaded from SharedPreferences on start; updated via setExcludedApps().
    private var excludedApps: List<String> = emptyList()

    companion object {
        private const val TAG = "TunnelVpnService"
        private const val CHANNEL_ID = "vpn_channel"
        private const val NOTIFICATION_ID = 1
        const val ACTION_CONNECT = "com.vpnapp.CONNECT"
        const val ACTION_DISCONNECT = "com.vpnapp.DISCONNECT"
        const val EXTRA_CONFIG_JSON = "config_json"

        const val PREFS_NAME = "vpn_prefs"
        private const val PREF_KILL_SWITCH = "kill_switch_enabled"
        const val PREF_EXCLUDED_APPS = "excluded_apps_json"

        // Singleton reference for the TurboModule to interact with
        var instance: TunnelVpnService? = null
            private set
    }

    /**
     * Updates the set of apps that bypass the VPN tunnel.
     * If the tunnel is currently running the TUN interface is rebuilt so the
     * new exclusion list takes effect without a full VPN reconnect.
     */
    fun setExcludedApps(apps: List<String>) {
        excludedApps = apps
        if (isRunning) {
            rebuildTunInterface()
        }
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
            // Update notification based on parsed state
            try {
                val state = JSONObject(json).optString("state", "")
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
                    "disconnecting" -> updateNotification("Disconnecting...")
                    "error" -> {
                        // Unexpected tunnel error from xray-core. If kill switch is
                        // enabled engage it now; otherwise just forward the event.
                        if (killSwitchEnabled && isRunning) {
                            Log.w(TAG, "Tunnel error detected — engaging kill switch")
                            stopVpn(isManual = false)
                            return // stopVpn sends its own status event
                        }
                        updateNotification("Error")
                    }
                }
            } catch (_: Exception) { }
            VpnTurboModule.sendStatusEvent(json)
        }
    }

    override fun onCreate() {
        super.onCreate()
        instance = this
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
        when (intent?.action) {
            ACTION_CONNECT -> {
                val configJson = intent.getStringExtra(EXTRA_CONFIG_JSON) ?: ""
                startVpn(configJson)
            }
            ACTION_DISCONNECT -> {
                // Manual disconnect — always tear down everything regardless of kill switch
                stopVpn(isManual = true)
            }
        }
        return START_STICKY
    }

    override fun onDestroy() {
        // System-initiated destroy: honour kill switch (isManual = false).
        // If the service was already stopped cleanly this is a no-op.
        stopVpn(isManual = false)
        instance = null
        super.onDestroy()
    }

    // MARK: - Kill Switch API

    /**
     * Enable or disable the kill switch. The preference is persisted in
     * SharedPreferences so it survives service restarts.
     */
    fun setKillSwitch(enabled: Boolean) {
        killSwitchEnabled = enabled
        getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
            .edit()
            .putBoolean(PREF_KILL_SWITCH, enabled)
            .apply()
        Log.i(TAG, "Kill switch ${if (enabled) "enabled" else "disabled"}")
    }

    /**
     * Returns true when the TUN interface is alive but the tunnel backend has
     * stopped — i.e. the kill switch is actively blackholing traffic.
     */
    fun isKillSwitchActive(): Boolean = killSwitchActive

    /**
     * Starts the VPN tunnel:
     * 1. Register socket protection (prevents routing loop)
     * 2. Start Go tunnel (xray-core SOCKS5 proxy on localhost:10808)
     * 3. Create TUN interface
     * 4. Start tun2socks (bridges TUN <-> SOCKS5)
     */
    private fun startVpn(configJson: String) {
        if (isRunning) return

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

        Log.i(TAG, "Starting VPN tunnel")
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
                VpnTurboModule.sendStatusEvent(JSONObject().put("state", "error").put("error", connectResult).toString())
                stopForeground(STOP_FOREGROUND_REMOVE)
                stopSelf()
                return
            }

            // 4. Create TUN interface AFTER xray starts (so its sockets are already protected)
            val builder = Builder()
                .setSession("VPN App")
                .addAddress("10.0.0.2", 32)           // IPv4 tunnel address
                .addRoute("0.0.0.0", 0)               // Route all IPv4
                .addAddress("fd00::2", 128)            // IPv6 tunnel address
                .addRoute("::", 0)                     // Route all IPv6 (prevents IPv6 leak)
                .addDnsServer("1.1.1.1")
                .addDnsServer("8.8.8.8")
                .setMtu(1500)
                // setBlocking(true) makes the TUN read block in the kernel. When kill
                // switch is active and nothing is reading from the fd, packets stall in
                // the kernel buffer instead of being dropped immediately, which is the
                // safer blackhole behaviour we want.
                .setBlocking(killSwitchEnabled)

            // Apply split tunneling exclusions — these apps will bypass the VPN.
            for (pkg in excludedApps) {
                try {
                    builder.addDisallowedApplication(pkg)
                } catch (e: Exception) {
                    Log.w(TAG, "Skipping unknown package for split tunnel exclusion: $pkg")
                }
            }

            vpnInterface = builder.establish()

            if (vpnInterface == null) {
                Log.e(TAG, "Failed to establish VPN interface")
                Tunnel.disconnect()
                stopForeground(STOP_FOREGROUND_REMOVE)
                stopSelf()
                return
            }

            // 5. Start tun2socks: pipes TUN packets to SOCKS5 proxy
            val tunFd = vpnInterface!!.fd
            val tunResult = Tunnel.startTun(tunFd.toLong())
            if (tunResult.isNotEmpty()) {
                Log.e(TAG, "tun2socks error: $tunResult")
                Tunnel.disconnect()
                vpnInterface?.close()
                vpnInterface = null
                stopForeground(STOP_FOREGROUND_REMOVE)
                stopSelf()
                return
            }

            isRunning = true
            updateNotification("Connected")
            Log.i(TAG, "VPN tunnel established")

        } catch (e: Exception) {
            Log.e(TAG, "Failed to start VPN", e)
            VpnTurboModule.sendStatusEvent(JSONObject().put("state", "error").put("error", e.message ?: "unknown error").toString())
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
        // Allow a manual disconnect to force-close even when we are in kill
        // switch active state (killSwitchActive = true, isRunning = false).
        if (!isRunning && !killSwitchActive) return

        val applyKillSwitch = killSwitchEnabled && !isManual

        Log.i(TAG, "Stopping VPN tunnel (isManual=$isManual, killSwitch=$applyKillSwitch)")

        // Always stop the tunnel backend — no traffic must pass through xray.
        Tunnel.stopTun()
        Tunnel.disconnect()

        if (applyKillSwitch) {
            // Keep TUN alive to blackhole all traffic. The fd remains open and
            // the kernel routes are still in place, but nothing reads from the fd.
            isRunning = false
            killSwitchActive = true
            VpnTurboModule.sendStatusEvent(
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
            VpnTurboModule.sendStatusEvent(
                """{"state":"disconnected","server_addr":"","protocol":"","connected_at":0,"bytes_up":0,"bytes_down":0}"""
            )

            stopForeground(STOP_FOREGROUND_REMOVE)
            stopSelf()
        }
    }

    fun isActive(): Boolean = isRunning

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
                .setMtu(1500)
                .setBlocking(killSwitchEnabled)

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

            Log.i(TAG, "TUN interface rebuilt successfully with ${excludedApps.size} excluded apps")
        } catch (e: Exception) {
            Log.e(TAG, "Failed to rebuild TUN interface", e)
            stopVpn(isManual = false)
        }
    }

    private fun createNotificationChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val channel = NotificationChannel(
                CHANNEL_ID,
                "VPN Service",
                NotificationManager.IMPORTANCE_LOW
            ).apply {
                description = "Shows VPN connection status"
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
}
