// Package tunnel provides the public API for the VPN tunnel library.
// This package is compiled via gomobile to produce:
//   - Android: tunnel.aar
//   - iOS: Tunnel.xcframework
//
// Build commands:
//
//	gomobile bind -target=android -o tunnel.aar ./
//	gomobile bind -target=ios -o Tunnel.xcframework ./
//
// The React Native native modules (TurboModules) call these functions
// to control the VPN tunnel lifecycle.
package tunnel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	// XRay-core — import only what VLESS+REALITY/WebSocket client needs
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf/serial"
	_ "github.com/xtls/xray-core/app/dispatcher"
	_ "github.com/xtls/xray-core/app/proxyman/inbound"
	_ "github.com/xtls/xray-core/app/proxyman/outbound"
	_ "github.com/xtls/xray-core/proxy/freedom"
	_ "github.com/xtls/xray-core/proxy/socks"
	_ "github.com/xtls/xray-core/proxy/vless/outbound"
	_ "github.com/xtls/xray-core/transport/internet/reality"
	_ "github.com/xtls/xray-core/transport/internet/tcp"
	_ "github.com/xtls/xray-core/transport/internet/tls"
	_ "github.com/xtls/xray-core/transport/internet/websocket"
)

// localSocksPort is the port xray-core SOCKS5 proxy listens on.
const localSocksPort = 10808

// Connection states exposed to the mobile app via TurboModule events.
const (
	StateDisconnected      = "disconnected"
	StateConnecting        = "connecting"
	StateConnected         = "connected"
	StateDisconnecting     = "disconnecting"
	StateReconnecting      = "reconnecting"
	StateSwitchingProtocol = "switching_protocol"
	StateError             = "error"
)

// ConnectConfig is the configuration passed from the mobile app to establish a tunnel.
type ConnectConfig struct {
	ServerAddress string               `json:"server_address"`
	ServerPort    int                  `json:"server_port"`
	Protocol      string               `json:"protocol"`
	UserID        string               `json:"user_id"`
	Reality       *RealityClientConfig `json:"reality,omitempty"`
	// WebSocket holds CDN transport settings for "vless-ws" protocol.
	// When set, traffic is routed through Cloudflare CDN via WebSocket+TLS.
	WebSocket *WebSocketConfig `json:"websocket,omitempty"`
	// AWG holds AmneziaWG configuration. Present only when Protocol is "amneziawg".
	// When set, the tunnel bypasses xray-core entirely and routes all traffic
	// through the WireGuard device at the TUN layer — no SOCKS5 proxy involved.
	AWG *AWGConfig `json:"awg,omitempty"`
	// ExcludedDomains lists domains that should bypass the VPN and go direct.
	// Used on iOS for domain-based split tunneling (Android uses per-app exclusion
	// via VpnService.Builder.addDisallowedApplication instead).
	ExcludedDomains []string `json:"excluded_domains,omitempty"`
}

// RealityClientConfig holds client-side REALITY settings.
type RealityClientConfig struct {
	PublicKey   string `json:"public_key"`
	ShortID     string `json:"short_id"`
	ServerName  string `json:"server_name"`
	Fingerprint string `json:"fingerprint"`
}

// WebSocketConfig holds CDN/WebSocket transport settings.
// Used when Protocol is "vless-ws": the client connects to a Cloudflare-proxied
// domain over HTTPS/WebSocket. Cloudflare terminates TLS and forwards the
// WebSocket connection to the origin server running xray-core.
type WebSocketConfig struct {
	// Host is the Cloudflare-proxied CDN domain, e.g. "vpn.example.com".
	// This is used as both the TLS SNI and the WebSocket Host header.
	Host string `json:"host"`
	// Path is the WebSocket upgrade path, e.g. "/ws".
	Path string `json:"path"`
}

// TunnelStatus holds the current state of the VPN tunnel.
type TunnelStatus struct {
	State       string `json:"state"`
	ServerAddr  string `json:"server_addr"`
	Protocol    string `json:"protocol"`
	ConnectedAt int64  `json:"connected_at"`
	BytesUp     int64  `json:"bytes_up"`
	BytesDown   int64  `json:"bytes_down"`
	Error       string `json:"error,omitempty"`
}

