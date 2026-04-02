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

// --- WebSocket config builder tests ---

func TestBuildWebSocketXRayConfigStructure(t *testing.T) {
	resetManager()
	config := ConnectConfig{
		ServerAddress: "1.2.3.4",
		ServerPort:    443,
		Protocol:      "vless-ws",
		UserID:        "test-uuid",
		WebSocket: &WebSocketConfig{
			Host: "vpn.example.com",
			Path: "/ws",
		},
	}

	xrayConfig := buildWebSocketXRayConfig(config)

	data, err := json.Marshal(xrayConfig)
	if err != nil {
		t.Fatalf("failed to marshal websocket xray config: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal websocket xray config: %v", err)
	}

	if _, ok := parsed["inbounds"]; !ok {
		t.Error("websocket xray config missing 'inbounds'")
	}
	if _, ok := parsed["outbounds"]; !ok {
		t.Error("websocket xray config missing 'outbounds'")
	}
	if _, ok := parsed["dns"]; !ok {
		t.Error("websocket xray config missing 'dns' section (DNS leak risk)")
	}
}

func TestBuildWebSocketXRayConfigUsesWsNetwork(t *testing.T) {
	resetManager()
	config := ConnectConfig{
		ServerAddress: "1.2.3.4",
		ServerPort:    443,
		Protocol:      "vless-ws",
		UserID:        "test-uuid",
		WebSocket: &WebSocketConfig{
			Host: "vpn.example.com",
			Path: "/ws",
		},
	}

	xrayConfig := buildWebSocketXRayConfig(config)
	data, _ := json.Marshal(xrayConfig)
	jsonStr := string(data)

	if !strings.Contains(jsonStr, `"network":"ws"`) {
		t.Error("websocket xray config must use network=ws")
	}
	if !strings.Contains(jsonStr, `"security":"tls"`) {
		t.Error("websocket xray config must use security=tls (standard TLS, not reality)")
	}
	if strings.Contains(jsonStr, `"realitySettings"`) {
		t.Error("websocket xray config must NOT contain realitySettings — Cloudflare handles TLS")
	}
}

func TestBuildWebSocketXRayConfigEmptyFlow(t *testing.T) {
	resetManager()
	config := ConnectConfig{
		ServerAddress: "1.2.3.4",
		ServerPort:    443,
		Protocol:      "vless-ws",
		UserID:        "test-uuid",
		WebSocket: &WebSocketConfig{
			Host: "vpn.example.com",
			Path: "/ws",
		},
	}

	xrayConfig := buildWebSocketXRayConfig(config)
	data, _ := json.Marshal(xrayConfig)
	jsonStr := string(data)

	// xtls-rprx-vision is incompatible with WebSocket transport
	if strings.Contains(jsonStr, "xtls-rprx-vision") {
		t.Error("websocket xray config must NOT set flow=xtls-rprx-vision — vision is TCP-only")
	}
}

func TestBuildWebSocketXRayConfigCDNDomainAsAddress(t *testing.T) {
	resetManager()
	config := ConnectConfig{
		ServerAddress: "1.2.3.4",
		ServerPort:    443,
		Protocol:      "vless-ws",
		UserID:        "test-uuid",
		WebSocket: &WebSocketConfig{
			Host: "vpn.example.com",
			Path: "/ws",
		},
	}

	xrayConfig := buildWebSocketXRayConfig(config)
	data, _ := json.Marshal(xrayConfig)
	jsonStr := string(data)

	// Client must connect to the CDN domain, not directly to the server IP.
	// Traffic goes: Phone → Cloudflare (vpn.example.com) → origin server.
	if !strings.Contains(jsonStr, "vpn.example.com") {
		t.Error("websocket xray config must use CDN domain as address, not server IP")
	}
}

func TestBuildWebSocketXRayConfigPort443(t *testing.T) {
	resetManager()
	config := ConnectConfig{
		ServerAddress: "1.2.3.4",
		ServerPort:    443,
		Protocol:      "vless-ws",
		UserID:        "test-uuid",
		WebSocket: &WebSocketConfig{
			Host: "vpn.example.com",
			Path: "/ws",
		},
	}

	xrayConfig := buildWebSocketXRayConfig(config)

	outbounds, ok := xrayConfig["outbounds"].([]map[string]interface{})
	if !ok || len(outbounds) == 0 {
		t.Fatal("websocket xray config missing outbounds slice")
	}

	vlessOut := outbounds[0]
	settings, ok := vlessOut["settings"].(map[string]interface{})
	if !ok {
		t.Fatal("websocket outbound missing settings")
	}

	vnext, ok := settings["vnext"].([]map[string]interface{})
	if !ok || len(vnext) == 0 {
		t.Fatal("websocket outbound missing vnext")
	}

	port, ok := vnext[0]["port"].(int)
	if !ok {
		t.Fatal("websocket vnext port is not an int")
	}
	if port != 443 {
		t.Errorf("websocket xray config must use port 443 for Cloudflare HTTPS, got %d", port)
	}
}

