package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds API server configuration loaded from environment variables.
type Config struct {
	Port                int
	DatabaseURL         string
	RedisURL            string
	JWTSecret           string
	StripeKey           string
	StripeWebhookSecret string
	StripePricePremium  string
	StripePriceUltimate string
	AppDeepLinkScheme   string
	TunnelVLESSUUID     string
	MinAppVersion       string

	// Background scheduler / sharing tunables. Defaults match the
	// previously hard-coded values; expose them so deployments can adjust
	// without recompiling.
	StaleConnectionAfter time.Duration // marks connections without heartbeat as stale
	StaleDeviceAfter     time.Duration // auto-removes idle device rows
	LinkCodeTTL          time.Duration // share-code lifetime before expiry
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	port, err := strconv.Atoi(getEnv("PORT", "3000"))
	if err != nil {
		return nil, fmt.Errorf("invalid PORT: %w", err)
	}

	cfg := &Config{
		Port:                 port,
		DatabaseURL:          getEnv("DATABASE_URL", "postgres://localhost:5432/vpnapp?sslmode=disable"),
		RedisURL:             getEnv("REDIS_URL", "redis://localhost:6379"),
		JWTSecret:            getEnv("JWT_SECRET", ""),
		StripeKey:            getEnv("STRIPE_KEY", ""),
		StripeWebhookSecret:  getEnv("STRIPE_WEBHOOK_SECRET", ""),
		StripePricePremium:   getEnv("STRIPE_PRICE_PREMIUM", "price_PLACEHOLDER_PREMIUM"),
		StripePriceUltimate:  getEnv("STRIPE_PRICE_ULTIMATE", "price_PLACEHOLDER_ULTIMATE"),
		AppDeepLinkScheme:    getEnv("APP_DEEP_LINK", "vpnapp"),
		TunnelVLESSUUID:      getEnv("TUNNEL_VLESS_UUID", ""),
		MinAppVersion:        getEnv("MIN_APP_VERSION", "2.0.0"),
		StaleConnectionAfter: getEnvDuration("STALE_CONNECTION_AFTER", 3*time.Minute),
		StaleDeviceAfter:     getEnvDuration("STALE_DEVICE_AFTER", 30*24*time.Hour),
		LinkCodeTTL:          getEnvDuration("LINK_CODE_TTL", 5*time.Minute),
	}

	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}

	if cfg.TunnelVLESSUUID == "" {
		return nil, fmt.Errorf("TUNNEL_VLESS_UUID is required")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

// getEnvDuration parses a Go duration string from the environment, falling
// back to the default if the var is unset or unparseable.
func getEnvDuration(key string, fallback time.Duration) time.Duration {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		return fallback
	}
	return d
}
