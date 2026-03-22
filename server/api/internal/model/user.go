package model

import "time"

// User represents a VPN user account.
// Email is stored as SHA-256 hash only — zero-knowledge policy.
type User struct {
	ID                    string     `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	EmailHash             string     `json:"-" gorm:"uniqueIndex;not null"`
	PasswordHash          string     `json:"-" gorm:"not null"`
	SubscriptionTier      string     `json:"subscription_tier" gorm:"default:free"`
	SubscriptionExpiresAt *time.Time `json:"subscription_expires_at"`
	CreatedAt             time.Time  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt             time.Time  `json:"-" gorm:"autoUpdateTime"`
}

// Session represents an active user session (refresh token).
type Session struct {
	ID               string    `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	UserID           string    `gorm:"not null;index"`
	RefreshTokenHash string    `gorm:"not null"`
	DeviceInfo       string    `gorm:"type:varchar(255)"`
	CreatedAt        time.Time `gorm:"autoCreateTime"`
	ExpiresAt        time.Time `gorm:"not null"`
}
