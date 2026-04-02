package tunnel

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
)

// socksAuth holds the generated SOCKS5 credentials for the current session.
// Prevents other apps on the device from piggybacking the local proxy.
var (
	socksAuthMu   sync.Mutex
	socksUser     string
	socksPassword string
)

// getSocksAuth returns (or generates) SOCKS5 credentials for the local proxy.
func getSocksAuth() (string, string) {
	socksAuthMu.Lock()
	defer socksAuthMu.Unlock()

	if socksUser == "" {
		b := make([]byte, 16)
		rand.Read(b)
		socksUser = hex.EncodeToString(b[:8])
		socksPassword = hex.EncodeToString(b[8:])
	}
	return socksUser, socksPassword
}

// resetSocksAuth clears credentials on disconnect so fresh ones are generated next connect.
func resetSocksAuth() {
	socksAuthMu.Lock()
	socksUser = ""
	socksPassword = ""
	socksAuthMu.Unlock()
}

// socksProxyURL returns the authenticated SOCKS5 proxy URL for tun2socks.
func socksProxyURL() string {
	user, pass := getSocksAuth()
	return fmt.Sprintf("socks5://%s:%s@127.0.0.1:%d", user, pass, localSocksPort)
}

// buildClientXRayConfig creates the XRay-core client configuration for VLESS+REALITY.
//
// This config tells XRay-core to:
// 1. Open a local SOCKS5 proxy with authentication (prevents local app piggybacking)
// 2. Connect to the remote server using VLESS protocol
// 3. Use REALITY for the TLS handshake (impersonating a real website)
// 4. Apply xtls-rprx-vision flow for optimal performance
// 5. Sniff DNS/TLS/HTTP to prevent DNS leaks
//
// The resulting JSON is passed to core.LoadConfig() to create an XRay instance.
func buildClientXRayConfig(config ConnectConfig) map[string]interface{} {
	fingerprint := "chrome"
	if config.Reality != nil && config.Reality.Fingerprint != "" {
		fingerprint = config.Reality.Fingerprint
	}

	serverName := "www.microsoft.com"
	if config.Reality != nil && config.Reality.ServerName != "" {
		serverName = config.Reality.ServerName
	}

	publicKey := ""
	shortID := ""
	if config.Reality != nil {
		publicKey = config.Reality.PublicKey
		shortID = config.Reality.ShortID
	}

	user, pass := getSocksAuth()

	return map[string]interface{}{
		"log": map[string]interface{}{
			"loglevel": "warning",
		},
		// DNS configuration — resolves through the tunnel to prevent DNS leaks
		"dns": map[string]interface{}{
			"servers": []string{"1.1.1.1", "8.8.8.8"},
		},
		"inbounds": []map[string]interface{}{
			{
				// Local SOCKS5 proxy with authentication
				"listen":   "127.0.0.1",
				"port":     localSocksPort,
				"protocol": "socks",
				"settings": map[string]interface{}{
					"auth": "password",
					"accounts": []map[string]interface{}{
						{
							"user": user,
							"pass": pass,
						},
					},
					"udp": true,
				},
				// Sniff traffic to detect DNS queries and route them through the tunnel
				"sniffing": map[string]interface{}{
					"enabled":      true,
					"destOverride": []string{"http", "tls", "quic"},
				},
				"tag": "socks-in",
			},
		},
		"outbounds": []map[string]interface{}{
			{
				"protocol": "vless",
				"settings": map[string]interface{}{
					"vnext": []map[string]interface{}{
						{
							"address": config.ServerAddress,
							"port":    config.ServerPort,
							"users": []map[string]interface{}{
								{
									"id":         config.UserID,
									"flow":       "xtls-rprx-vision",
									"encryption": "none",
								},
							},
						},
					},
				},
				"streamSettings": map[string]interface{}{
					"network":  "tcp",
					"security": "reality",
					"realitySettings": map[string]interface{}{
						"serverName":  serverName,
						"fingerprint": fingerprint,
						"publicKey":   publicKey,
						"shortId":     shortID,
					},
				},
				"tag": "vless-out",
			},
			{
				"protocol": "freedom",
				"tag":      "direct",
			},
		},
		"routing": map[string]interface{}{
			"rules": buildRoutingRules(config),
		},
	}
}