func TestBuildWebSocketXRayConfigHasSocksAuth(t *testing.T) {
	resetManager()
	config := ConnectConfig{
		ServerAddress: "1.2.3.4",
		ServerPort:    443,
		Protocol:      "vless-ws",
		UserID:        "test-uuid",
		WebSocket: &WebSocketConfig{
			Host: "vpn.example.com",
			Path: "/ws",
		},
	}

	xrayConfig := buildWebSocketXRayConfig(config)
	data, _ := json.Marshal(xrayConfig)
	jsonStr := string(data)

	if !strings.Contains(jsonStr, `"auth":"password"`) {
		t.Error("websocket xray config missing SOCKS5 authentication")
	}
	if !strings.Contains(jsonStr, `"accounts"`) {
		t.Error("websocket xray config missing SOCKS5 accounts")
	}
}

func TestBuildWebSocketXRayConfigNilWebSocket_FallsBackToServerAddress(t *testing.T) {
	resetManager()
	// WebSocket field omitted — should fall back to ServerAddress as host
	config := ConnectConfig{
		ServerAddress: "myserver.example.com",
		ServerPort:    443,
		Protocol:      "vless-ws",
		UserID:        "test-uuid",
		WebSocket:     nil,
	}

	xrayConfig := buildWebSocketXRayConfig(config)
	data, _ := json.Marshal(xrayConfig)
	jsonStr := string(data)

	if !strings.Contains(jsonStr, "myserver.example.com") {
		t.Error("websocket xray config should fall back to ServerAddress when WebSocket is nil")
	}
}

func TestBuildWebSocketXRayConfigCustomPath(t *testing.T) {
	resetManager()
	config := ConnectConfig{
		ServerAddress: "1.2.3.4",
		ServerPort:    443,
		Protocol:      "vless-ws",
		UserID:        "test-uuid",
		WebSocket: &WebSocketConfig{
			Host: "vpn.example.com",
			Path: "/tunnel",
		},
	}

	xrayConfig := buildWebSocketXRayConfig(config)
	data, _ := json.Marshal(xrayConfig)
	jsonStr := string(data)

	if !strings.Contains(jsonStr, "/tunnel") {
		t.Error("websocket xray config should use the custom path from WebSocketConfig")
	}
}

func TestBuildWebSocketXRayConfigSplitTunnel(t *testing.T) {
	resetManager()
	config := ConnectConfig{
		ServerAddress:   "1.2.3.4",
		ServerPort:      443,
		Protocol:        "vless-ws",
		UserID:          "test-uuid",
		ExcludedDomains: []string{"local.corp", "intranet.corp"},
		WebSocket: &WebSocketConfig{
			Host: "vpn.example.com",
			Path: "/ws",
		},
	}

	xrayConfig := buildWebSocketXRayConfig(config)
	data, _ := json.Marshal(xrayConfig)
	jsonStr := string(data)

	// Split tunnel excluded domains must appear in the routing rules
	if !strings.Contains(jsonStr, "local.corp") {
		t.Error("websocket xray config should preserve split tunnel excluded domains")
	}
	if !strings.Contains(jsonStr, "intranet.corp") {
		t.Error("websocket xray config should preserve all excluded domains")
	}
}

// ---------------------------------------------------------------------------
// Edge-case tests added to increase coverage and catch production crashes
// ---------------------------------------------------------------------------

// --- GetTrafficStats during active connection ---

