package com.vpnapp.vpn

import android.app.Activity
import android.content.Intent
import android.net.VpnService
import com.facebook.react.bridge.*
import com.facebook.react.modules.core.DeviceEventManagerModule

/**
 * React Native TurboModule that bridges JavaScript to the Android VPN service.
 *
 * JS calls:
 *   NativeModules.VpnModule.connect(configJSON)
 *   NativeModules.VpnModule.disconnect()
 *   NativeModules.VpnModule.getStatus()
 *
 * Events emitted to JS:
 *   onVpnStatusChanged  -> TunnelStatus JSON
 *   onVpnStatsUpdated   -> TrafficStats JSON
 */
class VpnTurboModule(reactContext: ReactApplicationContext)
    : ReactContextBaseJavaModule(reactContext) {

    companion object {
        private const val VPN_PREPARE_REQUEST = 1001
        private var pendingConfigJson: String? = null
        private var reactCtx: ReactApplicationContext? = null

        /**
         * Send a status event to React Native JS.
         * Called by TunnelVpnService when connection state changes.
         */
        fun sendStatusEvent(statusJson: String) {
            reactCtx?.let { ctx ->
                if (ctx.hasActiveReactInstance()) {
                    ctx.getJSModule(DeviceEventManagerModule.RCTDeviceEventEmitter::class.java)
                        .emit("onVpnStatusChanged", statusJson)
                }
            }
        }

        /**
         * Send traffic stats event to React Native JS.
         */
        fun sendStatsEvent(statsJson: String) {
            reactCtx?.let { ctx ->
                if (ctx.hasActiveReactInstance()) {
                    ctx.getJSModule(DeviceEventManagerModule.RCTDeviceEventEmitter::class.java)
                        .emit("onVpnStatsUpdated", statsJson)
                }
            }
        }
    }

    private val activityListener = object : BaseActivityEventListener() {
        override fun onActivityResult(activity: Activity?, requestCode: Int, resultCode: Int, intent: Intent?) {
            if (requestCode == VPN_PREPARE_REQUEST) {
                if (resultCode == Activity.RESULT_OK) {
                    // User granted VPN permission — start the service
                    pendingConfigJson?.let { startService(it) }
                    pendingConfigJson = null
                }
            }
        }
    }

    init {
        reactCtx = reactContext
        reactContext.addActivityEventListener(activityListener)
    }

    override fun getName(): String = "VpnModule"

    /**
     * Connect to a VPN server.
     * configJSON: JSON string matching ConnectConfig from the Go tunnel library.
     */
    @ReactMethod
    fun connect(configJSON: String, promise: Promise) {
        try {
            val activity = currentActivity
            if (activity == null) {
                promise.reject("NO_ACTIVITY", "No active activity")
                return
            }

            // Check if VPN permission is granted
            val vpnIntent = VpnService.prepare(activity)
            if (vpnIntent != null) {
                // Need to request permission — save config for after approval
                pendingConfigJson = configJSON
                activity.startActivityForResult(vpnIntent, VPN_PREPARE_REQUEST)
                promise.resolve("") // Will connect after permission granted
            } else {
                // Permission already granted — start immediately
                startService(configJSON)
                promise.resolve("")
            }
        } catch (e: Exception) {
            promise.reject("CONNECT_ERROR", e.message, e)
        }
    }

    /**
     * Disconnect from the VPN.
     */
    @ReactMethod
    fun disconnect(promise: Promise) {
        try {
            val intent = Intent(reactApplicationContext, TunnelVpnService::class.java).apply {
                action = TunnelVpnService.ACTION_DISCONNECT
            }
            reactApplicationContext.startService(intent)
            promise.resolve("")
        } catch (e: Exception) {
            promise.reject("DISCONNECT_ERROR", e.message, e)
        }
    }

    /**
     * Get current tunnel status as JSON.
     */
    @ReactMethod
    fun getStatus(promise: Promise) {
        val service = TunnelVpnService.instance
        val isActive = service?.isActive() ?: false
        val state = if (isActive) "connected" else "disconnected"
        promise.resolve("""{"state":"$state","server_addr":"","protocol":"vless-reality","connected_at":0,"bytes_up":0,"bytes_down":0}""")
    }

    /**
     * Probe servers for latency.
     */
    @ReactMethod
    fun probeServers(serversJSON: String, promise: Promise) {
        // When Go tunnel .aar is integrated:
        //   val result = Tunnel.probeServers(serversJSON)
        //   promise.resolve(result)
        promise.resolve("[]")
    }

    /**
     * Get current traffic stats as JSON.
     */
    @ReactMethod
    fun getTrafficStats(promise: Promise) {
        // When Go tunnel .aar is integrated:
        //   val result = Tunnel.getTrafficStats()
        //   promise.resolve(result)
        promise.resolve("""{"bytes_up":0,"bytes_down":0,"speed_up_bps":0,"speed_down_bps":0,"duration_secs":0}""")
    }

    private fun startService(configJson: String) {
        val intent = Intent(reactApplicationContext, TunnelVpnService::class.java).apply {
            action = TunnelVpnService.ACTION_CONNECT
            putExtra(TunnelVpnService.EXTRA_CONFIG_JSON, configJson)
        }
        if (android.os.Build.VERSION.SDK_INT >= android.os.Build.VERSION_CODES.O) {
            reactApplicationContext.startForegroundService(intent)
        } else {
            reactApplicationContext.startService(intent)
        }
    }
}
