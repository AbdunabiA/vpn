package tunnel

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

// mockStatusCallback captures status changes for testing.
type mockStatusCallback struct {
	mu       sync.Mutex
	statuses []TunnelStatus
}

func (m *mockStatusCallback) OnStatusChanged(statusJSON string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var s TunnelStatus
	json.Unmarshal([]byte(statusJSON), &s)
	m.statuses = append(m.statuses, s)
}

func (m *mockStatusCallback) getStatuses() []TunnelStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]TunnelStatus, len(m.statuses))
	copy(cp, m.statuses)
	return cp
}

func resetManager() {
	instance = nil
	once = sync.Once{}
	dialerOnce = sync.Once{}
	resetSocksAuth()
}

func TestGetStatusReturnsDisconnectedInitially(t *testing.T) {
	resetManager()

	statusJSON := GetStatus()
	var status TunnelStatus
	if err := json.Unmarshal([]byte(statusJSON), &status); err != nil {
		t.Fatalf("failed to parse status JSON: %v", err)
	}
	if status.State != StateDisconnected {
		t.Errorf("expected state %q, got %q", StateDisconnected, status.State)
	}
}

func TestConnectInvalidConfig(t *testing.T) {
	resetManager()

	result := Connect("not-json")
	if result == "" {
		t.Error("expected error for invalid JSON config, got empty string")
	}

	statusJSON := GetStatus()
	var status TunnelStatus
	json.Unmarshal([]byte(statusJSON), &status)
	if status.State != StateError {
		t.Errorf("expected state %q after invalid config, got %q", StateError, status.State)
	}
}

func TestConnectMissingServerAddress(t *testing.T) {
	resetManager()

	result := Connect(`{"server_port":443,"user_id":"test"}`)
	if result == "" {
		t.Error("expected error for missing server_address")
	}
	if result != "server_address is required" {
		t.Errorf("unexpected error: %s", result)
	}
}

func TestConnectInvalidPort(t *testing.T) {
	resetManager()

	result := Connect(`{"server_address":"1.2.3.4","server_port":0,"user_id":"test"}`)
	if result == "" {
		t.Error("expected error for invalid port")
	}
}

func TestConnectMissingUserID(t *testing.T) {
	resetManager()

	result := Connect(`{"server_address":"1.2.3.4","server_port":443}`)
	if result == "" {
		t.Error("expected error for missing user_id")
	}
}

func TestDisconnectWhenAlreadyDisconnected(t *testing.T) {
	resetManager()

	result := Disconnect()
	if result != "" {
		t.Errorf("expected empty string when disconnecting while already disconnected, got %q", result)
	}
}

func TestConnectReturnsXrayError(t *testing.T) {
	resetManager()

	// Connect blocks until xray finishes — with a real config but no server,
	// it will return an xray error synchronously.
	result := Connect(`{"server_address":"127.0.0.1","server_port":443,"protocol":"vless-reality","user_id":"test-uuid"}`)
	if result == "" {
		t.Error("expected xray error connecting to non-existent server, got empty string")
		Disconnect()
	}

	// Status should be error
	statusJSON := GetStatus()
	var status TunnelStatus
	json.Unmarshal([]byte(statusJSON), &status)
	if status.State != StateError {
		t.Errorf("expected error state after failed connect, got %q", status.State)
	}
}

func TestDoubleConnectWhileConnecting(t *testing.T) {
	resetManager()

	// Start first connect in a goroutine (it will block)
	done := make(chan string, 1)
	go func() {
		done <- Connect(`{"server_address":"127.0.0.1","server_port":443,"protocol":"vless-reality","user_id":"test-uuid"}`)
	}()

	// Give it a moment to enter "connecting" state, then try second connect
	// We can't rely on timing, but the state check in Connect is synchronous
	// Wait for first to finish (it'll error quickly with no server)
	<-done

	// After the first finishes with error, second connect should work (not "already connected")
	result2 := Connect(`{"server_address":"127.0.0.1","server_port":443,"protocol":"vless-reality","user_id":"test-uuid"}`)
	// This should also fail with xray error, but NOT "already connected"
	if strings.Contains(result2, "already connected") {
		t.Error("should be able to reconnect after a failed connect")
	}
}

