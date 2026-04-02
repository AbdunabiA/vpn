import Foundation
import React
import Tunnel

/// Swift implementation of the VPN React Native module for iOS.
/// Bridges React Native JS calls to VpnManager which controls the
/// NETunnelProviderManager -> PacketTunnelProvider -> Go tunnel chain.
///
/// Note: The Go tunnel lifecycle (Connect/Disconnect/StartTun/StopTun) runs
/// inside the Network Extension process. The main app communicates via IPC
/// (sendProviderMessage). Only ProbeServers runs directly in the main app
/// since it's just TCP probing — no xray-core needed.
@objc(VpnModule)
class VpnModuleImpl: RCTEventEmitter {

    private var hasListeners = false
    private var initialized = false

    override init() {
        super.init()
        setupVpnManager()
    }

    @objc override static func requiresMainQueueSetup() -> Bool {
        return false
    }

    override func supportedEvents() -> [String]! {
        return ["onVpnStatusChanged", "onVpnStatsUpdated"]
    }

    override func startObserving() {
        hasListeners = true
    }

    override func stopObserving() {
        hasListeners = false
    }

    // MARK: - React Native Methods

    @objc func connect(_ configJSON: String,
                       resolve: @escaping RCTPromiseResolveBlock,
                       reject: @escaping RCTPromiseRejectBlock) {

        ensureInitialized { [weak self] error in
            if let error = error {
                reject("INIT_ERROR", error.localizedDescription, error)
                return
            }

            VpnManager.shared.connect(configJSON: configJSON) { error in
                if let error = error {
                    reject("CONNECT_ERROR", error.localizedDescription, error)
                } else {
                    resolve("")
                }
            }
        }
    }

    @objc func disconnect(_ resolve: @escaping RCTPromiseResolveBlock,
                          reject: @escaping RCTPromiseRejectBlock) {
        VpnManager.shared.disconnect()
        resolve("")
    }

    @objc func getStatus(_ resolve: @escaping RCTPromiseResolveBlock,
                         reject: @escaping RCTPromiseRejectBlock) {
        // Try IPC to get live status from the Network Extension
        VpnManager.shared.sendMessage("status") { response in
            resolve(response ?? VpnManager.shared.getStatus())
        }
    }

    @objc func probeServers(_ serversJSON: String,
                            resolve: @escaping RCTPromiseResolveBlock,
                            reject: @escaping RCTPromiseRejectBlock) {
        // ProbeServers runs in the main app — it's just TCP probing, no tunnel needed
        DispatchQueue.global(qos: .userInitiated).async {
            let result = TunnelProbeServers(serversJSON)
            resolve(result)
        }
    }

    @objc func getTrafficStats(_ resolve: @escaping RCTPromiseResolveBlock,
                               reject: @escaping RCTPromiseRejectBlock) {
        // Get live stats from the Network Extension via IPC
        VpnManager.shared.sendMessage("stats") { response in
            resolve(response ?? """
            {"bytes_up":0,"bytes_down":0,"speed_up_bps":0,"speed_down_bps":0,"duration_secs":0}
            """)
        }
    }

    @objc func setKillSwitch(_ enabled: Bool,
                             resolve: @escaping RCTPromiseResolveBlock,
                             reject: @escaping RCTPromiseRejectBlock) {
        // Ensure the manager is initialized before applying the preference.
        // If it isn't, ensureInitialized will load it; setKillSwitch persists
        // the preference to shared UserDefaults so it is applied in setupManager.
        ensureInitialized { [weak self] error in
            if let error = error {
                // Non-fatal: the preference is still persisted and will take
                // effect when the manager is initialized the next time.
                _ = self // suppress unused-capture warning
                reject("KILL_SWITCH_ERROR", error.localizedDescription, error)
                return
            }

            VpnManager.shared.setKillSwitch(enabled: enabled) { error in
                if let error = error {
                    reject("KILL_SWITCH_ERROR", error.localizedDescription, error)
                } else {
                    resolve(nil)
                }
            }
        }
    }

    // MARK: - Split Tunneling

    /// Persist a JSON array of domain strings that should bypass the VPN tunnel.
    /// Stored in the shared App Group UserDefaults so that VpnManager can include
    /// them in the options dict when starting the Network Extension.
    @objc func setExcludedDomains(_ domainsJson: String,
                                   resolve: @escaping RCTPromiseResolveBlock,
                                   reject: @escaping RCTPromiseRejectBlock) {
        guard let defaults = UserDefaults(suiteName: "group.com.vpnapp.shared") else {
            reject("DEFAULTS_ERROR", "Cannot access shared UserDefaults", nil)
            return
        }
        defaults.set(domainsJson, forKey: "excluded_domains_json")
        resolve(nil)
    }

    /// iOS does not support per-app VPN without MDM — always returns an empty
    /// array so the JS layer can display the domain-based UI branch instead.
    @objc func getInstalledApps(_ resolve: @escaping RCTPromiseResolveBlock,
                                reject: @escaping RCTPromiseRejectBlock) {
        resolve("[]")
    }

    // MARK: - Private

    private func setupVpnManager() {
        VpnManager.shared.onStatusChanged = { [weak self] statusJSON in
            guard let self = self, self.hasListeners else { return }
            self.sendEvent(withName: "onVpnStatusChanged", body: statusJSON)
        }
    }

    private func ensureInitialized(completion: @escaping (Error?) -> Void) {
        if initialized {
            completion(nil)
            return
        }

        VpnManager.shared.initialize { [weak self] error in
            if error == nil {
                self?.initialized = true
            }
            completion(error)
        }
    }
}
