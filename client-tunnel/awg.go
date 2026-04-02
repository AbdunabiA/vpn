package tunnel

import (
	"fmt"
	"sync"

	"github.com/amnezia-vpn/amneziawg-go/device"
)

// AWGConfig holds all configuration for an AmneziaWG tunnel connection.
// Fields map directly to the WireGuard UAPI format plus AmneziaWG's
// obfuscation extension parameters (Jc, Jmin, Jmax, S1, S2, H1-H4).
type AWGConfig struct {
	// WireGuard core fields
	PrivateKey   string `json:"private_key"`
	PublicKey    string `json:"public_key"`    // server's public key
	PresharedKey string `json:"preshared_key,omitempty"`
	Endpoint     string `json:"endpoint"`      // server_ip:port (UDP)
	AllowedIPs   string `json:"allowed_ips"`   // e.g. "0.0.0.0/0, ::/0"

	// AmneziaWG obfuscation parameters.
	// These modify the WireGuard handshake to defeat DPI classifiers:
	//   Jc/Jmin/Jmax — prepend Jc junk UDP packets (sizes between Jmin and Jmax bytes)
	//                  before the real handshake initiation packet.
	//   S1/S2        — subtract from the magic header values in the init/response packets
	//                  so they no longer match the standard WireGuard magic constants.
	//   H1-H4        — XOR masks applied to the type field of each packet type
	//                  (init, response, cookie-reply, transport), making
	//                  the wire format unrecognisable to WireGuard fingerprinting rules.
	Jc   int `json:"jc"`   // Junk packet count    (recommended: 3-10)
	Jmin int `json:"jmin"` // Junk packet min size (recommended: 50-100)
	Jmax int `json:"jmax"` // Junk packet max size (recommended: 200-1000)
	S1   int `json:"s1"`   // Init packet header offset
	S2   int `json:"s2"`   // Response packet header offset
	H1   int `json:"h1"`   // Init packet type XOR mask
	H2   int `json:"h2"`   // Response packet type XOR mask
	H3   int `json:"h3"`   // Cookie-reply packet type XOR mask
	H4   int `json:"h4"`   // Transport packet type XOR mask
}

// awgState holds the live device and logger so stopAWGTunnel can shut down cleanly.
var (
	awgMu     sync.Mutex
	awgDevice *device.Device
	awgLogger *device.Logger
)

// pendingAWGConfig is set by runAWGTunnel and consumed by StartTunAWG.
// It bridges the Go tunnel goroutine and the native module's TUN fd delivery.
var (
	pendingAWGMu     sync.Mutex
	pendingAWGConfig *AWGConfig
)

// setPendingAWGConfig stores the config that StartTunAWG will consume.
func setPendingAWGConfig(cfg *AWGConfig) {
	pendingAWGMu.Lock()
	pendingAWGConfig = cfg
	pendingAWGMu.Unlock()
}

// clearPendingAWGConfig removes any unconsumed pending config.
func clearPendingAWGConfig() {
	pendingAWGMu.Lock()
	pendingAWGConfig = nil
	pendingAWGMu.Unlock()
}

// takePendingAWGConfig atomically retrieves and clears the pending config.
// Returns nil if none is pending.
func takePendingAWGConfig() *AWGConfig {
	pendingAWGMu.Lock()
	cfg := pendingAWGConfig
	pendingAWGConfig = nil
	pendingAWGMu.Unlock()
	return cfg
}

// stopAWGTunnel shuts down the running AmneziaWG device.
// Safe to call when no device is running (no-op).
func stopAWGTunnel() {
	awgMu.Lock()
	defer awgMu.Unlock()

	if awgDevice == nil {
		return
	}

	awgDevice.Close()
	awgDevice = nil
	awgLogger = nil
}

// buildAWGUAPI serialises AWGConfig into the WireGuard UAPI format extended
// with AmneziaWG's obfuscation keys.
//
// UAPI format: one "key=value\n" pair per line; the peer section starts with
// a "public_key=<hex>" line.  amneziawg-go parses the additional keys
// (junk_packet_count, junk_packet_min_size, etc.) before passing the
// remainder to the standard wireguard-go UAPI handler.
func buildAWGUAPI(cfg AWGConfig) (string, error) {
	if cfg.PrivateKey == "" {
		return "", fmt.Errorf("private_key is required")
	}
	if cfg.PublicKey == "" {
		return "", fmt.Errorf("public_key (server) is required")
	}
	if cfg.Endpoint == "" {
		return "", fmt.Errorf("endpoint is required")
	}
	if cfg.AllowedIPs == "" {
		return "", fmt.Errorf("allowed_ips is required")
	}

	allowedIPs := cfg.AllowedIPs

	lines := fmt.Sprintf(
		"private_key=%s\n"+
			"junk_packet_count=%d\n"+
			"junk_packet_min_size=%d\n"+
			"junk_packet_max_size=%d\n"+
			"init_packet_junk_size=%d\n"+
			"response_packet_junk_size=%d\n"+
			"init_packet_magic_header=%d\n"+
			"response_packet_magic_header=%d\n"+
			"underload_packet_magic_header=%d\n"+
			"transport_packet_magic_header=%d\n"+
			"public_key=%s\n"+
			"endpoint=%s\n"+
			"allowed_ip=%s\n",
		cfg.PrivateKey,
		cfg.Jc,
		cfg.Jmin,
		cfg.Jmax,
		cfg.S1,
		cfg.S2,
		cfg.H1,
		cfg.H2,
		cfg.H3,
		cfg.H4,
		cfg.PublicKey,
		cfg.Endpoint,
		allowedIPs,
	)

	if cfg.PresharedKey != "" {
		lines += fmt.Sprintf("preshared_key=%s\n", cfg.PresharedKey)
	}

	// persistent keepalive: 25 seconds keeps NAT mappings alive for UDP.
	lines += "persistent_keepalive_interval=25\n"

	return lines, nil
}
