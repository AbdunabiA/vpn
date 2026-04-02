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

	// XRay-core — import only what VLESS+REALITY client needs
	"github.com/xtls/xray-core/core"
	_ "github.com/xtls/xray-core/app/dispatcher"
	_ "github.com/xtls/xray-core/app/proxyman/inbound"
	_ "github.com/xtls/xray-core/app/proxyman/outbound"
	_ "github.com/xtls/xray-core/proxy/freedom"
	_ "github.com/xtls/xray-core/proxy/socks"
	_ "github.com/xtls/xray-core/proxy/vless/outbound"
	_ "github.com/xtls/xray-core/transport/internet/reality"
	_ "github.com/xtls/xray-core/transport/internet/tcp"
	_ "github.com/xtls/xray-core/infra/conf/serial"
)

// localSocksPort is the port xray-core SOCKS5 proxy listens on.
const localSocksPort = 10808

// Connection states exposed to the mobile app via TurboModule events.
const (
	StateDisconnected  = "disconnected"
	StateConnecting    = "connecting"
	StateConnected     = "connected"
	StateDisconnecting = "disconnecting"
	StateReconnecting  = "reconnecting"
	StateError         = "error"
)

// ConnectConfig is the configuration passed from the mobile app to establish a tunnel.
type ConnectConfig struct {
	ServerAddress   string               `json:"server_address"`
	ServerPort      int                  `json:"server_port"`
	Protocol        string               `json:"protocol"`
	UserID          string               `json:"user_id"`
	Reality         *RealityClientConfig `json:"reality,omitempty"`
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
		mgr.setStatus(TunnelStatus{
			State: StateError,
			Error: fmt.Sprintf("invalid config: %v", err),
		})
		mgr.mu.Unlock()
		return fmt.Sprintf("invalid config: %v", err)
	}

	// Validate required fields
	if config.ServerAddress == "" {
		errMsg := "server_address is required"
		mgr.setStatus(TunnelStatus{State: StateError, Error: errMsg})
		mgr.mu.Unlock()
		return errMsg
	}
	if config.ServerPort <= 0 || config.ServerPort > 65535 {
		errMsg := "invalid server_port"
		mgr.setStatus(TunnelStatus{State: StateError, Error: errMsg})
		mgr.mu.Unlock()
		return errMsg
	}
	if config.UserID == "" {
		errMsg := "user_id is required"
		mgr.setStatus(TunnelStatus{State: StateError, Error: errMsg})
		mgr.mu.Unlock()
		return errMsg
	}

	mgr.setStatus(TunnelStatus{
		State:      StateConnecting,
		ServerAddr: fmt.Sprintf("%s:%d", config.ServerAddress, config.ServerPort),
		Protocol:   config.Protocol,
	})

	mgr.stopCh = make(chan struct{})

	// readyCh: runTunnel sends "" on success, or an error message on failure.
	// Connect blocks on this so the caller knows xray is ready before proceeding.
	readyCh := make(chan string, 1)
	go mgr.runTunnel(config, readyCh)
	mgr.mu.Unlock()

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

	mgr.setStatus(TunnelStatus{State: StateDisconnecting})

	// Close XRay-core instance
	if mgr.xrayInstance != nil {
		mgr.xrayInstance.Close()
		mgr.xrayInstance = nil
	}

	// Clear stats and SOCKS auth credentials
	mgr.stats = nil
	resetSocksAuth()

	if mgr.stopCh != nil {
		close(mgr.stopCh)
		mgr.stopCh = nil
	}

	mgr.setStatus(TunnelStatus{State: StateDisconnected})
	mgr.mu.Unlock()
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

// runTunnel establishes the VPN connection via XRay-core.
// Sends "" to readyCh on success, or an error string on failure.
func (m *tunnelManager) runTunnel(config ConnectConfig, readyCh chan<- string) {
	// Capture non-sensitive metadata before clearing config
	serverAddr := fmt.Sprintf("%s:%d", config.ServerAddress, config.ServerPort)
	protocol := config.Protocol

	// Build XRay-core client configuration for VLESS+REALITY
	xrayConfig := buildClientXRayConfig(config)

	jsonConfig, err := json.Marshal(xrayConfig)
	if err != nil {
		errMsg := fmt.Sprintf("config error: %v", err)
		m.mu.Lock()
		m.setStatus(TunnelStatus{State: StateError, Error: errMsg})
		m.mu.Unlock()
		readyCh <- errMsg
		return
	}

	// Clear sensitive config from memory (defense-in-depth: UserID, keys)
	config = ConnectConfig{}
	xrayConfig = nil

	// Register Android socket protection (once) before creating xray instance
	registerDialerController()

	// Load and create XRay-core instance
	pbConfig, err := core.LoadConfig("json", bytes.NewReader(jsonConfig))
	// Zero the JSON config buffer (contains credentials)
	for i := range jsonConfig {
		jsonConfig[i] = 0
	}
	if err != nil {
		errMsg := fmt.Sprintf("load config error: %v", err)
		m.mu.Lock()
		m.setStatus(TunnelStatus{State: StateError, Error: errMsg})
		m.mu.Unlock()
		readyCh <- errMsg
		return
	}

	xrayInst, err := core.New(pbConfig)
	if err != nil {
		errMsg := fmt.Sprintf("xray init error: %v", err)
		m.mu.Lock()
		m.setStatus(TunnelStatus{State: StateError, Error: errMsg})
		m.mu.Unlock()
		readyCh <- errMsg
		return
	}

	// Start the local SOCKS5 proxy (XRay-core listens on localhost:10808)
	if err := xrayInst.Start(); err != nil {
		errMsg := fmt.Sprintf("xray start error: %v", err)
		m.mu.Lock()
		m.setStatus(TunnelStatus{State: StateError, Error: errMsg})
		m.mu.Unlock()
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

	m.setStatus(TunnelStatus{
		State:       StateConnected,
		ServerAddr:  serverAddr,
		Protocol:    protocol,
		ConnectedAt: m.connectedAt.Unix(),
	})
	m.mu.Unlock()

	// Signal that xray-core is ready — the SOCKS5 proxy is now accepting connections
	readyCh <- ""

	// Wait for disconnect signal
	<-stopCh
}

// setStatus updates the tunnel status and notifies the callback.
// Must be called with m.mu held. The callback is invoked outside the lock
// to prevent deadlocks when the callback calls back into Go.
func (m *tunnelManager) setStatus(status TunnelStatus) {
	m.status = status
	cb := m.callback
	if cb != nil {
		data, _ := json.Marshal(status)
		statusJSON := string(data)
		m.mu.Unlock()
		cb.OnStatusChanged(statusJSON)
		m.mu.Lock()
	}
}
