package internal

import (
	"fmt"
	"io"
	"net"
	"os/exec"
	"sync"

	"go.uber.org/zap"
)

// AWGServerConfig holds the AmneziaWG server-side configuration.
// It is populated from the "awg" section of config.json.
type AWGServerConfig struct {
	// Enabled controls whether the AWG server starts alongside xray-core.
	Enabled bool `json:"enabled"`

	// ListenPort is the UDP port the WireGuard interface binds to (default 51820).
	ListenPort int `json:"listen_port"`

	// PrivateKey is the server's Base64-encoded X25519 private key.
	PrivateKey string `json:"private_key"`

	// PublicKey is the server's Base64-encoded X25519 public key.
	// Derived from PrivateKey; shared with clients so they can authenticate the server.
	PublicKey string `json:"public_key"`

	// Address is the WireGuard interface address in CIDR notation, e.g. "10.8.0.1/24".
	Address string `json:"address"`

	// AmneziaWG obfuscation parameters — must match the client configuration exactly.
	// See AWGConfig in client-tunnel/awg.go for field descriptions.
	Jc   int `json:"jc"`
	Jmin int `json:"jmin"`
	Jmax int `json:"jmax"`
	S1   int `json:"s1"`
	S2   int `json:"s2"`
	H1   int `json:"h1"`
	H2   int `json:"h2"`
	H3   int `json:"h3"`
	H4   int `json:"h4"`
}

// AWGServer manages the lifecycle of the AmneziaWG server interface.
//
// The server uses the system's WireGuard / AmneziaWG tooling (wg / awg CLI)
// through the kernel module or a userspace implementation.  The interface is
// created via `ip link`, configured via `wg setconf`, and torn down on Stop.
//
// This design keeps the Go binary free of CGo and kernel dependencies while
// still orchestrating the interface lifecycle cleanly from a single place.
type AWGServer struct {
	config    *AWGServerConfig
	logger    *zap.Logger
	ifaceName string
	mu        sync.Mutex
	running   bool
}

// NewAWGServer creates a new AWGServer.  Call Start() to bring the interface up.
func NewAWGServer(config *AWGServerConfig, logger *zap.Logger) *AWGServer {
	return &AWGServer{
		config:    config,
		logger:    logger,
		ifaceName: "awg0",
	}
}

// Start creates and configures the AmneziaWG network interface.
//
// Sequence:
//  1. Add a WireGuard-type network interface named "awg0".
//  2. Set the interface address from config.Address.
//  3. Apply the WireGuard/AmneziaWG configuration (keys, port, obfuscation params).
//  4. Bring the interface up.
//
// Requires NET_ADMIN capability (provided by the Docker container's cap_add).
func (s *AWGServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("awg server: interface %s is already running", s.ifaceName)
	}

	if err := s.config.validate(); err != nil {
		return fmt.Errorf("awg server: invalid config: %w", err)
	}

	s.logger.Info("starting amneziawg server",
		zap.String("interface", s.ifaceName),
		zap.Int("listen_port", s.config.ListenPort),
		zap.String("address", s.config.Address),
	)

	// Step 1: create the network interface.
	// Try "awg" link type first (kernel amneziawg module); fall back to "wireguard".
	if err := s.createInterface(); err != nil {
		return fmt.Errorf("awg server: create interface: %w", err)
	}

	// Step 2: assign the IP address.
	if err := runCmd("ip", "address", "add", s.config.Address, "dev", s.ifaceName); err != nil {
		s.destroyInterface() //nolint:errcheck — best-effort cleanup
		return fmt.Errorf("awg server: assign address: %w", err)
	}

	// Step 3: apply WireGuard / AmneziaWG configuration via wg(8).
	if err := s.applyWGConfig(); err != nil {
		s.destroyInterface() //nolint:errcheck
		return fmt.Errorf("awg server: apply wg config: %w", err)
	}

	// Step 4: bring the interface up.
	if err := runCmd("ip", "link", "set", "up", "dev", s.ifaceName); err != nil {
		s.destroyInterface() //nolint:errcheck
		return fmt.Errorf("awg server: bring interface up: %w", err)
	}

	s.running = true
	s.logger.Info("amneziawg server started",
		zap.String("interface", s.ifaceName),
		zap.Int("listen_port", s.config.ListenPort),
	)
	return nil
}