// StatusCallback is implemented by the native module (iOS/Android) to receive
// tunnel state changes. gomobile will generate the appropriate interface binding.
type StatusCallback interface {
	OnStatusChanged(statusJSON string)
}

var (
	instance *tunnelManager
	once     sync.Once
)

type tunnelManager struct {
	mu           sync.Mutex
	status       TunnelStatus
	callback     StatusCallback
	xrayInstance *core.Instance
	stats        *TrafficStats
	stopCh       chan struct{}
	connectedAt  time.Time
}

func getManager() *tunnelManager {
	once.Do(func() {
		instance = &tunnelManager{
			status: TunnelStatus{State: StateDisconnected},
		}
	})
	return instance
}

// SetStatusCallback registers a callback that receives tunnel state changes.
func SetStatusCallback(cb StatusCallback) {
	mgr := getManager()
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	mgr.callback = cb
}

// Connect establishes a VPN tunnel using the provided configuration JSON.
// Blocks until the SOCKS5 proxy is listening or an error occurs.
// The configJSON parameter is a JSON-serialized ConnectConfig.
// Returns an error string (empty on success).
func Connect(configJSON string) string {
	mgr := getManager()
	mgr.mu.Lock()

	if mgr.status.State == StateConnected || mgr.status.State == StateConnecting {
		mgr.mu.Unlock()
		return "already connected or connecting"
	}

	var config ConnectConfig
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		errStatus := TunnelStatus{
			State: StateError,
			Error: fmt.Sprintf("invalid config: %v", err),
		}
		mgr.setStatus(errStatus)
		mgr.mu.Unlock()
		mgr.notifyStatus(errStatus)
		return fmt.Sprintf("invalid config: %v", err)
	}

	// Validate required fields
	if config.ServerAddress == "" {
		errMsg := "server_address is required"
		errStatus := TunnelStatus{State: StateError, Error: errMsg}
		mgr.setStatus(errStatus)
		mgr.mu.Unlock()
		mgr.notifyStatus(errStatus)
		return errMsg
	}
	if config.ServerPort <= 0 || config.ServerPort > 65535 {
		errMsg := "invalid server_port"
		errStatus := TunnelStatus{State: StateError, Error: errMsg}
		mgr.setStatus(errStatus)
		mgr.mu.Unlock()
		mgr.notifyStatus(errStatus)
		return errMsg
	}
	// user_id is required for xray-based protocols; not used for AmneziaWG.
	if config.Protocol != "amneziawg" && config.UserID == "" {
		errMsg := "user_id is required"
		errStatus := TunnelStatus{State: StateError, Error: errMsg}
		mgr.setStatus(errStatus)
		mgr.mu.Unlock()
		mgr.notifyStatus(errStatus)
		return errMsg
	}

	connectingStatus := TunnelStatus{
		State:      StateConnecting,
		ServerAddr: fmt.Sprintf("%s:%d", config.ServerAddress, config.ServerPort),
		Protocol:   config.Protocol,
	}
	mgr.setStatus(connectingStatus)

	mgr.stopCh = make(chan struct{})

	// readyCh: runTunnel sends "" on success, or an error message on failure.
	// Connect blocks on this so the caller knows xray is ready before proceeding.
	readyCh := make(chan string, 1)
	go mgr.runTunnel(config, readyCh)
	mgr.mu.Unlock()
	mgr.notifyStatus(connectingStatus)

	// Block until xray-core is ready or errored
	return <-readyCh
}

// Disconnect tears down the active VPN tunnel.
func Disconnect() string {
	mgr := getManager()
	mgr.mu.Lock()

	if mgr.status.State == StateDisconnected {
		mgr.mu.Unlock()
		return ""
	}

	disconnectingStatus := TunnelStatus{State: StateDisconnecting}
	mgr.setStatus(disconnectingStatus)

	// Close XRay-core instance (nil-safe; only set for xray-based protocols).
	if mgr.xrayInstance != nil {
		mgr.xrayInstance.Close()
		mgr.xrayInstance = nil
	}

	// Close AmneziaWG device (no-op when not running).
	stopAWGTunnel()

	// Clear stats and SOCKS auth credentials
	mgr.stats = nil
	resetSocksAuth()

	if mgr.stopCh != nil {
		close(mgr.stopCh)
		mgr.stopCh = nil
	}

	disconnectedStatus := TunnelStatus{State: StateDisconnected}
	mgr.setStatus(disconnectedStatus)
	mgr.mu.Unlock()
	mgr.notifyStatus(disconnectingStatus)
	mgr.notifyStatus(disconnectedStatus)
	return ""
}

