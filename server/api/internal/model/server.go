package model

import "time"

// VPNServer represents a VPN tunnel server in the database.
type VPNServer struct {
	ID              string    `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	Hostname        string    `json:"hostname" gorm:"uniqueIndex;not null"`
	IPAddress       string    `json:"ip_address" gorm:"not null"`
	Region          string    `json:"region" gorm:"not null;index"`
	City            string    `json:"city" gorm:"not null"`
	Country         string    `json:"country" gorm:"not null"`
	CountryCode     string    `json:"country_code" gorm:"type:char(2);not null"`
	Protocol        string    `json:"protocol" gorm:"not null;default:vless-reality"`
	Capacity        int       `json:"-" gorm:"not null;default:500"`
	CurrentLoad     int       `json:"load_percent" gorm:"default:0"`
	IsActive        bool      `json:"is_active" gorm:"default:true;index"`
	RealityPublicKey string   `json:"-" gorm:"column:reality_public_key"`
	RealityShortID   string   `json:"-" gorm:"column:reality_short_id"`
	CreatedAt       time.Time `json:"created_at" gorm:"autoCreateTime"`
}

// Connection tracks an active VPN connection (for concurrent device limiting).
type Connection struct {
	ID             string     `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	UserID         string     `gorm:"not null;index"`
	ServerID       string     `gorm:"not null;index"`
	ConnectedAt    time.Time  `gorm:"autoCreateTime"`
	DisconnectedAt *time.Time `gorm:"index"`
	BytesUp        int64      `gorm:"default:0"`
	BytesDown      int64      `gorm:"default:0"`
}
