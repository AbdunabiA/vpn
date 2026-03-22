package tunnel

// buildClientXRayConfig creates the XRay-core client configuration for VLESS+REALITY.
//
// This config tells XRay-core to:
// 1. Open a local SOCKS5 proxy (for the OS to route traffic through)
// 2. Connect to the remote server using VLESS protocol
// 3. Use REALITY for the TLS handshake (impersonating a real website)
// 4. Apply xtls-rprx-vision flow for optimal performance
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

	return map[string]interface{}{
		"log": map[string]interface{}{
			"loglevel": "warning",
		},
		"inbounds": []map[string]interface{}{
			{
				// Local SOCKS5 proxy — the OS VPN service routes traffic here
				"port":     10808,
				"protocol": "socks",
				"settings": map[string]interface{}{
					"udp": true,
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
