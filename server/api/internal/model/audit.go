package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// AuditLogEntry records a single mutating admin action.
// See migration 014 for the table schema and rationale.
type AuditLogEntry struct {
	ID        string          `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	AdminID   string          `json:"admin_id" gorm:"type:uuid;not null;index"`
	Action    string          `json:"action" gorm:"type:varchar(64);not null"`
	TargetID  *string         `json:"target_id" gorm:"type:uuid;index"`
	Details   AuditDetails    `json:"details" gorm:"type:jsonb"`
	IP        string          `json:"ip" gorm:"type:varchar(45)"`
	CreatedAt time.Time       `json:"created_at" gorm:"autoCreateTime"`
}

// TableName pins the table to its snake_case form so GORM's default
// pluralisation ("audit_log_entries") does not leak into SQL.
func (AuditLogEntry) TableName() string { return "audit_log" }

// AuditDetails is a typed alias for map[string]interface{} with GORM
// Scanner/Valuer so the jsonb column round-trips cleanly. Carrying a
// concrete type instead of raw json.RawMessage lets handlers build
// payloads with plain map literals.
type AuditDetails map[string]interface{}

// Value implements driver.Valuer so GORM can store AuditDetails as jsonb.
func (d AuditDetails) Value() (driver.Value, error) {
	if d == nil {
		return nil, nil
	}
	b, err := json.Marshal(d)
	if err != nil {
		return nil, fmt.Errorf("marshaling AuditDetails: %w", err)
	}
	return string(b), nil
}

// Scan implements sql.Scanner so GORM can read AuditDetails from jsonb.
func (d *AuditDetails) Scan(value interface{}) error {
	if value == nil {
		*d = nil
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("AuditDetails.Scan: unsupported type %T", value)
	}
	return json.Unmarshal(bytes, d)
}