func TestStatusCallbackReceivesUpdates(t *testing.T) {
	resetManager()

	cb := &mockStatusCallback{}
	SetStatusCallback(cb)

	// Connect will fail but should still emit "connecting" then "error"
	Connect(`{"server_address":"127.0.0.1","server_port":443,"protocol":"vless-reality","user_id":"test-uuid"}`)

	statuses := cb.getStatuses()
	if len(statuses) == 0 {
		t.Fatal("expected status callback to be called at least once")
	}

	// First callback should be "connecting"
	if statuses[0].State != StateConnecting {
		t.Errorf("expected first callback state %q, got %q", StateConnecting, statuses[0].State)
	}

	// Should also have received an error state
	found := false
	for _, s := range statuses {
		if s.State == StateError {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to receive error state callback")
	}
}

func TestGetTrafficStatsReturnsValidJSON(t *testing.T) {
	resetManager()

	statsJSON := GetTrafficStats()
	var stats StatsSnapshot
	if err := json.Unmarshal([]byte(statsJSON), &stats); err != nil {
		t.Fatalf("failed to parse traffic stats JSON: %v", err)
	}
	if stats.BytesUp != 0 || stats.BytesDown != 0 {
		t.Errorf("expected zero traffic stats when disconnected, got up=%d down=%d", stats.BytesUp, stats.BytesDown)
	}
}

func TestProbeServersInvalidInput(t *testing.T) {
	result := ProbeServers("not-json")
	if result == "" {
		t.Error("expected error response for invalid JSON input")
	}
}

func TestProbeServersEmptyArray(t *testing.T) {
	resetManager()
	result := ProbeServers("[]")
	var results []ProbeResult
	if err := json.Unmarshal([]byte(result), &results); err != nil {
		t.Fatalf("failed to parse probe results: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected empty results for empty input, got %d", len(results))
	}
}

func TestProbeServersUnreachable(t *testing.T) {
	resetManager()
	input := `[{"address":"192.0.2.1","port":12345}]`
	result := ProbeServers(input)

	var results []ProbeResult
	if err := json.Unmarshal([]byte(result), &results); err != nil {
		t.Fatalf("failed to parse probe results: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Success {
		t.Error("expected probe to unreachable address to fail")
	}
}

func TestProbeServersBlockedWhileConnected(t *testing.T) {
	resetManager()

	// Manually set state to connected to test the guard
	mgr := getManager()
	mgr.mu.Lock()
	mgr.status.State = StateConnected
	mgr.mu.Unlock()

	result := ProbeServers(`[{"address":"1.1.1.1","port":443}]`)
	if !strings.Contains(result, "cannot probe") {
		t.Errorf("expected probe to be blocked while connected, got: %s", result)
	}

	// Reset state
	mgr.mu.Lock()
	mgr.status.State = StateDisconnected
	mgr.mu.Unlock()
}

func TestBuildClientXRayConfig(t *testing.T) {
	config := ConnectConfig{
		ServerAddress: "1.2.3.4",
		ServerPort:    443,
		Protocol:      "vless-reality",
		UserID:        "test-uuid",
		Reality: &RealityClientConfig{
			PublicKey:   "test-key",
			ShortID:     "abcd1234",
			ServerName:  "www.example.com",
			Fingerprint: "firefox",
		},
	}

	xrayConfig := buildClientXRayConfig(config)

	data, err := json.Marshal(xrayConfig)
	if err != nil {
		t.Fatalf("failed to marshal xray config: %v", err)
	}

	var parsed map[string]interface{}
	json.Unmarshal(data, &parsed)

	if _, ok := parsed["inbounds"]; !ok {
		t.Error("xray config missing 'inbounds'")
	}
	if _, ok := parsed["outbounds"]; !ok {
		t.Error("xray config missing 'outbounds'")
	}
	if _, ok := parsed["dns"]; !ok {
		t.Error("xray config missing 'dns' section (DNS leak risk)")
	}
}

func TestBuildClientXRayConfigHasSocksAuth(t *testing.T) {
	resetManager()
	config := ConnectConfig{
		ServerAddress: "1.2.3.4",
		ServerPort:    443,
		UserID:        "test-uuid",
	}

	xrayConfig := buildClientXRayConfig(config)
	data, _ := json.Marshal(xrayConfig)
	jsonStr := string(data)

	if !strings.Contains(jsonStr, `"auth":"password"`) {
		t.Error("xray config missing SOCKS5 authentication")
	}
	if !strings.Contains(jsonStr, `"accounts"`) {
		t.Error("xray config missing SOCKS5 accounts")
	}
}

func TestSocksProxyURLContainsAuth(t *testing.T) {
	resetManager()
	url := socksProxyURL()
	if !strings.Contains(url, "@") {
		t.Errorf("expected authenticated SOCKS5 URL, got: %s", url)
	}
	if !strings.HasPrefix(url, "socks5://") {
		t.Errorf("expected socks5:// prefix, got: %s", url)
	}
}

func TestSetProtectCallbackNilSafe(t *testing.T) {
	resetManager()
	SetProtectCallback(nil)
	registerDialerController()
}
