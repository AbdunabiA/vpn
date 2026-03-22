package com.vpnapp.vpn

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.os.ParcelFileDescriptor
import android.util.Log

/**
 * Android VpnService that manages the VPN tunnel.
 *
 * When the Go tunnel library (.aar compiled via gomobile) is integrated,
 * this service loads the Go tunnel and routes device traffic through it.
 *
 * Flow:
 *   React Native -> VpnTurboModule -> TunnelVpnService -> Go tunnel (xray-core)
 *
 * The Go tunnel opens a local SOCKS5 proxy on localhost:10808.
 * This service creates a TUN interface and routes traffic to that proxy.
 */
class TunnelVpnService : VpnService() {

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
     * Starts the VPN tunnel.
     *
     * 1. Creates a TUN interface via VpnService.Builder
     * 2. Starts the Go tunnel library with the provided config
     * 3. The Go tunnel connects to the remote VLESS+REALITY server
     * 4. Device traffic flows: App -> TUN -> Go tunnel -> Remote server
     */
    private fun startVpn(configJson: String) {
        if (isRunning) return

        Log.i(TAG, "Starting VPN tunnel")

        // Start as foreground service (required for Android 8+)
        startForeground(NOTIFICATION_ID, buildNotification("Connecting..."))

        try {
            // Create the VPN TUN interface
            val builder = Builder()
                .setSession("VPN App")
                .addAddress("10.0.0.2", 32)           // VPN client IP
                .addRoute("0.0.0.0", 0)               // Route all IPv4 traffic
                .addDnsServer("1.1.1.1")               // Cloudflare DNS
                .addDnsServer("8.8.8.8")               // Google DNS backup
                .setMtu(1500)
                .setBlocking(true)

            // Protect the tunnel socket from being routed through VPN (prevents loop)
            // When Go tunnel is integrated:
            //   builder.protect(goTunnelSocketFd)

            vpnInterface = builder.establish()

            if (vpnInterface == null) {
                Log.e(TAG, "Failed to establish VPN interface")
                stopSelf()
                return
            }

            // --- Go tunnel integration point ---
            // When the .aar is integrated:
            //
            //   import tunnel.Tunnel  // gomobile generated
            //
            //   // Pass the TUN file descriptor to the Go tunnel
            //   val tunFd = vpnInterface!!.fd
            //   val result = Tunnel.connect(configJson)
            //   if (result.isNotEmpty()) {
            //       Log.e(TAG, "Tunnel connect error: $result")
            //       stopVpn()
            //       return
            //   }
            //
            // For now, the VPN interface is established but traffic isn't tunneled yet.

            isRunning = true
            updateNotification("Connected")
            VpnTurboModule.sendStatusEvent("""{"state":"connected","server_addr":"","protocol":"vless-reality","connected_at":${System.currentTimeMillis() / 1000},"bytes_up":0,"bytes_down":0}""")

            Log.i(TAG, "VPN tunnel established")

        } catch (e: Exception) {
            Log.e(TAG, "Failed to start VPN", e)
            VpnTurboModule.sendStatusEvent("""{"state":"error","error":"${e.message}"}""")
            stopSelf()
        }
    }

    /**
     * Stops the VPN tunnel and cleans up.
     */
    private fun stopVpn() {
        if (!isRunning) return

        Log.i(TAG, "Stopping VPN tunnel")

        // When Go tunnel is integrated:
        //   Tunnel.disconnect()

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

    /**
     * Checks if the VPN tunnel is currently active.
     */
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
