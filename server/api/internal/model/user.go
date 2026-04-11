package model

import "time"

// User represents a VPN user account.
// Email is stored as SHA-256 hash only — zero-knowledge policy.
// Guest users have no email or password (both fields are nil).
//
// Telegram fields (ADR-006) are an optional recovery binding. A
// user who pays for premium can link their Telegram account in the
// mobile app; the numeric Telegram user ID then lets them restore
// their subscription on any future device, across platform switches
// and factory resets. Free users have no reason to link and stay
// anonymous by default.
type User struct {
	ID                    string     `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	EmailHash             *string    `json:"-" gorm:"uniqueIndex"`
	PasswordHash          *string    `json:"-"`
	FullName              string     `json:"full_name" gorm:"type:varchar(255);default:''"`
	SubscriptionTier      string     `json:"subscription_tier" gorm:"default:free"`
	SubscriptionExpiresAt *time.Time `json:"subscription_expires_at"`
	Role                  string     `json:"role" gorm:"type:varchar(20);default:user"`
	TelegramUserID        *int64     `json:"telegram_user_id" gorm:"column:telegram_user_id;uniqueIndex"`
	TelegramLinkedAt      *time.Time `json:"telegram_linked_at" gorm:"column:telegram_linked_at"`
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