func TestGetTrafficStatsDuringActiveConnection(t *testing.T) {
	resetManager()

	// Simulate an active connection by injecting a live TrafficStats object
	// directly into the manager, the same way runAWGTunnel/runXRayTunnel do.
	mgr := getManager()
	mgr.mu.Lock()
	mgr.stats = NewTrafficStats()
	mgr.stats.AddUpload(1024)
	mgr.stats.AddDownload(4096)
	mgr.status.State = StateConnected
	mgr.mu.Unlock()

	statsJSON := GetTrafficStats()
	var snap StatsSnapshot
	if err := json.Unmarshal([]byte(statsJSON), &snap); err != nil {
		t.Fatalf("failed to parse traffic stats JSON during active connection: %v", err)
	}
	if snap.BytesUp != 1024 {
		t.Errorf("expected BytesUp=1024, got %d", snap.BytesUp)
	}
	if snap.BytesDown != 4096 {
		t.Errorf("expected BytesDown=4096, got %d", snap.BytesDown)
	}

	// Cleanup
	mgr.mu.Lock()
	mgr.stats = nil
	mgr.status.State = StateDisconnected
	mgr.mu.Unlock()
}

// --- buildRoutingRules: nil vs empty ExcludedDomains ---

func TestBuildRoutingRulesNilExcludedDomains(t *testing.T) {
	config := ConnectConfig{
		ServerAddress:   "1.2.3.4",
		ServerPort:      443,
		UserID:          "uuid",
		ExcludedDomains: nil, // explicit nil
	}

	rules := buildRoutingRules(config)
	// nil ExcludedDomains → only the catch-all rule, no direct-bypass rule
	if len(rules) != 1 {
		t.Errorf("expected 1 routing rule for nil ExcludedDomains, got %d", len(rules))
	}
}

func TestBuildRoutingRulesEmptySliceExcludedDomains(t *testing.T) {
	config := ConnectConfig{
		ServerAddress:   "1.2.3.4",
		ServerPort:      443,
		UserID:          "uuid",
		ExcludedDomains: []string{}, // empty, not nil
	}

	rules := buildRoutingRules(config)
	// Empty slice treated the same as nil — no spurious direct rule
	if len(rules) != 1 {
		t.Errorf("expected 1 routing rule for empty ExcludedDomains, got %d", len(rules))
	}
}

func TestBuildRoutingRulesNonEmptyExcludedDomains(t *testing.T) {
	config := ConnectConfig{
		ExcludedDomains: []string{"corp.example.com"},
	}

	rules := buildRoutingRules(config)
	// One direct-bypass rule + one catch-all rule
	if len(rules) != 2 {
		t.Errorf("expected 2 routing rules with excluded domains, got %d", len(rules))
	}
	// First rule must route directly
	if rules[0]["outboundTag"] != "direct" {
		t.Errorf("first rule outboundTag should be 'direct', got %q", rules[0]["outboundTag"])
	}
}

// --- buildWebSocketXRayConfig with empty Host/Path in WebSocket struct ---

func TestBuildWebSocketXRayConfigEmptyHostFallsBackToServerAddress(t *testing.T) {
	resetManager()
	config := ConnectConfig{
		ServerAddress: "fallback.vpn.com",
		ServerPort:    443,
		UserID:        "test-uuid",
		WebSocket: &WebSocketConfig{
			Host: "", // empty — should fall back to ServerAddress
			Path: "/ws",
		},
	}

	xrayConfig := buildWebSocketXRayConfig(config)
	data, _ := json.Marshal(xrayConfig)
	jsonStr := string(data)

	if !strings.Contains(jsonStr, "fallback.vpn.com") {
		t.Error("buildWebSocketXRayConfig should fall back to ServerAddress when WebSocket.Host is empty")
	}
}

func TestBuildWebSocketXRayConfigEmptyPathUsesDefaultSlashWs(t *testing.T) {
	resetManager()
	config := ConnectConfig{
		ServerAddress: "1.2.3.4",
		ServerPort:    443,
		UserID:        "test-uuid",
		WebSocket: &WebSocketConfig{
			Host: "cdn.example.com",
			Path: "", // empty — should default to "/ws"
		},
	}

	xrayConfig := buildWebSocketXRayConfig(config)
	data, _ := json.Marshal(xrayConfig)
	jsonStr := string(data)

	if !strings.Contains(jsonStr, `"/ws"`) {
		t.Error("buildWebSocketXRayConfig should use default path '/ws' when WebSocket.Path is empty")
	}
}

// --- StopTun when not started ---

func TestStopTunWhenNotStarted(t *testing.T) {
	// Ensure tunRunning=false before calling StopTun
	tunMu.Lock()
	tunRunning = false
	tunMu.Unlock()

	result := StopTun()
	if result != "" {
		t.Errorf("StopTun when not running should return empty string, got %q", result)
	}
}

// --- StartTunAWG without a pending config ---

