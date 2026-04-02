import NetworkExtension
import os.log

/// Bridges NEPacketTunnelProvider.packetFlow to a file descriptor that
/// the Go tun2socks library can read/write as if it were a TUN device.
///
/// iOS doesn't expose the raw TUN fd to Network Extensions, so we create
/// a UNIX socketpair: one end for Swift (reads/writes packetFlow), the
/// other end is passed to Go's StartTun(fd).
///
/// Packet flow:
///   App traffic -> packetFlow.readPackets -> socketpair -> Go tun2socks -> SOCKS5 -> xray-core
///   xray-core -> SOCKS5 -> Go tun2socks -> socketpair -> packetFlow.writePackets -> App
class PacketFlowBridge {

    private let packetFlow: NEPacketTunnelFlow
    private let log = OSLog(subsystem: "com.vpnapp.tunnel", category: "PacketFlowBridge")

    private var swiftFD: Int32 = -1   // Swift side of the socketpair
    private var goFD: Int32 = -1      // Go side — passed to Tunnel.startTun()
    private var goFDPassedToTunnel = false
    private var running = false
    private let readQueue = DispatchQueue(label: "com.vpnapp.tunnel.bridge.read", qos: .userInteractive)

    init(packetFlow: NEPacketTunnelFlow) {
        self.packetFlow = packetFlow
    }

    /// Creates the socketpair and starts bridging packets.
    /// Returns the Go-side fd to pass to Tunnel.startTun(), or -1 on failure.
    func start() -> Int32 {
        var fds: [Int32] = [0, 0]
        let result = socketpair(AF_UNIX, SOCK_DGRAM, 0, &fds)
        guard result == 0 else {
            os_log("socketpair failed: %{public}d", log: log, type: .error, errno)
            return -1
        }

        swiftFD = fds[0]
        goFD = fds[1]
        running = true

        // Set socket buffer sizes — conservative for Network Extension memory limits
        var bufSize: Int32 = 256 * 1024 // 256KB
        setsockopt(swiftFD, SOL_SOCKET, SO_SNDBUF, &bufSize, socklen_t(MemoryLayout<Int32>.size))
        setsockopt(swiftFD, SOL_SOCKET, SO_RCVBUF, &bufSize, socklen_t(MemoryLayout<Int32>.size))
        setsockopt(goFD, SOL_SOCKET, SO_SNDBUF, &bufSize, socklen_t(MemoryLayout<Int32>.size))
        setsockopt(goFD, SOL_SOCKET, SO_RCVBUF, &bufSize, socklen_t(MemoryLayout<Int32>.size))

        readFromPacketFlow()
        readFromGo()

        os_log("PacketFlowBridge started, goFD=%{public}d", log: log, type: .info, goFD)
        return goFD
    }

    /// Call after successfully passing goFD to Tunnel.startTun().
    /// Marks goFD as owned by Go — stop() will not close it.
    func markGoFDOwned() {
        goFDPassedToTunnel = true
    }

    /// Stops the bridge. Closes the Swift side of the socketpair.
    /// The Go side (goFD) is owned by tun2socks after StartTun() — not closed here.
    func stop() {
        running = false

        // Shutdown the Swift fd to unblock the read loop cleanly,
        // then close. This avoids fd-recycling races.
        if swiftFD >= 0 {
            shutdown(swiftFD, SHUT_RDWR)
            Darwin.close(swiftFD)
            swiftFD = -1
        }

        // Only close goFD if it was never passed to Go
        if !goFDPassedToTunnel && goFD >= 0 {
            Darwin.close(goFD)
        }
        goFD = -1

        os_log("PacketFlowBridge stopped", log: log, type: .info)
    }

    // MARK: - Private

    /// Reads IP packets from the system (packetFlow) and writes them to Go via socketpair.
    private func readFromPacketFlow() {
        packetFlow.readPackets { [weak self] packets, protocols in
            guard let self = self, self.running, self.swiftFD >= 0 else { return }

            for packet in packets {
                packet.withUnsafeBytes { ptr in
                    guard let base = ptr.baseAddress else { return }
                    Darwin.write(self.swiftFD, base, packet.count)
                }
            }

            // Continue reading recursively
            self.readFromPacketFlow()
        }
    }

    /// Reads IP packets from Go (via socketpair) and writes them back to the system.
    private func readFromGo() {
        readQueue.async { [weak self] in
            let bufferSize = 65535 // Max IP packet size
            let buffer = UnsafeMutablePointer<UInt8>.allocate(capacity: bufferSize)
            defer { buffer.deallocate() }

            while let self = self, self.running {
                let bytesRead = Darwin.read(self.swiftFD, buffer, bufferSize)
                if bytesRead <= 0 { break }

                let data = Data(bytes: buffer, count: bytesRead)

                // Determine IP version from first nibble
                let version = (buffer[0] >> 4) & 0x0F
                let proto: NSNumber = (version == 6) ? 10 : 2 // AF_INET6 : AF_INET

                self.packetFlow.writePackets([data], withProtocols: [proto])
            }
        }
    }
}
