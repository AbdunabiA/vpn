import Foundation
import NetworkExtension

/// VpnManager wraps NETunnelProviderManager to control the VPN tunnel
/// from the main app. It communicates with PacketTunnelProvider
/// running in the Network Extension process.
///
/// Flow:
///   React Native -> VpnModule.m -> VpnManager -> NETunnelProviderManager
///     -> PacketTunnelProvider (separate process) -> Go tunnel (xray-core)
///
/// Kill switch:
///   Uses Apple's built-in includeAllNetworks API (iOS 14+). When enabled,
///   iOS routes ALL device traffic through the VPN at the OS level. If the
///   tunnel drops, iOS blocks all traffic until it reconnects — no app-level
///   logic required. excludeLocalNetworks is also set so AirDrop / mDNS still
///   work on the local network.
///
class VpnManager: NSObject {

    static let shared = VpnManager()

    private var manager: NETunnelProviderManager?
    private var statusObserver: NSObjectProtocol?

    private let sharedDefaults = UserDefaults(suiteName: "group.com.vpnapp.shared")
    private let killSwitchKey = "kill_switch_enabled"
    private let excludedDomainsKey = "excluded_domains_json"

    // Callback to notify React Native of status changes
    var onStatusChanged: ((String) -> Void)?

    private override init() {
        super.init()
    }

    /// Load or create the VPN configuration
    func initialize(completion: @escaping (Error?) -> Void) {
        NETunnelProviderManager.loadAllFromPreferences { [weak self] managers, error in
            if let error = error {
                completion(error)
                return
            }

            // Use existing config or create new one
            self?.manager = managers?.first ?? NETunnelProviderManager()
            self?.setupManager()
            self?.observeStatus()
            completion(nil)
        }
    }

    // Pending connect completion — resolved when tunnel reports connected/error
    private var connectCompletion: ((Error?) -> Void)?

    /// Connect to VPN with the given configuration.
    /// The completion handler is NOT called until the tunnel actually reports
    /// connected or error via NEVPNStatusDidChange.
    func connect(configJSON: String, completion: @escaping (Error?) -> Void) {
        guard let manager = manager else {
            completion(NSError(domain: "VpnManager", code: 1,
                userInfo: [NSLocalizedDescriptionKey: "VPN not initialized"]))
            return
        }

        // Save config to preferences first
        manager.saveToPreferences { [weak self] error in
            if let error = error {
                completion(error)
                return
            }

            // Load fresh preferences then start
            manager.loadFromPreferences { [weak self] error in
                if let error = error {
                    completion(error)
                    return
                }

                do {
                    var options: [String: NSObject] = ["config": configJSON as NSObject]

                    // Pass excluded domains to the Network Extension so it can
                    // inject direct routing rules into the xray config.
                    if let domainsJson = self?.sharedDefaults?.string(forKey: self?.excludedDomainsKey ?? ""),
                       !domainsJson.isEmpty, domainsJson != "[]" {
                        options["excluded_domains"] = domainsJson as NSObject
                    }

                    // Store completion — will be called by observeStatus when tunnel
                    // reports connected or disconnected/invalid.
                    self?.connectCompletion = completion

                    try manager.connection.startVPNTunnel(options: options)
                } catch {
                    self?.connectCompletion = nil
                    completion(error)
                }
            }
        }
    }

    /// Disconnect from VPN
    func disconnect() {
        manager?.connection.stopVPNTunnel()
    }

    /// Get current connection status
    func getStatus() -> String {
        guard let manager = manager else {
            return stateToJSON("disconnected")
        }

        let state: String
        switch manager.connection.status {
        case .connected: state = "connected"
        case .connecting: state = "connecting"
        case .disconnecting: state = "disconnecting"
        case .reasserting: state = "reconnecting"
        case .disconnected: state = "disconnected"
        case .invalid: state = "error"
        @unknown default: state = "disconnected"
        }

        return stateToJSON(state)
    }

    /// Send a message to the Network Extension and get a response
    func sendMessage(_ message: String, completion: @escaping (String?) -> Void) {
        guard let session = manager?.connection as? NETunnelProviderSession,
              let data = message.data(using: .utf8) else {
            completion(nil)
            return
        }

        do {
            try session.sendProviderMessage(data) { response in
                if let response = response, let str = String(data: response, encoding: .utf8) {
                    completion(str)
                } else {
                    completion(nil)
                }
            }
        } catch {
            completion(nil)
        }
    }

