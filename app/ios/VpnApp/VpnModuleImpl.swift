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
