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
class VpnManager: NSObject {

    static let shared = VpnManager()

    private var manager: NETunnelProviderManager?
    private var statusObserver: NSObjectProtocol?

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

    /// Connect to VPN with the given configuration
    func connect(configJSON: String, completion: @escaping (Error?) -> Void) {
        guard let manager = manager else {
            completion(NSError(domain: "VpnManager", code: 1,
                userInfo: [NSLocalizedDescriptionKey: "VPN not initialized"]))
            return
        }

        // Save config to preferences first
        manager.saveToPreferences { error in
            if let error = error {
                completion(error)
                return
            }

            // Load fresh preferences then start
            manager.loadFromPreferences { error in
                if let error = error {
                    completion(error)
                    return
                }

                do {
                    let options = ["config": configJSON as NSObject]
                    try manager.connection.startVPNTunnel(options: options)
                    completion(nil)
                } catch {
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

    // MARK: - Private

    private func setupManager() {
        let proto = NETunnelProviderProtocol()
        proto.providerBundleIdentifier = "com.vpnapp.VpnAppNetworkExtension"
        proto.serverAddress = "VPN App" // Display name in Settings
        proto.disconnectOnSleep = false

        manager?.protocolConfiguration = proto
        manager?.localizedDescription = "VPN App"
        manager?.isEnabled = true
    }

    private func observeStatus() {
        statusObserver = NotificationCenter.default.addObserver(
            forName: .NEVPNStatusDidChange,
            object: manager?.connection,
            queue: .main
        ) { [weak self] _ in
            guard let self = self else { return }
            let status = self.getStatus()
            self.onStatusChanged?(status)
        }
    }

    private func stateToJSON(_ state: String) -> String {
        return """
        {"state":"\(state)","server_addr":"","protocol":"vless-reality","connected_at":0,"bytes_up":0,"bytes_down":0}
        """
    }

    deinit {
        if let observer = statusObserver {
            NotificationCenter.default.removeObserver(observer)
        }
    }
}
