// Package tunnel provides the public API for the VPN tunnel library.
// This package is compiled via gomobile to produce:
//   - Android: tunnel.aar
//   - iOS: Tunnel.xcframework
//
// Build commands:
//   gomobile bind -target=android -o tunnel.aar ./
//   gomobile bind -target=ios -o Tunnel.xcframework ./
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
	ServerAddress string               `json:"server_address"`
	ServerPort    int                  `json:"server_port"`
	Protocol      string               `json:"protocol"`
	UserID        string               `json:"user_id"`
	Reality       *RealityClientConfig `json:"reality,omitempty"`
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
// The configJSON parameter is a JSON-serialized ConnectConfig.
// Returns an error string (empty on success).
func Connect(configJSON string) string {
	mgr := getManager()
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	if mgr.status.State == StateConnected || mgr.status.State == StateConnecting {
		return "already connected or connecting"
	}

	var config ConnectConfig
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		mgr.updateStatus(TunnelStatus{
			State: StateError,
			Error: fmt.Sprintf("invalid config: %v", err),
		})
		return fmt.Sprintf("invalid config: %v", err)
	}

	mgr.updateStatus(TunnelStatus{
		State:      StateConnecting,
		ServerAddr: fmt.Sprintf("%s:%d", config.ServerAddress, config.ServerPort),
		Protocol:   config.Protocol,
	})

	mgr.stopCh = make(chan struct{})
	go mgr.runTunnel(config)

	return ""
}

// Disconnect tears down the active VPN tunnel.
func Disconnect() string {
	mgr := getManager()
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	if mgr.status.State == StateDisconnected {
		return ""
	}

	mgr.updateStatus(TunnelStatus{State: StateDisconnecting})

	// Close XRay-core instance
	if mgr.xrayInstance != nil {
		mgr.xrayInstance.Close()
		mgr.xrayInstance = nil
	}

	if mgr.stopCh != nil {
		close(mgr.stopCh)
		mgr.stopCh = nil
	}

	mgr.updateStatus(TunnelStatus{State: StateDisconnected})
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
func (m *tunnelManager) runTunnel(config ConnectConfig) {
	// Build XRay-core client configuration for VLESS+REALITY
	xrayConfig := buildClientXRayConfig(config)

	jsonConfig, err := json.Marshal(xrayConfig)
	if err != nil {
		m.mu.Lock()
		m.updateStatus(TunnelStatus{
			State: StateError,
			Error: fmt.Sprintf("config error: %v", err),
		})
		m.mu.Unlock()
		return
	}

	// Load and create XRay-core instance
	pbConfig, err := core.LoadConfig("json", bytes.NewReader(jsonConfig))
	if err != nil {
		m.mu.Lock()
		m.updateStatus(TunnelStatus{
			State: StateError,
			Error: fmt.Sprintf("load config error: %v", err),
		})
		m.mu.Unlock()
		return
	}

	xrayInst, err := core.New(pbConfig)
	if err != nil {
		m.mu.Lock()
		m.updateStatus(TunnelStatus{
			State: StateError,
			Error: fmt.Sprintf("xray init error: %v", err),
		})
		m.mu.Unlock()
		return
	}

	// Start the local SOCKS5 proxy (XRay-core listens on localhost:10808)
	if err := xrayInst.Start(); err != nil {
		m.mu.Lock()
		m.updateStatus(TunnelStatus{
			State: StateError,
			Error: fmt.Sprintf("xray start error: %v", err),
		})
		m.mu.Unlock()
		return
	}

	// Connected successfully
	m.mu.Lock()
	m.xrayInstance = xrayInst
	m.connectedAt = time.Now()
	m.updateStatus(TunnelStatus{
		State:       StateConnected,
		ServerAddr:  fmt.Sprintf("%s:%d", config.ServerAddress, config.ServerPort),
		Protocol:    config.Protocol,
		ConnectedAt: m.connectedAt.Unix(),
	})
	m.mu.Unlock()

	// Wait for disconnect signal
	<-m.stopCh
}

// updateStatus updates the tunnel status and notifies the callback.
// Must be called with m.mu held.
func (m *tunnelManager) updateStatus(status TunnelStatus) {
	m.status = status

	if m.callback != nil {
		data, _ := json.Marshal(status)
		m.callback.OnStatusChanged(string(data))
	}
}
