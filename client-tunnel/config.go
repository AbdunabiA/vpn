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
			"rules": []map[string]interface{}{
				{
					// Route all traffic through the VPN
					"type":        "field",
					"inboundTag":  []string{"socks-in"},
					"outboundTag": "vless-out",
				},
			},
		},
	}
}
