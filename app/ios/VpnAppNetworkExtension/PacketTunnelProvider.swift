import NetworkExtension
import os.log
import Tunnel

/// PacketTunnelProvider is the iOS Network Extension entry point for the VPN tunnel.
///
/// This runs in a separate process from the main app. It:
/// 1. Receives start/stop commands from the main app via NETunnelProviderManager
/// 2. Starts the Go tunnel (xray-core SOCKS5 proxy on localhost:10808)
/// 3. Bridges packetFlow <-> Go tun2socks via PacketFlowBridge
/// 4. Routes all device traffic through VLESS+REALITY
class PacketTunnelProvider: NEPacketTunnelProvider {

    private let log = OSLog(subsystem: "com.vpnapp.tunnel", category: "PacketTunnel")
    private var packetFlowBridge: PacketFlowBridge?

    // StatusCallback: stores status in shared UserDefaults for the main app
    private class TunnelStatusHandler: NSObject, TunnelStatusCallbackProtocol {
        func onStatusChanged(_ statusJSON: String?) {
            guard let json = statusJSON else { return }
            let defaults = UserDefaults(suiteName: "group.com.vpnapp.shared")
            defaults?.set(json, forKey: "tunnel_status")
        }
    }

    private let statusHandler = TunnelStatusHandler()

    override func startTunnel(options: [String : NSObject]?, completionHandler: @escaping (Error?) -> Void) {
        os_log("Starting VPN tunnel", log: log, type: .info)

        let baseConfigJSON = options?["config"] as? String ?? "{}"

        // Merge excluded_domains into the config JSON so that the Go tunnel can
        // insert a direct routing rule for those domains.
        let configJSON = mergeExcludedDomains(into: baseConfigJSON, options: options)

        // 1. Register status callback
        TunnelSetStatusCallback(statusHandler)

        // 2. Start the Go tunnel (xray-core SOCKS5 proxy on 10808)
        let connectResult = TunnelConnect(configJSON)
        if !connectResult.isEmpty {
            os_log("Tunnel connect error: %{public}@", log: log, type: .error, connectResult)
            completionHandler(NSError(
                domain: "com.vpnapp.tunnel", code: 1,
                userInfo: [NSLocalizedDescriptionKey: connectResult]))
            return
        }

        // 3. Configure tunnel network settings
        let settings = NEPacketTunnelNetworkSettings(tunnelRemoteAddress: "10.0.0.1")

        let ipv4 = NEIPv4Settings(addresses: ["10.0.0.2"], subnetMasks: ["255.255.255.0"])
        ipv4.includedRoutes = [NEIPv4Route.default()]
        settings.ipv4Settings = ipv4

        // IPv6 — route all IPv6 traffic through the tunnel to prevent IPv6 leaks
        let ipv6 = NEIPv6Settings(addresses: ["fd00::2"], networkPrefixLengths: [128])
        ipv6.includedRoutes = [NEIPv6Route.default()]
        settings.ipv6Settings = ipv6

        settings.dnsSettings = NEDNSSettings(servers: ["1.1.1.1", "8.8.8.8"])
        settings.mtu = 1500

        // 4. Apply settings, then start the packet flow bridge + tun2socks
        setTunnelNetworkSettings(settings) { [weak self] error in
            guard let self = self else { return }

            if let error = error {
                os_log("Failed to set tunnel settings: %{public}@",
                       log: self.log, type: .error, error.localizedDescription)
                TunnelDisconnect()
                completionHandler(error)
                return
            }

            // 5. Create PacketFlowBridge: packetFlow <-> socketpair <-> Go tun2socks
            let bridge = PacketFlowBridge(packetFlow: self.packetFlow)
            let goFD = bridge.start()

            if goFD < 0 {
                os_log("PacketFlowBridge failed to start", log: self.log, type: .error)
                TunnelDisconnect()
                completionHandler(NSError(
                    domain: "com.vpnapp.tunnel", code: 2,
                    userInfo: [NSLocalizedDescriptionKey: "Failed to create packet flow bridge"]))
                return
            }

            self.packetFlowBridge = bridge

            // 6. Start tun2socks: bridges the socketpair fd to the SOCKS5 proxy
            let tunResult = TunnelStartTun(Int(goFD))
            if tunResult.isEmpty {
                bridge.markGoFDOwned() // Go now owns this fd
            }
            if !tunResult.isEmpty {
                os_log("tun2socks error: %{public}@", log: self.log, type: .error, tunResult)
                bridge.stop()
                TunnelDisconnect()
                completionHandler(NSError(
                    domain: "com.vpnapp.tunnel", code: 3,
                    userInfo: [NSLocalizedDescriptionKey: tunResult]))
                return
            }

            os_log("VPN tunnel started successfully", log: self.log, type: .info)
            completionHandler(nil)
        }
    }

    override func stopTunnel(with reason: NEProviderStopReason, completionHandler: @escaping () -> Void) {
        os_log("Stopping VPN tunnel, reason: %{public}d", log: log, type: .info, reason.rawValue)

        // Order: stop tun2socks -> stop bridge -> disconnect xray
        TunnelStopTun()
        packetFlowBridge?.stop()
        packetFlowBridge = nil
        TunnelDisconnect()

        completionHandler()
    }

    // MARK: - Split Tunneling Helpers

    /// Reads the `excluded_domains` JSON string from the tunnel options (set by
    /// VpnManager.connect) and merges it into the xray ConnectConfig JSON as the
    /// `excluded_domains` array field.  Returns the original configJSON unchanged
    /// if parsing fails or if no domains are present.
    private func mergeExcludedDomains(into configJSON: String,
                                       options: [String: NSObject]?) -> String {
        guard
            let domainsJson = options?["excluded_domains"] as? String,
            !domainsJson.isEmpty,
            let configData = configJSON.data(using: .utf8),
            var configDict = try? JSONSerialization.jsonObject(with: configData) as? [String: Any],
            let domainsData = domainsJson.data(using: .utf8),
            let domainsArray = try? JSONSerialization.jsonObject(with: domainsData) as? [String],
            !domainsArray.isEmpty
        else {
            return configJSON
        }

        configDict["excluded_domains"] = domainsArray

        if let merged = try? JSONSerialization.data(withJSONObject: configDict),
           let mergedString = String(data: merged, encoding: .utf8) {
            return mergedString
        }

        return configJSON
    }

    override func handleAppMessage(_ messageData: Data, completionHandler: ((Data?) -> Void)?) {
        guard let message = String(data: messageData, encoding: .utf8) else {
            completionHandler?(nil)
            return
        }

        os_log("Received app message: %{public}@", log: log, type: .debug, message)

        switch message {
        case "status":
            let status = TunnelGetStatus()
            completionHandler?(status.data(using: .utf8))
        case "stats":
            let stats = TunnelGetTrafficStats()
            completionHandler?(stats.data(using: .utf8))
        default:
            completionHandler?(nil)
        }
    }
}