// buildWebSocketXRayConfig creates the XRay-core client configuration for VLESS over WebSocket CDN.
//
// This config routes traffic through Cloudflare CDN instead of connecting directly to
// the VPN server. Key differences from the REALITY config:
//   - Uses standard TLS (Cloudflare terminates TLS at the edge)
//   - Uses WebSocket transport ("ws") so it looks like normal web traffic
//   - flow must be empty — xtls-rprx-vision is incompatible with WebSocket transport
//   - Connects to the CDN domain on port 443, not to the server IP directly
//
// The resulting JSON is passed to core.LoadConfig() to create an XRay instance.
func buildWebSocketXRayConfig(config ConnectConfig) map[string]interface{} {
	wsHost := ""
	wsPath := "/ws"
	if config.WebSocket != nil {
		if config.WebSocket.Host != "" {
			wsHost = config.WebSocket.Host
		}
		if config.WebSocket.Path != "" {
			wsPath = config.WebSocket.Path
		}
	}

	// Fall back to ServerAddress when no explicit CDN host is configured.
	// This allows the config to function even if WebSocket is partially specified.
	if wsHost == "" {
		wsHost = config.ServerAddress
	}

	user, pass := getSocksAuth()

	return map[string]interface{}{
		"log": map[string]interface{}{
			"loglevel": "warning",
		},
		// DNS configuration — resolves through the tunnel to prevent DNS leaks
		"dns": map[string]interface{}{
			"servers": []string{"1.1.1.1", "8.8.8.8"},
		},
		"inbounds": []map[string]interface{}{
			{
				// Local SOCKS5 proxy with authentication
				"listen":   "127.0.0.1",
				"port":     localSocksPort,
				"protocol": "socks",
				"settings": map[string]interface{}{
					"auth": "password",
					"accounts": []map[string]interface{}{
						{
							"user": user,
							"pass": pass,
						},
					},
					"udp": true,
				},
				// Sniff traffic to detect DNS queries and route them through the tunnel
				"sniffing": map[string]interface{}{
					"enabled":      true,
					"destOverride": []string{"http", "tls", "quic"},
				},
				"tag": "socks-in",
			},
		},
		"outbounds": []map[string]interface{}{
			{
				"protocol": "vless",
				"settings": map[string]interface{}{
					"vnext": []map[string]interface{}{
						{
							// Connect to the CDN domain — Cloudflare proxies to origin.
							// Port 443 because Cloudflare only proxies WebSocket on HTTPS ports.
							"address": wsHost,
							"port":    443,
							"users": []map[string]interface{}{
								{
									"id": config.UserID,
									// flow MUST be empty for WebSocket transport.
									// xtls-rprx-vision is TCP-only and will crash with "ws".
									"flow":       "",
									"encryption": "none",
								},
							},
						},
					},
				},
				"streamSettings": map[string]interface{}{
					"network":  "ws",
					"security": "tls",
					// Standard TLS — Cloudflare terminates it. No realitySettings needed.
					"tlsSettings": map[string]interface{}{
						"serverName": wsHost,
					},
					"wsSettings": map[string]interface{}{
						"path": wsPath,
						"headers": map[string]interface{}{
							"Host": wsHost,
						},
					},
				},
				"tag": "vless-out",
			},
			{
				"protocol": "freedom",
				"tag":      "direct",
			},
		},
		"routing": map[string]interface{}{
			"rules": buildRoutingRules(config),
		},
	}
}

// buildRoutingRules returns the XRay routing rules for the given config.
// If ExcludedDomains is non-empty a direct-bypass rule is prepended before the
// catch-all vless-out rule so that those domains never enter the VPN tunnel.
func buildRoutingRules(config ConnectConfig) []map[string]interface{} {
	rules := []map[string]interface{}{}

	// Per-domain split tunnel bypass — must come BEFORE the catch-all rule.
	if len(config.ExcludedDomains) > 0 {
		rules = append(rules, map[string]interface{}{
			"type":        "field",
			"domain":      config.ExcludedDomains,
			"outboundTag": "direct",
		})
	}

	// Catch-all: route every inbound packet through the VPN.
	rules = append(rules, map[string]interface{}{
		"type":        "field",
		"inboundTag":  []string{"socks-in"},
		"outboundTag": "vless-out",
	})

	return rules
}
