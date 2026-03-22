import NetworkExtension
import os.log

/// PacketTunnelProvider is the iOS Network Extension entry point for the VPN tunnel.
///
/// This runs in a separate process from the main app. It:
/// 1. Receives start/stop commands from the main app via NETunnelProviderManager
/// 2. Loads the Go tunnel library (Tunnel.xcframework compiled via gomobile)
/// 3. Creates a packet tunnel using NEPacketTunnelProvider
/// 4. Routes device traffic through the Go tunnel to the VLESS+REALITY server
///
/// To integrate the Go tunnel:
/// 1. Build: cd client-tunnel && make ios
/// 2. Add Tunnel.xcframework to this target in Xcode
/// 3. import Tunnel (gomobile generated module)
/// 4. Call Tunnel.Connect(configJSON) in startTunnel()
///
class PacketTunnelProvider: NEPacketTunnelProvider {

    private let log = OSLog(subsystem: "com.vpnapp.tunnel", category: "PacketTunnel")

    override func startTunnel(options: [String : NSObject]?, completionHandler: @escaping (Error?) -> Void) {
        os_log("Starting VPN tunnel", log: log, type: .info)

        // Extract config JSON passed from the main app
        let configJSON = options?["config"] as? String ?? "{}"

        // Configure the tunnel network settings
        let settings = NEPacketTunnelNetworkSettings(tunnelRemoteAddress: "10.0.0.1")

        // IPv4 settings — route all traffic through VPN
        let ipv4 = NEIPv4Settings(addresses: ["10.0.0.2"], subnetMasks: ["255.255.255.0"])
        ipv4.includedRoutes = [NEIPv4Route.default()]
        settings.ipv4Settings = ipv4

        // DNS settings — use Cloudflare + Google
        settings.dnsSettings = NEDNSSettings(servers: ["1.1.1.1", "8.8.8.8"])

        settings.mtu = 1500

        // Apply network settings
        setTunnelNetworkSettings(settings) { [weak self] error in
            if let error = error {
                os_log("Failed to set tunnel settings: %{public}@", log: self?.log ?? .default, type: .error, error.localizedDescription)
                completionHandler(error)
                return
            }

            // --- Go tunnel integration point ---
            //
            // When Tunnel.xcframework is added to this target:
            //
            //   import Tunnel
            //
            //   let result = TunnelConnect(configJSON)
            //   if !result.isEmpty {
            //       let error = NSError(domain: "com.vpnapp.tunnel", code: 1,
            //           userInfo: [NSLocalizedDescriptionKey: result])
            //       completionHandler(error)
            //       return
            //   }
            //
            // The Go tunnel starts a local SOCKS5 proxy on localhost:10808.
            // iOS routes packets through the TUN interface, which the Go tunnel reads.

            os_log("VPN tunnel started successfully", log: self?.log ?? .default, type: .info)
            completionHandler(nil)
        }
    }

    override func stopTunnel(with reason: NEProviderStopReason, completionHandler: @escaping () -> Void) {
        os_log("Stopping VPN tunnel, reason: %{public}d", log: log, type: .info, reason.rawValue)

        // When Go tunnel is integrated:
        //   TunnelDisconnect()

        completionHandler()
    }

    override func handleAppMessage(_ messageData: Data, completionHandler: ((Data?) -> Void)?) {
        // Handle messages from the main app (e.g., status queries)
        if let message = String(data: messageData, encoding: .utf8) {
            os_log("Received app message: %{public}@", log: log, type: .debug, message)

            if message == "status" {
                // When Go tunnel is integrated:
                //   let status = TunnelGetStatus()
                //   completionHandler?(status.data(using: .utf8))
                let status = """
                {"state":"connected","server_addr":"","protocol":"vless-reality","connected_at":0,"bytes_up":0,"bytes_down":0}
                """
                completionHandler?(status.data(using: .utf8))
                return
            }
        }
        completionHandler?(nil)
    }
}