// GetStatus returns the current tunnel status as a JSON string.
func GetStatus() string {
	mgr := getManager()
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	data, _ := json.Marshal(mgr.status)
	return string(data)
}

// runTunnel dispatches to the correct tunnel implementation based on Protocol.
//   - "amneziawg" → runAWGTunnel (WireGuard at TUN level, no xray-core)
//   - "vless-reality", "vless-ws", or anything else → runXRayTunnel
//
// Sends "" to readyCh on success, or an error string on failure.
func (m *tunnelManager) runTunnel(config ConnectConfig, readyCh chan<- string) {
	// Catch Go panics and convert to error strings so the app doesn't SIGSEGV.
	defer func() {
		if r := recover(); r != nil {
			errMsg := fmt.Sprintf("PANIC in runTunnel: %v", r)
			errStatus := TunnelStatus{State: StateError, Error: errMsg}
			m.mu.Lock()
			m.setStatus(errStatus)
			m.mu.Unlock()
			m.notifyStatus(errStatus)
			select {
			case readyCh <- errMsg:
			default:
			}
		}
	}()

	if config.Protocol == "amneziawg" {
		m.runAWGTunnel(config, readyCh)
		return
	}
	m.runXRayTunnel(config, readyCh)
}

// runAWGTunnel is the AmneziaWG (WireGuard-based) tunnel path.
//
// Unlike the xray path this function does NOT start a SOCKS5 proxy.
// Instead it registers the AWG config so that StartTunAWG() — called by
// the native module immediately after — can hand the TUN fd to the
// amneziawg-go device.  The goroutine then blocks on stopCh.
func (m *tunnelManager) runAWGTunnel(config ConnectConfig, readyCh chan<- string) {
	serverAddr := fmt.Sprintf("%s:%d", config.ServerAddress, config.ServerPort)
	protocol := config.Protocol

	if config.AWG == nil {
		errMsg := "awg config is required for amneziawg protocol"
		errStatus := TunnelStatus{State: StateError, Error: errMsg}
		m.mu.Lock()
		m.setStatus(errStatus)
		m.mu.Unlock()
		m.notifyStatus(errStatus)
		readyCh <- errMsg
		return
	}

	// Snapshot the AWG config then clear sensitive data from the outer struct.
	awgCfg := *config.AWG
	config = ConnectConfig{}

	// Store the pending AWG config so StartTunAWG() can retrieve it.
	setPendingAWGConfig(&awgCfg)

	m.mu.Lock()

	if m.stopCh == nil {
		clearPendingAWGConfig()
		m.mu.Unlock()
		readyCh <- "disconnected during connect"
		return
	}

	m.connectedAt = time.Now()
	m.stats = NewTrafficStats()
	stopCh := m.stopCh

	connectedStatus := TunnelStatus{
		State:       StateConnected,
		ServerAddr:  serverAddr,
		Protocol:    protocol,
		ConnectedAt: m.connectedAt.Unix(),
	}
	m.setStatus(connectedStatus)
	m.mu.Unlock()
	m.notifyStatus(connectedStatus)

	// Signal to the native module that it can now call StartTunAWG().
	readyCh <- ""

	// Block until Disconnect() is called.
	<-stopCh

	// Clean up any config that was never consumed (e.g. if native never called StartTunAWG).
	clearPendingAWGConfig()
}

