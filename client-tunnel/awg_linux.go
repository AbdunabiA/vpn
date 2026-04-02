//go:build linux

package tunnel

import (
	"fmt"

	"github.com/amnezia-vpn/amneziawg-go/conn"
	"github.com/amnezia-vpn/amneziawg-go/device"
	"github.com/amnezia-vpn/amneziawg-go/ipc"
	"github.com/amnezia-vpn/amneziawg-go/tun"
	"golang.org/x/sys/unix"
)

// startAWGTunnel creates an AmneziaWG device on the given TUN file descriptor.
//
// fd must be an open, non-blocking TUN file descriptor obtained from the OS
// VPN service (VpnService.Builder.establish().getFd() on Android, or the
// NEPacketTunnelFlow fd on iOS).  The fd is used directly — no tun2socks or
// SOCKS5 proxy is involved; AmneziaWG operates at the packet level.
//
// The function configures the device via the amneziawg-go UAPI interface and
// returns once the device is up and the peer is registered.
//
// Linux/Android only: uses CreateUnmonitoredTUNFromFD which is available on
// the Linux kernel used by both Android and Linux servers.
func startAWGTunnel(fd int, cfg AWGConfig) error {
	awgMu.Lock()
	defer awgMu.Unlock()

	if awgDevice != nil {
		return fmt.Errorf("awg: device already running; call stopAWGTunnel first")
	}

	// Duplicate the fd so the OS can close the original without affecting us.
	dupFD, err := unix.Dup(fd)
	if err != nil {
		return fmt.Errorf("awg: dup fd: %w", err)
	}

	// Wrap the raw fd into a Go TUN device.
	// CreateUnmonitoredTUNFromFD does not set up a route monitor (unnecessary
	// in a mobile VPN context where routing is managed by the OS VPN service).
	tunDev, _, err := tun.CreateUnmonitoredTUNFromFD(dupFD)
	if err != nil {
		unix.Close(dupFD)
		return fmt.Errorf("awg: create tun from fd: %w", err)
	}

	awgLogger = device.NewLogger(device.LogLevelError, "awg: ")

	dev := device.NewDevice(tunDev, conn.NewDefaultBind(), awgLogger)

	// Build the UAPI configuration string.
	// amneziawg-go extends the standard WireGuard UAPI with additional
	// keys for its obfuscation parameters (junk_packet_count, etc.).
	uapiCfg, err := buildAWGUAPI(cfg)
	if err != nil {
		dev.Close()
		return fmt.Errorf("awg: build uapi config: %w", err)
	}

	// Apply config through the UAPI IPC reader.
	if err := dev.IpcSetOperation(ipc.StringToReader(uapiCfg)); err != nil {
		dev.Close()
		return fmt.Errorf("awg: ipc set operation: %w", err)
	}

	dev.Up()

	awgDevice = dev
	return nil
}
