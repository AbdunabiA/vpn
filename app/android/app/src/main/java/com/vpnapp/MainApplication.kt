package com.vpnapp

import android.app.Application
import android.util.Log
import com.facebook.react.PackageList
import org.json.JSONObject
import com.facebook.react.ReactApplication
import com.facebook.react.ReactHost
import com.facebook.react.ReactNativeApplicationEntryPoint.loadReactNative
import com.facebook.react.defaults.DefaultReactHost.getDefaultReactHost
import com.vpnapp.vpn.VpnPackage
import java.net.HttpURLConnection
import java.net.URL

class MainApplication : Application(), ReactApplication {

  override val reactHost: ReactHost by lazy {
    getDefaultReactHost(
      context = applicationContext,
      packageList =
        PackageList(this).packages.apply {
          // Register VPN native module
          add(VpnPackage())
        },
    )
  }

  override fun onCreate() {
    super.onCreate()

    // Global crash handler — sends crash info to API before dying
    val defaultHandler = Thread.getDefaultUncaughtExceptionHandler()
    Thread.setDefaultUncaughtExceptionHandler { thread, throwable ->
      Log.e("VpnApp", "FATAL CRASH: ${throwable.javaClass.name}: ${throwable.message}", throwable)
      try {
        val body = JSONObject().apply {
          put("error", "FATAL: ${throwable.javaClass.name}: ${throwable.message}")
          put("action", "crash")
          put("stack", throwable.stackTraceToString().take(500))
        }.toString()
        val url = URL("https://vpnapi.mydayai.uz:9443/api/v1/debug/error")
        val conn = url.openConnection() as HttpURLConnection
        conn.requestMethod = "POST"
        conn.setRequestProperty("Content-Type", "application/json")
        conn.connectTimeout = 3000
        conn.readTimeout = 3000
        conn.doOutput = true
        conn.outputStream.write(body.toByteArray())
        conn.responseCode // trigger the request
        conn.disconnect()
      } catch (_: Throwable) { }
      defaultHandler?.uncaughtException(thread, throwable)
    }

    loadReactNative(this)
  }
}