// runXRayTunnel establishes the VPN connection via XRay-core.
// Used for "vless-reality", "vless-ws", and all future xray-based protocols.
// Sends "" to readyCh on success, or an error string on failure.
func (m *tunnelManager) runXRayTunnel(config ConnectConfig, readyCh chan<- string) {
	// Capture non-sensitive metadata before clearing config
	serverAddr := fmt.Sprintf("%s:%d", config.ServerAddress, config.ServerPort)
	protocol := config.Protocol

	// Build XRay-core client configuration based on the requested protocol.
	// "vless-ws" routes through Cloudflare CDN via WebSocket+TLS.
	// Everything else defaults to VLESS+REALITY.
	var xrayConfig map[string]interface{}
	switch config.Protocol {
	case "vless-ws":
		xrayConfig = buildWebSocketXRayConfig(config)
	default:
		xrayConfig = buildClientXRayConfig(config)
	}

	jsonConfig, err := json.Marshal(xrayConfig)
	if err != nil {
		errMsg := fmt.Sprintf("config error: %v", err)
		errStatus := TunnelStatus{State: StateError, Error: errMsg}
		m.mu.Lock()
		m.setStatus(errStatus)
		m.mu.Unlock()
		m.notifyStatus(errStatus)
		readyCh <- errMsg
		return
	}

	// Clear sensitive config from memory (defense-in-depth: UserID, keys)
	config = ConnectConfig{}
	xrayConfig = nil

	// Register Android socket protection (once) before creating xray instance
	registerDialerController()

	// Load and create XRay-core instance.
	// Use serial.LoadJSONConfig directly instead of core.LoadConfig("json", ...)
	// because the init()-based format registration is unreliable with gomobile builds.
	pbConfig, err := serial.LoadJSONConfig(bytes.NewReader(jsonConfig))
	// Zero the JSON config buffer (contains credentials)
	for i := range jsonConfig {
		jsonConfig[i] = 0
	}
	if err != nil {
		errMsg := fmt.Sprintf("load config error: %v", err)
		errStatus := TunnelStatus{State: StateError, Error: errMsg}
		m.mu.Lock()
		m.setStatus(errStatus)
		m.mu.Unlock()
		m.notifyStatus(errStatus)
		readyCh <- errMsg
		return
	}

	xrayInst, err := core.New(pbConfig)
	if err != nil {
		errMsg := fmt.Sprintf("xray init error: %v", err)
		errStatus := TunnelStatus{State: StateError, Error: errMsg}
		m.mu.Lock()
		m.setStatus(errStatus)
		m.mu.Unlock()
		m.notifyStatus(errStatus)
		readyCh <- errMsg
		return
	}

	// Start the local SOCKS5 proxy (XRay-core listens on localhost:10808)
	if err := xrayInst.Start(); err != nil {
		errMsg := fmt.Sprintf("xray start error: %v", err)
		errStatus := TunnelStatus{State: StateError, Error: errMsg}
		m.mu.Lock()
		m.setStatus(errStatus)
		m.mu.Unlock()
		m.notifyStatus(errStatus)
		readyCh <- errMsg
		return
	}

	// Connected successfully — acquire lock, store instance, capture stopCh
	m.mu.Lock()

	// Check if Disconnect() was called while we were starting
	if m.stopCh == nil {
		xrayInst.Close()
		m.mu.Unlock()
		readyCh <- "disconnected during connect"
		return
	}

	m.xrayInstance = xrayInst
	m.connectedAt = time.Now()
	m.stats = NewTrafficStats()
	stopCh := m.stopCh // capture under lock to avoid race

	connectedStatus := TunnelStatus{
		State:       StateConnected,
		ServerAddr:  serverAddr,
		Protocol:    protocol,
		ConnectedAt: m.connectedAt.Unix(),
	}
	m.setStatus(connectedStatus)
	m.mu.Unlock()
	m.notifyStatus(connectedStatus)

	// Signal that xray-core is ready — the SOCKS5 proxy is now accepting connections
	readyCh <- ""

	// Wait for disconnect signal
	<-stopCh
}

// setStatus updates m.status. Must be called with m.mu held.
// It does NOT invoke the callback — call notifyStatus after releasing the lock.
func (m *tunnelManager) setStatus(status TunnelStatus) {
	m.status = status
}

// notifyStatus marshals status and fires the callback WITHOUT holding the lock.
// Call this after releasing m.mu to avoid deadlocks when the callback calls
// back into Go (e.g. GetStatus).
func (m *tunnelManager) notifyStatus(status TunnelStatus) {
	m.mu.Lock()
	cb := m.callback
	m.mu.Unlock()
	if cb != nil {
		data, _ := json.Marshal(status)
		cb.OnStatusChanged(string(data))
	}
}