    // MARK: - Kill Switch

    /// Enable or disable the OS-level kill switch.
    ///
    /// On iOS 14+ this sets `includeAllNetworks = true` on the
    /// NETunnelProviderProtocol, which instructs iOS to block all traffic
    /// outside the VPN tunnel at the network layer. If the tunnel drops,
    /// iOS itself prevents any traffic from leaking until the tunnel comes
    /// back up — no user-space logic required.
    ///
    /// The preference is saved to the shared UserDefaults group so that the
    /// Network Extension can also read it if needed. The manager configuration
    /// is updated and saved immediately; if the VPN is currently connected the
    /// caller is responsible for reconnecting to apply the new settings.
    func setKillSwitch(enabled: Bool, completion: @escaping (Error?) -> Void) {
        sharedDefaults?.set(enabled, forKey: killSwitchKey)

        guard let manager = manager else {
            // Not yet initialized — preference is saved and will be applied on
            // the next setupManager() call.
            completion(nil)
            return
        }

        applyKillSwitchToProtocol(manager: manager, enabled: enabled)

        manager.saveToPreferences { error in
            if let error = error {
                completion(error)
                return
            }
            manager.loadFromPreferences { error in
                completion(error)
            }
        }
    }

    // MARK: - Private

    private func setupManager() {
        let proto = NETunnelProviderProtocol()
        proto.providerBundleIdentifier = "com.vpnapp.VpnAppNetworkExtension"
        proto.serverAddress = "VPN App" // Display name in Settings
        proto.disconnectOnSleep = false

        // Apply the persisted kill switch preference
        let killSwitchEnabled = sharedDefaults?.bool(forKey: killSwitchKey) ?? false
        applyKillSwitchToProtocol(proto: proto, enabled: killSwitchEnabled)

        manager?.protocolConfiguration = proto
        manager?.localizedDescription = "VPN App"
        manager?.isEnabled = true
    }

    /// Applies includeAllNetworks / excludeLocalNetworks directly to a protocol
    /// object. Requires iOS 14+; silently skipped on older OS versions.
    private func applyKillSwitchToProtocol(proto: NETunnelProviderProtocol, enabled: Bool) {
        if #available(iOS 14.0, *) {
            // Route ALL traffic through the VPN (kill switch)
            proto.includeAllNetworks = enabled
            // Still allow LAN/mDNS/AirDrop so local network features work
            proto.excludeLocalNetworks = enabled
        }
    }

    /// Convenience overload that reads the protocol from an existing manager.
    private func applyKillSwitchToProtocol(manager: NETunnelProviderManager, enabled: Bool) {
        guard let proto = manager.protocolConfiguration as? NETunnelProviderProtocol else {
            return
        }
        applyKillSwitchToProtocol(proto: proto, enabled: enabled)
        manager.protocolConfiguration = proto
    }

    private func observeStatus() {
        statusObserver = NotificationCenter.default.addObserver(
            forName: .NEVPNStatusDidChange,
            object: manager?.connection,
            queue: .main
        ) { [weak self] _ in
            guard let self = self else { return }
            let status = self.getStatus()

            // Resolve pending connect completion when tunnel reaches a final state
            if let completion = self.connectCompletion {
                switch self.manager?.connection.status {
                case .connected:
                    self.connectCompletion = nil
                    completion(nil)
                case .disconnected, .invalid:
                    self.connectCompletion = nil
                    completion(NSError(domain: "VpnManager", code: 2,
                        userInfo: [NSLocalizedDescriptionKey: "Tunnel connection failed"]))
                default:
                    break // still transitioning (connecting, reasserting)
                }
            }

            self.onStatusChanged?(status)
        }
    }

    private func stateToJSON(_ state: String) -> String {
        let dict: [String: Any] = [
            "state": state,
            "server_addr": "",
            "protocol": "vless-reality",
            "connected_at": 0,
            "bytes_up": 0,
            "bytes_down": 0
        ]
        guard let data = try? JSONSerialization.data(withJSONObject: dict, options: [.sortedKeys]),
              let json = String(data: data, encoding: .utf8) else {
            // Fallback: return a safe minimal JSON — should never happen with a static dict.
            return #"{"state":"disconnected","server_addr":"","protocol":"vless-reality","connected_at":0,"bytes_up":0,"bytes_down":0}"#
        }
        return json
    }

    deinit {
        if let observer = statusObserver {
            NotificationCenter.default.removeObserver(observer)
        }
    }
}
