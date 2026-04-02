package handler

import (
	"errors"

	"vpnapp/server/api/internal/model"
	"vpnapp/server/api/internal/repository"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ServerConfig is the connection configuration sent to clients.
type ServerConfig struct {
	ServerAddress string               `json:"server_address"`
	ServerPort    int                  `json:"server_port"`
	Protocol      string               `json:"protocol"`
	UserID        string               `json:"user_id"`
	Reality       *RealityClientConfig `json:"reality,omitempty"`
	// WebSocket is present only when the server has WebSocket CDN transport configured.
	// Clients should use protocol "vless-ws" with this config when available and when
	// direct REALITY connections fail.
	WebSocket *WebSocketClientConfig `json:"websocket,omitempty"`
	// AWG is present only when the server has AmneziaWG configured (awg_public_key IS NOT NULL).
	// When present, the client may connect via protocol "amneziawg" instead of VLESS.
	// AmneziaWG is a WireGuard variant with anti-DPI obfuscation — it does not use xray-core.
	AWG *AWGClientConfig `json:"awg,omitempty"`
}

// AWGClientConfig holds everything the client needs to create an AmneziaWG tunnel.
// It is embedded in the ServerConfig response when the server supports AmneziaWG.
type AWGClientConfig struct {
	// PublicKey is the server's X25519 public key (Base64, 44 chars).
	PublicKey string `json:"public_key"`
	// Endpoint is the UDP address to connect to, e.g. "95.216.1.1:51820".
	Endpoint string `json:"endpoint"`
	// AllowedIPs is the set of IP ranges to route through the tunnel.
	// The client should use "0.0.0.0/0, ::/0" for a full-tunnel VPN.
	AllowedIPs string `json:"allowed_ips"`
	// Obfuscation parameters — must be mirrored on client and server.
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

// RealityClientConfig holds REALITY settings for the client.
type RealityClientConfig struct {
	PublicKey   string `json:"public_key"`
	ShortID     string `json:"short_id"`
	ServerName  string `json:"server_name"`
	Fingerprint string `json:"fingerprint"`
}

// WebSocketClientConfig holds WebSocket CDN transport settings for the client.
// The client connects to Host:443 over WebSocket+TLS through Cloudflare CDN.
type WebSocketClientConfig struct {
	// Host is the Cloudflare-proxied CDN domain, e.g. "vpn.example.com".
	Host string `json:"host"`
	// Path is the WebSocket upgrade path, e.g. "/ws".
	Path string `json:"path"`
}

// ListServers handles GET /servers.
// Returns active VPN servers from the database, limited by subscription tier.
func ListServers(logger *zap.Logger, db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID := c.Locals("user_id").(string)
		tier := c.Locals("tier").(string)

		servers, err := repository.ListActiveServers(db)
		if err != nil {
			logger.Error("failed to list servers", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}

		// Apply tier-based limit: free users see fewer servers
		if limits, ok := model.PlanLimits[tier]; ok && limits.MaxServers > 0 {
			if len(servers) > limits.MaxServers {
				servers = servers[:limits.MaxServers]
			}
		}

		logger.Debug("listing servers",
			zap.String("user_id", userID),
			zap.String("tier", tier),
			zap.Int("count", len(servers)),
		)

		return c.JSON(fiber.Map{
			"data": servers,
		})
	}
}

// GetServerConfig handles GET /servers/:id/config.
// Returns the connection configuration for a specific server.
// Enforces the plan's MaxDevices limit: returns 429 if the user is already at capacity.
func GetServerConfig(logger *zap.Logger, db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		serverID := c.Params("id")
		userID := c.Locals("user_id").(string)
		tier := c.Locals("tier").(string)

		server, err := repository.FindServerByID(db, serverID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
					"error": "server not found",
				})
			}
			logger.Error("failed to find server", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}

		if !server.IsActive {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "server unavailable",
			})
		}

		// Enforce device limit before issuing a configuration.
		limits, ok := model.PlanLimits[tier]
		if !ok {
			limits = model.PlanLimits["free"]
		}

		activeCount, err := repository.CountActiveConnections(db, userID)
		if err != nil {
			logger.Error("failed to count active connections",
				zap.String("user_id", userID),
				zap.Error(err),
			)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "internal server error",
			})
		}

		if int(activeCount) >= limits.MaxDevices {
			logger.Warn("device limit reached at config fetch",
				zap.String("user_id", userID),
				zap.String("tier", tier),
				zap.Int64("active", activeCount),
				zap.Int("limit", limits.MaxDevices),
			)
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":        "device limit reached",
				"active_count": activeCount,
				"max_devices":  limits.MaxDevices,
			})
		}

		logger.Debug("serving server config",
			zap.String("server_id", serverID),
			zap.String("user_id", userID),
		)

		// Build config with real REALITY keys from the database
		publicKey := server.RealityPublicKey
		shortID := server.RealityShortID
		if publicKey == "" {
			publicKey = "OAmaJn5JqNlYdNIulgafHAwZs8MLLuU8MXs9rt26sl0"
		}
		if shortID == "" {
			shortID = "abcd1234"
		}

		config := ServerConfig{
			ServerAddress: server.IPAddress,
			ServerPort:    443,
			Protocol:      server.Protocol,
			UserID:        userID,
			Reality: &RealityClientConfig{
				PublicKey:   publicKey,
				ShortID:     shortID,
				ServerName:  "www.microsoft.com",
				Fingerprint: "chrome",
			},
		}

		// Include WebSocket CDN config when the server has it configured.
		// Clients use this as a fallback when direct REALITY connections are blocked.
		if server.WSEnabled && server.WSHost != "" {
			wsPath := server.WSPath
			if wsPath == "" {
				wsPath = "/ws"
			}
			config.WebSocket = &WebSocketClientConfig{
				Host: server.WSHost,
				Path: wsPath,
			}
		}

		// Include AmneziaWG config when the server has it provisioned.
		// The client can choose to connect via "amneziawg" instead of VLESS when
		// both protocols are available — AmneziaWG offers stronger anti-DPI properties.
		if server.AWGPublicKey != nil && *server.AWGPublicKey != "" &&
			server.AWGEndpoint != nil && *server.AWGEndpoint != "" {

			awgCfg := &AWGClientConfig{
				PublicKey:  *server.AWGPublicKey,
				Endpoint:   *server.AWGEndpoint,
				AllowedIPs: "0.0.0.0/0, ::/0",
			}

			// Copy obfuscation params when present; otherwise they default to zero
			// which means standard WireGuard behaviour (no obfuscation).
			if server.AWGParams != nil {
				awgCfg.Jc = server.AWGParams.Jc
				awgCfg.Jmin = server.AWGParams.Jmin
				awgCfg.Jmax = server.AWGParams.Jmax
				awgCfg.S1 = server.AWGParams.S1
				awgCfg.S2 = server.AWGParams.S2
				awgCfg.H1 = server.AWGParams.H1
				awgCfg.H2 = server.AWGParams.H2
				awgCfg.H3 = server.AWGParams.H3
				awgCfg.H4 = server.AWGParams.H4
			}

			config.AWG = awgCfg

			logger.Debug("including awg config in server response",
				zap.String("server_id", serverID),
				zap.String("awg_endpoint", *server.AWGEndpoint),
			)
		}

		return c.JSON(fiber.Map{
			"data": config,
		})
	}
}