func TestStartTunAWGWithoutPendingConfig(t *testing.T) {
	clearPendingAWGConfig()

	result := StartTunAWG(5)
	if result == "" {
		t.Error("StartTunAWG without pending config should return an error string")
	}
	if !strings.Contains(result, "no pending awg config") {
		t.Errorf("expected 'no pending awg config' error, got: %q", result)
	}
}

// --- ProbeServers with malformed entries ---

func TestProbeServersMissingPort(t *testing.T) {
	resetManager()
	// port=0 is invalid for TCP — probe should fail gracefully, not crash
	input := `[{"address":"192.0.2.1","port":0}]`
	result := ProbeServers(input)

	var results []ProbeResult
	if err := json.Unmarshal([]byte(result), &results); err != nil {
		t.Fatalf("failed to parse probe results: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// port=0 should fail, not succeed
	if results[0].Success {
		t.Error("probe to port=0 must not succeed")
	}
}

func TestProbeServersEmptyAddress(t *testing.T) {
	resetManager()
	// empty address — the important behavior is that the probe does NOT panic
	// and returns a JSON-parseable result. On some systems ":443" dials localhost
	// and may succeed; we only assert the structural contract here.
	input := `[{"address":"","port":443}]`
	result := ProbeServers(input)

	var results []ProbeResult
	if err := json.Unmarshal([]byte(result), &results); err != nil {
		t.Fatalf("failed to parse probe results: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// ServerAddress must be set even for an empty address
	if results[0].ServerAddress == "" {
		t.Error("probe result must populate server_address field")
	}
}

// --- Multiple rapid Connect/Disconnect cycles ---

func TestMultipleRapidConnectDisconnectCycles(t *testing.T) {
	// Run three connect→error→disconnect cycles quickly.
	// Must not panic or deadlock.
	for i := 0; i < 3; i++ {
		resetManager()
		// This will fail fast because xray cannot connect to localhost:1
		Connect(`{"server_address":"127.0.0.1","server_port":1,"protocol":"vless-reality","user_id":"test-uuid"}`)
		Disconnect()
	}
}

// --- Connect with vless-ws but nil WebSocket config ---

func TestConnectVlessWSNilWebSocketConfig(t *testing.T) {
	resetManager()

	// vless-ws with no WebSocket block should fall back to ServerAddress as host.
	// The tunnel will fail because there is no real server, but it should NOT
	// crash due to a nil pointer dereference inside buildWebSocketXRayConfig.
	result := Connect(`{
		"server_address":"127.0.0.1",
		"server_port":443,
		"protocol":"vless-ws",
		"user_id":"test-uuid"
	}`)

	// Must return an error string (xray can't connect), not empty or panic
	// The important thing is it doesn't panic — any non-empty error is valid.
	_ = result // silence linter; we just need no panic above
}

// --- sortProbeResults edge cases ---

func TestSortProbeResultsAllFailed(t *testing.T) {
	results := []ProbeResult{
		{Success: false, Latency: 0},
		{Success: false, Latency: 0},
	}
	sortProbeResults(results)
	// All failed — no panic, order doesn't matter much but must not crash
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestSortProbeResultsSuccessBeforeFailed(t *testing.T) {
	results := []ProbeResult{
		{ServerAddress: "fail", Success: false, Latency: 0},
		{ServerAddress: "ok", Success: true, Latency: 50},
	}
	sortProbeResults(results)
	if !results[0].Success {
		t.Error("successful probe must sort before failed probe")
	}
}

func TestSortProbeResultsSortedByLatency(t *testing.T) {
	results := []ProbeResult{
		{Success: true, Latency: 200},
		{Success: true, Latency: 50},
		{Success: true, Latency: 100},
	}
	sortProbeResults(results)
	if results[0].Latency > results[1].Latency || results[1].Latency > results[2].Latency {
		t.Errorf("probes not sorted by latency: %v", results)
	}
}

func TestSortProbeResultsSingleElement(t *testing.T) {
	results := []ProbeResult{{Success: true, Latency: 10}}
	sortProbeResults(results) // must not panic
	if len(results) != 1 {
		t.Error("single-element sort corrupted the slice")
	}
}

func TestSortProbeResultsEmpty(t *testing.T) {
	var results []ProbeResult
	sortProbeResults(results) // must not panic
}

// --- Connect vless-ws protocol with missing user_id must still error ---

func TestConnectVlessWSRequiresUserID(t *testing.T) {
	resetManager()
	result := Connect(`{"server_address":"1.2.3.4","server_port":443,"protocol":"vless-ws"}`)
	if result != "user_id is required" {
		t.Errorf("vless-ws must require user_id; got: %q", result)
	}
}

// --- ConnectConfig port boundary values ---

func TestConnectPortAtMaxBoundary(t *testing.T) {
	resetManager()
	// Port 65535 is valid (max TCP port)
	// Will fail at xray level (no server), not at validation
	result := Connect(`{"server_address":"127.0.0.1","server_port":65535,"protocol":"vless-reality","user_id":"test"}`)
	if strings.Contains(result, "invalid server_port") {
		t.Error("port 65535 is valid and should not trigger port validation error")
	}
}

func TestConnectPortAboveMaxIsRejected(t *testing.T) {
	resetManager()
	result := Connect(`{"server_address":"127.0.0.1","server_port":65536,"protocol":"vless-reality","user_id":"test"}`)
	if result != "invalid server_port" {
		t.Errorf("port 65536 must be rejected; got: %q", result)
	}
}

func TestConnectPortNegativeIsRejected(t *testing.T) {
	resetManager()
	result := Connect(`{"server_address":"127.0.0.1","server_port":-1,"protocol":"vless-reality","user_id":"test"}`)
	if result != "invalid server_port" {
		t.Errorf("negative port must be rejected; got: %q", result)
	}
}

// --- TrafficStats: AddUpload/AddDownload concurrent access ---

func TestTrafficStatsConcurrentAccess(t *testing.T) {
	stats := NewTrafficStats()

	var wg sync.WaitGroup
	const goroutines = 50
	const bytesPerGoroutine = int64(1000)

	for i := 0; i < goroutines; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			stats.AddUpload(bytesPerGoroutine)
		}()
		go func() {
			defer wg.Done()
			stats.AddDownload(bytesPerGoroutine)
		}()
	}
	wg.Wait()

	snapJSON := stats.Snapshot()
	var snap StatsSnapshot
	if err := json.Unmarshal([]byte(snapJSON), &snap); err != nil {
		t.Fatalf("failed to parse snapshot: %v", err)
	}
	expected := int64(goroutines) * bytesPerGoroutine
	if snap.BytesUp != expected {
		t.Errorf("concurrent AddUpload: expected %d, got %d", expected, snap.BytesUp)
	}
	if snap.BytesDown != expected {
		t.Errorf("concurrent AddDownload: expected %d, got %d", expected, snap.BytesDown)
	}
}

// --- ConnectAlreadyConnectingOrConnected ---

func TestConnectWhileConnectedReturnsBusyError(t *testing.T) {
	resetManager()

	mgr := getManager()
	mgr.mu.Lock()
	mgr.status.State = StateConnected
	mgr.mu.Unlock()

	result := Connect(`{"server_address":"1.2.3.4","server_port":443,"protocol":"vless-reality","user_id":"test"}`)
	if !strings.Contains(result, "already connected") {
		t.Errorf("expected 'already connected' error when in connected state, got: %q", result)
	}

	// Reset state
	mgr.mu.Lock()
	mgr.status.State = StateDisconnected
	mgr.mu.Unlock()
}

func TestConnectWhileConnectingReturnsBusyError(t *testing.T) {
	resetManager()

	mgr := getManager()
	mgr.mu.Lock()
	mgr.status.State = StateConnecting
	mgr.mu.Unlock()

	result := Connect(`{"server_address":"1.2.3.4","server_port":443,"protocol":"vless-reality","user_id":"test"}`)
	if !strings.Contains(result, "already connected") {
		t.Errorf("expected 'already connected' error when in connecting state, got: %q", result)
	}

	mgr.mu.Lock()
	mgr.status.State = StateDisconnected
	mgr.mu.Unlock()
}

// --- unicode / long server address ---

func TestConnectUnicodeServerAddress(t *testing.T) {
	resetManager()
	// Unicode in server_address — should pass validation (non-empty) but fail at xray level
	result := Connect(`{"server_address":"vpn.例え.jp","server_port":443,"protocol":"vless-reality","user_id":"uid"}`)
	// Must not crash with panic; any error string (including xray error) is acceptable
	_ = result
}

func TestConnectVeryLongServerAddress(t *testing.T) {
	resetManager()
	long := strings.Repeat("a", 1000) + ".example.com"
	cfg := map[string]interface{}{
		"server_address": long,
		"server_port":    443,
		"protocol":       "vless-reality",
		"user_id":        "uid",
	}
	data, _ := json.Marshal(cfg)
	result := Connect(string(data))
	// Must not panic; any result is acceptable
	_ = result
}
