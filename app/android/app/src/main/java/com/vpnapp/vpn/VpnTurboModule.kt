package com.vpnapp.vpn

import android.app.Activity
import android.content.Intent
import android.content.pm.PackageManager
import android.net.VpnService
import com.facebook.react.bridge.*
import com.facebook.react.modules.core.DeviceEventManagerModule
import org.json.JSONArray
import org.json.JSONObject
import tunnel.Tunnel

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
        override fun onActivityResult(activity: Activity, requestCode: Int, resultCode: Int, intent: Intent?) {
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
            val activity = reactApplicationContext.currentActivity
            if (activity == null) {
                promise.reject("NO_ACTIVITY", "No active activity")
                return
            }

            // Check if VPN permission is granted
            val vpnIntent = VpnService.prepare(activity as android.content.Context)
            if (vpnIntent != null) {
                // Need to request permission — save config for after approval
                pendingConfigJson = configJSON
                activity.startActivityForResult(vpnIntent, VPN_PREPARE_REQUEST, null)
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
        promise.resolve(Tunnel.getStatus())
    }

    /**
     * Probe servers for latency.
     */
    @ReactMethod
    fun probeServers(serversJSON: String, promise: Promise) {
        // Run on background thread — probing involves network I/O
        Thread {
            try {
                val result = Tunnel.probeServers(serversJSON)
                promise.resolve(result)
            } catch (e: Exception) {
                promise.reject("PROBE_ERROR", e.message, e)
            }
        }.start()
    }

    /**
     * Get current traffic stats as JSON.
     */
    @ReactMethod
    fun getTrafficStats(promise: Promise) {
        promise.resolve(Tunnel.getTrafficStats())
    }

    /**
     * Enable or disable the kill switch.
     *
     * The preference is persisted in SharedPreferences so it survives app and
     * service restarts. If TunnelVpnService is currently running the setting is
     * applied immediately.
     */
    @ReactMethod
    fun setKillSwitch(enabled: Boolean, promise: Promise) {
        try {
            // Persist so the service can read it on next start
            reactApplicationContext
                .getSharedPreferences("vpn_prefs", android.content.Context.MODE_PRIVATE)
                .edit()
                .putBoolean("kill_switch_enabled", enabled)
                .apply()

            // Apply to the running service instance if available
            TunnelVpnService.instance?.setKillSwitch(enabled)

            promise.resolve(null)
        } catch (e: Exception) {
            promise.reject("KILL_SWITCH_ERROR", e.message, e)
        }
    }

    /**
     * Persist and apply the list of package names that should bypass the VPN.
     * appsJson: JSON array of package name strings, e.g. ["com.google.android.youtube"].
     */
    @ReactMethod
    fun setExcludedApps(appsJson: String, promise: Promise) {
        try {
            // Validate JSON
            val arr = JSONArray(appsJson)
            val apps = (0 until arr.length()).map { arr.getString(it) }

            // Persist so the service can restore the list on restart
            reactApplicationContext
                .getSharedPreferences(TunnelVpnService.PREFS_NAME, android.content.Context.MODE_PRIVATE)
                .edit()
                .putString(TunnelVpnService.PREF_EXCLUDED_APPS, appsJson)
                .apply()

            // Apply to the running service instance if available (rebuilds TUN immediately)
            TunnelVpnService.instance?.setExcludedApps(apps)

            promise.resolve(null)
        } catch (e: Exception) {
            promise.reject("SET_EXCLUDED_APPS_ERROR", e.message, e)
        }
    }

    /**
     * Query installed apps that have INTERNET permission.
     * Returns a JSON array of {packageName, appName, isSystemApp}.
     * System apps (android.uid.system flag) are included but flagged.
     *
     * Runs on a background thread — PackageManager queries can be slow.
     */
    @ReactMethod
    fun getInstalledApps(promise: Promise) {
        Thread {
            try {
                val pm = reactApplicationContext.packageManager
                val packages = pm.getInstalledPackages(PackageManager.GET_PERMISSIONS)
                val result = JSONArray()

                for (pkg in packages) {
                    // Only include apps that declare INTERNET permission
                    val perms = pkg.requestedPermissions ?: continue
                    if (!perms.contains("android.permission.INTERNET")) continue

                    val isSystem = (pkg.applicationInfo.flags and
                        android.content.pm.ApplicationInfo.FLAG_SYSTEM) != 0

                    val appName = try {
                        pm.getApplicationLabel(pkg.applicationInfo).toString()
                    } catch (_: Exception) {
                        pkg.packageName
                    }

                    val item = JSONObject().apply {
                        put("packageName", pkg.packageName)
                        put("appName", appName)
                        put("isSystemApp", isSystem)
                    }
                    result.put(item)
                }

                promise.resolve(result.toString())
            } catch (e: Exception) {
                promise.reject("GET_INSTALLED_APPS_ERROR", e.message, e)
            }
        }.start()
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
