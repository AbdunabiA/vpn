package internal

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config holds all tunnel server configuration.
type Config struct {
	// Port to listen on (typically 443 to mimic HTTPS)
	Port int `json:"port"`

	// Protocol: "vless-reality" or "amneziawg"
	Protocol string `json:"protocol"`

	// REALITY configuration (used when Protocol is "vless-reality")
	Reality RealityConfig `json:"reality"`

	// Health check endpoint port (separate from tunnel port)
	HealthPort int `json:"health_port"`
}

// RealityConfig holds VLESS+REALITY specific settings.
// REALITY works by impersonating a real TLS server during the handshake.
// Only clients with the correct private key can complete the connection.
type RealityConfig struct {
	// X25519 private key for REALITY handshake (base64)
	PrivateKey string `json:"private_key"`

	// Corresponding public key (shared with clients)
	PublicKey string `json:"public_key"`

	// Short IDs for client authentication (hex strings)
	ShortIDs []string `json:"short_ids"`

	// Destination: the real website to impersonate (e.g., "www.microsoft.com:443")
	// REALITY will proxy the TLS handshake to this server for unauthenticated clients
	Dest string `json:"dest"`

	// Server names (SNI values) to accept in TLS ClientHello
	ServerNames []string `json:"server_names"`
}

// LoadConfig reads and parses the configuration file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &config, nil
}

// validate checks that the configuration has all required fields.
func (c *Config) validate() error {
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", c.Port)
	}

	switch c.Protocol {
	case "vless-reality":
		if c.Reality.PrivateKey == "" {
			return fmt.Errorf("reality.private_key is required")
		}
		if c.Reality.Dest == "" {
			return fmt.Errorf("reality.dest is required")
		}
		if len(c.Reality.ServerNames) == 0 {
			return fmt.Errorf("reality.server_names must have at least one entry")
		}
		if len(c.Reality.ShortIDs) == 0 {
			return fmt.Errorf("reality.short_ids must have at least one entry")
		}
	default:
		return fmt.Errorf("unsupported protocol: %s (supported: vless-reality)", c.Protocol)
	}

	if c.HealthPort <= 0 {
		c.HealthPort = 8080
	}

	return nil
}
