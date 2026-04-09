package com.vpnapp.vpn

import android.app.Activity
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.content.pm.PackageManager
import android.net.VpnService
import android.os.Build
import androidx.core.content.ContextCompat
import com.facebook.react.bridge.*
import com.facebook.react.modules.core.DeviceEventManagerModule
import org.json.JSONArray
import org.json.JSONObject

/**
 * React Native TurboModule that bridges JavaScript to the Android VPN service.
 *
 * TunnelVpnService runs in the ":vpn" process. Communication is done via:
 *   Main -> VPN : explicit startService(Intent) with an ACTION_* action.
 *   VPN -> Main : sendBroadcast(intent.setPackage(packageName)) received here.
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
        private const val TAG = "VpnTurboModule"
        private const val VPN_PREPARE_REQUEST = 1001
    }

    // Config saved while waiting for the VPN permission dialog to complete.
    private var pendingConfigJson: String? = null

    // JS promise for the in-flight connect() call. Resolved on BROADCAST_VPN_CONNECT_RESULT,
    // rejected on error/disconnected status broadcasts.
    private var connectPromise: Promise? = null

    // Handler and runnable used to enforce a 30-second timeout on connectPromise.
    private val mainHandler = android.os.Handler(android.os.Looper.getMainLooper())
    private val connectTimeoutRunnable = Runnable {
        connectPromise?.let { promise ->
            connectPromise = null
            promise.reject("CONNECT_TIMEOUT", "VPN connection timed out")
        }
    }

    // Last known status JSON. Returned by getStatus() without crossing the process boundary.
    private var cachedStatusJson: String =
        """{"state":"disconnected","server_addr":"","protocol":"","connected_at":0,"bytes_up":0,"bytes_down":0}"""

    // BroadcastReceiver that handles messages sent from the :vpn process.
    private val vpnReceiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context, intent: Intent) {
            when (intent.action) {
                TunnelVpnService.BROADCAST_VPN_STATUS -> {
                    val statusJson = intent.getStringExtra(TunnelVpnService.EXTRA_STATUS_JSON) ?: return
                    handleStatusBroadcast(statusJson)
                }
                TunnelVpnService.BROADCAST_VPN_CONNECT_RESULT -> {
                    val success = intent.getBooleanExtra(TunnelVpnService.EXTRA_SUCCESS, false)
                    handleConnectResultBroadcast(success)
                }
            }
        }
    }

    private val activityListener = object : BaseActivityEventListener() {
        override fun onActivityResult(activity: Activity, requestCode: Int, resultCode: Int, intent: Intent?) {
            if (requestCode == VPN_PREPARE_REQUEST) {
                if (resultCode == Activity.RESULT_OK) {
                    pendingConfigJson?.let { startService(it) }
                    pendingConfigJson = null
                } else {
                    // User denied the VPN permission dialog.
                    mainHandler.removeCallbacks(connectTimeoutRunnable)
                    connectPromise?.reject("VPN_PERMISSION_DENIED", "VPN permission not granted")
                    connectPromise = null
                    pendingConfigJson = null
                }
            }
        }
    }

    init {
        reactContext.addActivityEventListener(activityListener)

        // Register for broadcasts from the :vpn process.
        val filter = IntentFilter().apply {
            addAction(TunnelVpnService.BROADCAST_VPN_STATUS)
            addAction(TunnelVpnService.BROADCAST_VPN_CONNECT_RESULT)
        }
        // RECEIVER_NOT_EXPORTED ensures Android 14+ does not expose the receiver
        // to other apps. setPackage on the sender side provides a second layer.
        ContextCompat.registerReceiver(
            reactContext,
            vpnReceiver,
            filter,
            ContextCompat.RECEIVER_NOT_EXPORTED
        )

        // Check for crash breadcrumbs written by the :vpn process in a previous
        // session and upload them to the debug API.
        Thread {
            try {
                val file = java.io.File(reactContext.filesDir, "crash_breadcrumb.txt")
                if (file.exists()) {
                    val breadcrumbs = file.readText().takeLast(1000)
                    file.delete()
                    if (breadcrumbs.isNotBlank()) {
                        val body = JSONObject().apply {
                            put("error", "CRASH BREADCRUMB from previous session")
                            put("action", "crash_breadcrumb")
                            put("stack", breadcrumbs)
                        }
                        val url = java.net.URL("https://vpnapi.mydayai.uz:9443/api/v1/debug/error")
                        val conn = url.openConnection() as java.net.HttpURLConnection
                        conn.requestMethod = "POST"
                        conn.setRequestProperty("Content-Type", "application/json")
                        conn.connectTimeout = 5000
                        conn.readTimeout = 5000
                        conn.doOutput = true
                        conn.outputStream.write(body.toString().toByteArray())
                        val code = conn.responseCode
                        conn.disconnect()
                        android.os.Handler(android.os.Looper.getMainLooper()).post {
                            android.widget.Toast.makeText(
                                reactContext,
                                "Crash log sent to server (${breadcrumbs.lines().size} lines, HTTP $code)",
                                android.widget.Toast.LENGTH_LONG
                            ).show()
                        }
                    }
                }
            } catch (_: Throwable) { }
        }.start()
    }

    override fun getName(): String = "VpnModule"

    override fun invalidate() {
        try { reactApplicationContext.unregisterReceiver(vpnReceiver) } catch (_: Exception) { }
        reactApplicationContext.removeActivityEventListener(activityListener)
        mainHandler.removeCallbacks(connectTimeoutRunnable)
        connectPromise?.reject("MODULE_INVALIDATED", "VPN module was destroyed")
        connectPromise = null
        super.invalidate()
    }

    // MARK: - Broadcast handlers

    /**
     * Handle a VPN_STATUS broadcast from the :vpn process.
     * Updates cached status, forwards to JS, and rejects the pending promise on
     * terminal error/disconnected states.
     */
    private fun handleStatusBroadcast(statusJson: String) {
        cachedStatusJson = statusJson

        // Reject the connect promise on terminal failure states.
        // The success path is handled separately by BROADCAST_VPN_CONNECT_RESULT.
        connectPromise?.let { promise ->
            try {
                val state = JSONObject(statusJson).optString("state", "")
                val error = JSONObject(statusJson).optString("error", "")
                when (state) {
                    "error", "disconnected" -> {
                        mainHandler.removeCallbacks(connectTimeoutRunnable)
                        connectPromise = null
                        if (error.isNotEmpty()) {
                            promise.reject("TUNNEL_ERROR", error)
                        } else {
                            promise.reject("TUNNEL_ERROR", "Connection failed")
                        }
                    }
                }
            } catch (_: Exception) { }
        }

        emitToJS("onVpnStatusChanged", statusJson)
    }

    /**
     * Handle a VPN_CONNECT_RESULT broadcast from the :vpn process.
     * Resolves the pending connect() promise when the tunnel is fully up.
     */
    private fun handleConnectResultBroadcast(success: Boolean) {
        val promise = connectPromise ?: return
        mainHandler.removeCallbacks(connectTimeoutRunnable)
        connectPromise = null
        if (success) {
            promise.resolve("")
        } else {
            promise.reject("TUNNEL_ERROR", "Connection failed")
        }
    }

    private fun emitToJS(event: String, payload: String) {
        val ctx = reactApplicationContext
        if (ctx.hasActiveReactInstance()) {
            ctx.getJSModule(DeviceEventManagerModule.RCTDeviceEventEmitter::class.java)
                .emit(event, payload)
        }
    }

    // MARK: - React Native @ReactMethod implementations

    /**
     * Connect to a VPN server.
     * configJSON: JSON string matching ConnectConfig from the Go tunnel library.
     */
    @ReactMethod
    fun connect(configJSON: String, promise: Promise) {
        try {
            if (connectPromise != null) {
                promise.reject("ALREADY_CONNECTING", "Connect already in progress")
                return
            }

            val activity = reactApplicationContext.currentActivity
            if (activity == null) {
                promise.reject("NO_ACTIVITY", "No active activity")
                return
            }

            requestBatteryOptimizationExemption(activity)

            val vpnIntent = VpnService.prepare(activity as Context)
            if (vpnIntent != null) {
                // VPN permission dialog needs to be shown first.
                pendingConfigJson = configJSON
                connectPromise = promise
                mainHandler.postDelayed(connectTimeoutRunnable, 30_000)
                activity.startActivityForResult(vpnIntent, VPN_PREPARE_REQUEST, null)
            } else {
                connectPromise = promise
                mainHandler.postDelayed(connectTimeoutRunnable, 30_000)
                startService(configJSON)
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
            sendServiceIntent(TunnelVpnService.ACTION_DISCONNECT)
            promise.resolve("")
        } catch (e: Exception) {
            promise.reject("DISCONNECT_ERROR", e.message, e)
        }
    }

    /**
     * Get current tunnel status as JSON.
     * Returns the locally cached value — avoids crossing the process boundary
     * since the Go library lives in the :vpn process.
     */
    @ReactMethod
    fun getStatus(promise: Promise) {
        promise.resolve(cachedStatusJson)
    }

    /**
     * Probe servers for latency.
     * TODO: Route through the :vpn process via Intent to avoid loading the Go
     * runtime in the main process. Returns empty results until that is wired up.
     */
    @ReactMethod
    fun probeServers(serversJSON: String, promise: Promise) {
        // TODO: Route through :vpn process via Intent. For now return empty results.
        promise.resolve("[]")
    }

    /**
     * Get current traffic stats as JSON.
     * Returns cached zeros when the tunnel is in the :vpn process — the main
     * process cannot call Tunnel.getTrafficStats() cross-process.
     * Stats are broadcast via onVpnStatsUpdated events instead.
     */
    @ReactMethod
    fun getTrafficStats(promise: Promise) {
        promise.resolve("""{"bytes_up":0,"bytes_down":0}""")
    }

    /**
     * Enable or disable the kill switch.
     *
     * Persists the setting locally and sends an Intent to the :vpn process so
     * TunnelVpnService can apply it immediately if the tunnel is running.
     */
    @ReactMethod
    fun setKillSwitch(enabled: Boolean, promise: Promise) {
        try {
            // Forward to the :vpn process service. TunnelVpnService is the sole
            // writer of kill_switch_enabled via applyKillSwitchSetting().
            val intent = Intent(reactApplicationContext, TunnelVpnService::class.java).apply {
                action = TunnelVpnService.ACTION_SET_KILL_SWITCH
                putExtra(TunnelVpnService.EXTRA_KILL_SWITCH_ENABLED, enabled)
            }
            reactApplicationContext.startService(intent)

            promise.resolve(null)
        } catch (e: Exception) {
            promise.reject("KILL_SWITCH_ERROR", e.message, e)
        }
    }

    /**
     * Persist and apply the list of package names that should bypass the VPN.
     * appsJson: JSON array of package name strings.
     */
    @ReactMethod
    fun setExcludedApps(appsJson: String, promise: Promise) {
        try {
            // Validate JSON before sending it cross-process.
            JSONArray(appsJson)

            // Forward to the :vpn process service. TunnelVpnService is the sole
            // writer of excluded_apps_json via applyExcludedApps().
            val intent = Intent(reactApplicationContext, TunnelVpnService::class.java).apply {
                action = TunnelVpnService.ACTION_SET_EXCLUDED_APPS
                putExtra(TunnelVpnService.EXTRA_APPS_JSON, appsJson)
            }
            reactApplicationContext.startService(intent)

            promise.resolve(null)
        } catch (e: Exception) {
            promise.reject("SET_EXCLUDED_APPS_ERROR", e.message, e)
        }
    }

    /**
     * Query installed apps that have INTERNET permission.
     * Returns a JSON array of {packageName, appName, isSystemApp}.
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
                    val perms = pkg.requestedPermissions ?: continue
                    if (!perms.contains("android.permission.INTERNET")) continue

                    val appInfo = pkg.applicationInfo ?: continue

                    val isSystem = (appInfo.flags and
                        android.content.pm.ApplicationInfo.FLAG_SYSTEM) != 0

                    val appName = try {
                        pm.getApplicationLabel(appInfo).toString()
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

    // MARK: - Private helpers

    private fun requestBatteryOptimizationExemption(activity: Activity) {
        try {
            val pm = activity.getSystemService(Context.POWER_SERVICE) as android.os.PowerManager
            if (pm.isIgnoringBatteryOptimizations(activity.packageName)) return

            // Only ask once — don't pester the user on every connect
            val prefs = activity.getSharedPreferences("vpn_prefs", Context.MODE_PRIVATE)
            if (prefs.getBoolean("battery_opt_asked", false)) return

            prefs.edit().putBoolean("battery_opt_asked", true).apply()

            val intent = Intent(android.provider.Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS).apply {
                data = android.net.Uri.parse("package:${activity.packageName}")
            }
            activity.startActivity(intent)
        } catch (_: Throwable) { }
    }

    /** Start TunnelVpnService with ACTION_CONNECT and the given config. */
    private fun startService(configJson: String) {
        val intent = Intent(reactApplicationContext, TunnelVpnService::class.java).apply {
            action = TunnelVpnService.ACTION_CONNECT
            putExtra(TunnelVpnService.EXTRA_CONFIG_JSON, configJson)
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            reactApplicationContext.startForegroundService(intent)
        } else {
            reactApplicationContext.startService(intent)
        }
    }

    /** Send a bare action Intent to TunnelVpnService (no extras). */
    private fun sendServiceIntent(action: String) {
        val intent = Intent(reactApplicationContext, TunnelVpnService::class.java).apply {
            this.action = action
        }
        // Use startForegroundService on O+ for reliable delivery to the :vpn process
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            reactApplicationContext.startForegroundService(intent)
        } else {
            reactApplicationContext.startService(intent)
        }
    }
}
