package model

import "time"

// Subscription tracks a user's payment/plan status.
type Subscription struct {
	ID        string     `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	UserID    string     `json:"user_id" gorm:"not null;index"`
	Plan      string     `json:"plan" gorm:"not null;default:free"` // free, premium, ultimate
	StripeID  string     `json:"-" gorm:"type:varchar(255)"`
	IsActive  bool       `json:"is_active" gorm:"default:true"`
	StartedAt time.Time  `json:"started_at" gorm:"autoCreateTime"`
	ExpiresAt *time.Time `json:"expires_at"`
}

// Plan limits for each subscription tier.
var PlanLimits = map[string]struct {
	MaxDevices    int
	MaxServers    int // -1 = unlimited
	SpeedLimitMbps int // 0 = unlimited
}{
	"free":     {MaxDevices: 1, MaxServers: 3, SpeedLimitMbps: 50},
	"premium":  {MaxDevices: 5, MaxServers: -1, SpeedLimitMbps: 0},
	"ultimate": {MaxDevices: 10, MaxServers: -1, SpeedLimitMbps: 0},
}
