-- Initial database schema for the VPN API server
-- Run against PostgreSQL 16+

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Users table: stores account info with zero-knowledge email (SHA-256 hash only)
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email_hash VARCHAR(64) NOT NULL UNIQUE, -- SHA-256 hex digest of email
    password_hash VARCHAR(255) NOT NULL,     -- bcrypt hash
    subscription_tier VARCHAR(20) NOT NULL DEFAULT 'free',
    subscription_expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Sessions table: tracks active refresh tokens per device
CREATE TABLE sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    refresh_token_hash VARCHAR(64) NOT NULL, -- SHA-256 of refresh token
    device_info VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_sessions_user_id ON sessions(user_id);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);

-- VPN servers table: tracks available tunnel servers
CREATE TABLE vpn_servers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hostname VARCHAR(100) NOT NULL UNIQUE,
    ip_address INET NOT NULL,
    region VARCHAR(50) NOT NULL,
    city VARCHAR(100) NOT NULL,
    country VARCHAR(100) NOT NULL,
    country_code CHAR(2) NOT NULL,
    protocol VARCHAR(50) NOT NULL DEFAULT 'vless-reality',
    capacity INTEGER NOT NULL DEFAULT 500,
    current_load INTEGER NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_vpn_servers_region ON vpn_servers(region);
CREATE INDEX idx_vpn_servers_active ON vpn_servers(is_active);

-- Connections table: tracks active VPN connections (for device limiting)
CREATE TABLE connections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    server_id UUID NOT NULL REFERENCES vpn_servers(id) ON DELETE CASCADE,
    connected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    disconnected_at TIMESTAMPTZ,
    bytes_up BIGINT NOT NULL DEFAULT 0,
    bytes_down BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX idx_connections_user_id ON connections(user_id);
CREATE INDEX idx_connections_server_id ON connections(server_id);
CREATE INDEX idx_connections_active ON connections(disconnected_at) WHERE disconnected_at IS NULL;

-- Subscriptions table: payment and plan tracking
CREATE TABLE subscriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    plan VARCHAR(20) NOT NULL DEFAULT 'free',
    stripe_id VARCHAR(255),
    is_active BOOLEAN NOT NULL DEFAULT true,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ
);

CREATE INDEX idx_subscriptions_user_id ON subscriptions(user_id);

-- Seed some initial VPN servers
INSERT INTO vpn_servers (hostname, ip_address, region, city, country, country_code, protocol, capacity) VALUES
    ('fi-hel-01', '95.216.1.1', 'Europe', 'Helsinki', 'Finland', 'FI', 'vless-reality', 500),
    ('nl-ams-01', '178.63.2.2', 'Europe', 'Amsterdam', 'Netherlands', 'NL', 'vless-reality', 500),
    ('de-fra-01', '116.202.3.3', 'Europe', 'Frankfurt', 'Germany', 'DE', 'vless-reality', 500),
    ('kz-ala-01', '103.150.4.4', 'Asia', 'Almaty', 'Kazakhstan', 'KZ', 'vless-reality', 500),
    ('tr-ist-01', '185.193.5.5', 'Europe', 'Istanbul', 'Turkey', 'TR', 'vless-reality', 500);
