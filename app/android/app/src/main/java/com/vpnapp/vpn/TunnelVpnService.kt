package com.vpnapp.vpn

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
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
 */
class TunnelVpnService : VpnService(), ProtectSocket, StatusCallback {

    private var vpnInterface: ParcelFileDescriptor? = null
    private var isRunning = false

    companion object {
        private const val TAG = "TunnelVpnService"
        private const val CHANNEL_ID = "vpn_channel"
        private const val NOTIFICATION_ID = 1
        const val ACTION_CONNECT = "com.vpnapp.CONNECT"
        const val ACTION_DISCONNECT = "com.vpnapp.DISCONNECT"
        const val EXTRA_CONFIG_JSON = "config_json"

        // Singleton reference for the TurboModule to interact with
        var instance: TunnelVpnService? = null
            private set
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
            VpnTurboModule.sendStatusEvent(json)
            // Update notification based on parsed state
            try {
                val state = JSONObject(json).optString("state", "")
                when (state) {
                    "connected" -> updateNotification("Connected")
                    "connecting" -> updateNotification("Connecting...")
                    "disconnecting" -> updateNotification("Disconnecting...")
                    "error" -> updateNotification("Error")
                }
            } catch (_: Exception) { }
        }
    }

    override fun onCreate() {
        super.onCreate()
        instance = this
        createNotificationChannel()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_CONNECT -> {
                val configJson = intent.getStringExtra(EXTRA_CONFIG_JSON) ?: ""
                startVpn(configJson)
            }
            ACTION_DISCONNECT -> {
                stopVpn()
            }
        }
        return START_STICKY
    }

    override fun onDestroy() {
        stopVpn()
        instance = null
        super.onDestroy()
    }

    /**
     * Starts the VPN tunnel:
     * 1. Register socket protection (prevents routing loop)
     * 2. Start Go tunnel (xray-core SOCKS5 proxy on localhost:10808)
     * 3. Create TUN interface
     * 4. Start tun2socks (bridges TUN <-> SOCKS5)
     */
    private fun startVpn(configJson: String) {
        if (isRunning) return

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
                .setBlocking(false)

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
     * Stops the VPN tunnel. Order matters:
     * 1. Stop tun2socks
     * 2. Disconnect xray-core
     * 3. Close TUN interface
     */
    private fun stopVpn() {
        if (!isRunning) return

        Log.i(TAG, "Stopping VPN tunnel")

        Tunnel.stopTun()
        Tunnel.disconnect()

        try {
            vpnInterface?.close()
            vpnInterface = null
        } catch (e: Exception) {
            Log.e(TAG, "Error closing VPN interface", e)
        }

        isRunning = false
        VpnTurboModule.sendStatusEvent("""{"state":"disconnected","server_addr":"","protocol":"","connected_at":0,"bytes_up":0,"bytes_down":0}""")

        stopForeground(STOP_FOREGROUND_REMOVE)
        stopSelf()
    }

    fun isActive(): Boolean = isRunning

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