// Stop removes the AmneziaWG network interface.
func (s *AWGServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	if err := s.destroyInterface(); err != nil {
		s.logger.Error("awg server: error destroying interface",
			zap.String("interface", s.ifaceName),
			zap.Error(err),
		)
		return err
	}

	s.running = false
	s.logger.Info("amneziawg server stopped", zap.String("interface", s.ifaceName))
	return nil
}

// IsRunning reports whether the AWG server interface is currently active.
func (s *AWGServer) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// createInterface adds a WireGuard (or AmneziaWG) link.
// Tries the "amneziawg" link type first; if the kernel module is not loaded
// it falls back to the standard "wireguard" type.  Operators who want full
// AmneziaWG obfuscation must ensure the amneziawg kernel module is installed.
func (s *AWGServer) createInterface() error {
	err := runCmd("ip", "link", "add", "dev", s.ifaceName, "type", "amneziawg")
	if err == nil {
		s.logger.Info("created amneziawg interface (kernel module)", zap.String("iface", s.ifaceName))
		return nil
	}

	s.logger.Warn("amneziawg kernel module unavailable, falling back to wireguard type",
		zap.String("iface", s.ifaceName),
		zap.Error(err),
	)

	if fallbackErr := runCmd("ip", "link", "add", "dev", s.ifaceName, "type", "wireguard"); fallbackErr != nil {
		return fmt.Errorf("wireguard fallback failed: %w (original error: %v)", fallbackErr, err)
	}

	s.logger.Warn("interface created as standard wireguard — obfuscation parameters will NOT take effect",
		zap.String("iface", s.ifaceName),
	)
	return nil
}

// destroyInterface removes the network interface.
func (s *AWGServer) destroyInterface() error {
	return runCmd("ip", "link", "del", "dev", s.ifaceName)
}

// applyWGConfig writes a wg(8)-compatible configuration and pipes it through
// `wg setconf`.  The AmneziaWG obfuscation parameters are written as
// Interface-level keys that the amneziawg-patched wg(8) understands.
func (s *AWGServer) applyWGConfig() error {
	cfg := fmt.Sprintf(
		"[Interface]\n"+
			"PrivateKey = %s\n"+
			"ListenPort = %d\n"+
			"Jc = %d\n"+
			"Jmin = %d\n"+
			"Jmax = %d\n"+
			"S1 = %d\n"+
			"S2 = %d\n"+
			"H1 = %d\n"+
			"H2 = %d\n"+
			"H3 = %d\n"+
			"H4 = %d\n",
		s.config.PrivateKey,
		s.config.ListenPort,
		s.config.Jc,
		s.config.Jmin,
		s.config.Jmax,
		s.config.S1,
		s.config.S2,
		s.config.H1,
		s.config.H2,
		s.config.H3,
		s.config.H4,
	)

	// wg setconf reads from stdin.
	cmd := exec.Command("wg", "setconf", s.ifaceName, "/dev/stdin")
	cmd.Stdin = newStringReader(cfg)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wg setconf: %w (output: %s)", err, string(out))
	}

	return nil
}

// validate checks that required AWGServerConfig fields are present.
func (c *AWGServerConfig) validate() error {
	if c.ListenPort <= 0 || c.ListenPort > 65535 {
		return fmt.Errorf("listen_port must be 1-65535, got %d", c.ListenPort)
	}
	if c.PrivateKey == "" {
		return fmt.Errorf("private_key is required")
	}
	if c.Address == "" {
		return fmt.Errorf("address is required")
	}
	if _, _, err := net.ParseCIDR(c.Address); err != nil {
		return fmt.Errorf("address is not a valid CIDR: %w", err)
	}
	return nil
}

// runCmd executes a shell command and returns a wrapped error on failure.
func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("command %q failed: %w (output: %s)", name, err, string(out))
	}
	return nil
}

// newStringReader wraps a string in an io.Reader — avoids importing strings
// package just for strings.NewReader (it is already available, but keeping
// this explicit makes the dependency clear).
func newStringReader(s string) *stringReader {
	return &stringReader{data: []byte(s)}
}

type stringReader struct {
	data []byte
	pos  int
}

func (r *stringReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
