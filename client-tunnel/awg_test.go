package tunnel

import (
	"strings"
	"testing"
)

// --- buildAWGUAPI tests ---

func TestBuildAWGUAPIHappyPath(t *testing.T) {
	cfg := AWGConfig{
		PrivateKey: "PrivateKeyBase64==",
		PublicKey:  "PublicKeyBase64===",
		Endpoint:   "1.2.3.4:51820",
		AllowedIPs: "0.0.0.0/0, ::/0",
		Jc:         5,
		Jmin:       50,
		Jmax:       1000,
		S1:         10,
		S2:         20,
		H1:         11,
		H2:         22,
		H3:         33,
		H4:         44,
	}

	uapi, err := buildAWGUAPI(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Core WireGuard fields must be present.
	for _, required := range []string{
		"private_key=PrivateKeyBase64==",
		"public_key=PublicKeyBase64===",
		"endpoint=1.2.3.4:51820",
		"allowed_ip=0.0.0.0/0, ::/0",
	} {
		if !strings.Contains(uapi, required) {
			t.Errorf("UAPI output missing required field %q\nFull output:\n%s", required, uapi)
		}
	}

	// AmneziaWG obfuscation parameters.
	for _, required := range []string{
		"junk_packet_count=5",
		"junk_packet_min_size=50",
		"junk_packet_max_size=1000",
		"init_packet_junk_size=10",
		"response_packet_junk_size=20",
		"init_packet_magic_header=11",
		"response_packet_magic_header=22",
		"underload_packet_magic_header=33",
		"transport_packet_magic_header=44",
	} {
		if !strings.Contains(uapi, required) {
			t.Errorf("UAPI output missing obfuscation field %q\nFull output:\n%s", required, uapi)
		}
	}

	// Keepalive should always be set.
	if !strings.Contains(uapi, "persistent_keepalive_interval=25") {
		t.Error("UAPI output missing persistent_keepalive_interval")
	}
}

func TestBuildAWGUAPIWithPresharedKey(t *testing.T) {
	cfg := AWGConfig{
		PrivateKey:   "PrivKey==",
		PublicKey:    "PubKey===",
		PresharedKey: "PSK======",
		Endpoint:     "1.2.3.4:51820",
		AllowedIPs:   "0.0.0.0/0",
	}

	uapi, err := buildAWGUAPI(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(uapi, "preshared_key=PSK======") {
		t.Error("UAPI output missing preshared_key when PresharedKey is set")
	}
}

func TestBuildAWGUAPIWithoutPresharedKey(t *testing.T) {
	cfg := AWGConfig{
		PrivateKey: "PrivKey==",
		PublicKey:  "PubKey===",
		Endpoint:   "1.2.3.4:51820",
		AllowedIPs: "0.0.0.0/0",
	}

	uapi, err := buildAWGUAPI(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(uapi, "preshared_key=") {
		t.Error("UAPI output must not contain preshared_key when PresharedKey is empty")
	}
}

func TestBuildAWGUAPIZeroObfuscationParams(t *testing.T) {
	// All obfuscation params zero means standard WireGuard behaviour.
	cfg := AWGConfig{
		PrivateKey: "PrivKey==",
		PublicKey:  "PubKey===",
		Endpoint:   "1.2.3.4:51820",
		AllowedIPs: "0.0.0.0/0",
		// Jc, Jmin, Jmax, S1, S2, H1-H4 all default to 0.
	}

	uapi, err := buildAWGUAPI(cfg)
	if err != nil {
		t.Fatalf("unexpected error building zero-param config: %v", err)
	}

	// Keys should still be present even when params are zero.
	if !strings.Contains(uapi, "junk_packet_count=0") {
		t.Error("UAPI should include junk_packet_count=0 (explicit zero)")
	}
}

// --- Validation error tests ---

func TestBuildAWGUAPIMissingPrivateKey(t *testing.T) {
	cfg := AWGConfig{
		PublicKey:  "PubKey===",
		Endpoint:   "1.2.3.4:51820",
		AllowedIPs: "0.0.0.0/0",
	}

	_, err := buildAWGUAPI(cfg)
	if err == nil {
		t.Fatal("expected error for missing private_key, got nil")
	}
	if !strings.Contains(err.Error(), "private_key") {
		t.Errorf("error should mention 'private_key', got: %v", err)
	}
}

func TestBuildAWGUAPIMissingPublicKey(t *testing.T) {
	cfg := AWGConfig{
		PrivateKey: "PrivKey==",
		Endpoint:   "1.2.3.4:51820",
		AllowedIPs: "0.0.0.0/0",
	}

	_, err := buildAWGUAPI(cfg)
	if err == nil {
		t.Fatal("expected error for missing public_key, got nil")
	}
	if !strings.Contains(err.Error(), "public_key") {
		t.Errorf("error should mention 'public_key', got: %v", err)
	}
}

func TestBuildAWGUAPIMissingEndpoint(t *testing.T) {
	cfg := AWGConfig{
		PrivateKey: "PrivKey==",
		PublicKey:  "PubKey===",
		AllowedIPs: "0.0.0.0/0",
	}

	_, err := buildAWGUAPI(cfg)
	if err == nil {
		t.Fatal("expected error for missing endpoint, got nil")
	}
	if !strings.Contains(err.Error(), "endpoint") {
		t.Errorf("error should mention 'endpoint', got: %v", err)
	}
}

func TestBuildAWGUAPIMissingAllowedIPs(t *testing.T) {
	cfg := AWGConfig{
		PrivateKey: "PrivKey==",
		PublicKey:  "PubKey===",
		Endpoint:   "1.2.3.4:51820",
	}

	_, err := buildAWGUAPI(cfg)
	if err == nil {
		t.Fatal("expected error for missing allowed_ips, got nil")
	}
	if !strings.Contains(err.Error(), "allowed_ips") {
		t.Errorf("error should mention 'allowed_ips', got: %v", err)
	}
}

// --- pendingAWGConfig lifecycle tests ---

func TestPendingAWGConfigSetAndTake(t *testing.T) {
	// Clean state.
	clearPendingAWGConfig()

	cfg := &AWGConfig{
		PrivateKey: "PrivKey==",
		PublicKey:  "PubKey===",
		Endpoint:   "1.2.3.4:51820",
		AllowedIPs: "0.0.0.0/0",
	}

	setPendingAWGConfig(cfg)

	taken := takePendingAWGConfig()
	if taken == nil {
		t.Fatal("expected non-nil config after setPendingAWGConfig, got nil")
	}
	if taken.Endpoint != "1.2.3.4:51820" {
		t.Errorf("taken config endpoint mismatch: got %q", taken.Endpoint)
	}

	// Second take should return nil — the config was consumed.
	again := takePendingAWGConfig()
	if again != nil {
		t.Error("expected nil on second takePendingAWGConfig (config already consumed)")
	}
}

func TestPendingAWGConfigClear(t *testing.T) {
	cfg := &AWGConfig{Endpoint: "1.2.3.4:51820"}
	setPendingAWGConfig(cfg)
	clearPendingAWGConfig()

	taken := takePendingAWGConfig()
	if taken != nil {
		t.Error("expected nil after clearPendingAWGConfig, got a config")
	}
}

func TestTakeWhenNoPendingConfig(t *testing.T) {
	clearPendingAWGConfig()

	taken := takePendingAWGConfig()
	if taken != nil {
		t.Error("expected nil when no pending config is set")
	}
}

// --- Connect validation tests for amneziawg protocol ---

func TestConnectAWGMissingAWGField(t *testing.T) {
	resetManager()

	// Protocol is amneziawg but the "awg" JSON block is absent.
	result := Connect(`{
		"server_address": "1.2.3.4",
		"server_port": 51820,
		"protocol": "amneziawg"
	}`)

	// runAWGTunnel should return an error because config.AWG is nil.
	if result == "" {
		t.Error("expected error when amneziawg config block is missing")
		Disconnect()
	}
	if !strings.Contains(result, "awg config is required") {
		t.Errorf("expected 'awg config is required' error, got: %q", result)
	}
}

func TestConnectAWGDoesNotRequireUserID(t *testing.T) {
	resetManager()

	// user_id is absent — that's valid for amneziawg.
	// The connection will fail when runAWGTunnel notices AWG is nil (no awg block),
	// but it must NOT fail with "user_id is required".
	result := Connect(`{
		"server_address": "1.2.3.4",
		"server_port": 51820,
		"protocol": "amneziawg"
	}`)

	if strings.Contains(result, "user_id is required") {
		t.Error("amneziawg protocol must not require user_id — it uses WireGuard keys instead")
	}
}

func TestConnectXRayProtocolStillRequiresUserID(t *testing.T) {
	resetManager()

	// vless-reality must still enforce user_id.
	result := Connect(`{"server_address":"1.2.3.4","server_port":443,"protocol":"vless-reality"}`)
	if result != "user_id is required" {
		t.Errorf("vless-reality must require user_id; got: %q", result)
	}
}

// --- stopAWGTunnel no-op test ---

func TestStopAWGTunnelWhenNotRunning(t *testing.T) {
	// Calling stopAWGTunnel() when no device is running must not panic.
	awgMu.Lock()
	awgDevice = nil
	awgMu.Unlock()

	// Should be a safe no-op.
	stopAWGTunnel()
}
