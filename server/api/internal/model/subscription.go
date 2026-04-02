package model

import "time"

// UnlimitedServers is a sentinel value for MaxServers meaning no server cap.
const UnlimitedServers = -1

// UnlimitedDevices is a sentinel value for MaxDevices meaning no device cap.
const UnlimitedDevices = -1

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

// PlanLimits maps subscription tier names to their resource limits.
// Use UnlimitedServers / UnlimitedDevices (-1) for plans without a cap.
var PlanLimits = map[string]struct {
	MaxDevices     int
	MaxServers     int // UnlimitedServers (-1) = no cap
	SpeedLimitMbps int // 0 = unlimited
}{
	"free":     {MaxDevices: 1, MaxServers: 3, SpeedLimitMbps: 50},
	"premium":  {MaxDevices: 5, MaxServers: UnlimitedServers, SpeedLimitMbps: 0},
	"ultimate": {MaxDevices: 10, MaxServers: UnlimitedServers, SpeedLimitMbps: 0},
}
