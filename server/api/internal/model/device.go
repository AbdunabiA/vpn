package model

import "time"

// Device represents a unique physical device that has authenticated against
// the API. It is keyed by the OS-issued stable device identifier (ANDROID_ID
// on Android, identifierForVendor on iOS). A device is owned by exactly one
// user at a time; redeeming a link code reassigns the device row to the plan
// owner so that subsequent connections count against the owner's quota.
//
// DeviceSecretHash is the SHA-256 of a 32-byte client-generated secret
// stored in the app's private storage. It pairs with device_id so that
// knowing the device_id alone is not enough to impersonate the user — see
// migration 012 for the threat model. The field is null-able for backwards
// compatibility with rows created before the secret was introduced; the
// first authenticated touch from the legitimate device populates it.
type Device struct {
	ID               string    `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	UserID           string    `json:"user_id" gorm:"not null;index"`
	DeviceID         string    `json:"device_id" gorm:"uniqueIndex;not null"`
	DeviceSecretHash string    `json:"-" gorm:"column:device_secret_hash"`
	Platform         string    `json:"platform" gorm:"type:varchar(20);default:''"`
	Model            string    `json:"model" gorm:"type:varchar(255);default:''"`
	FirstSeenAt      time.Time `json:"first_seen_at" gorm:"autoCreateTime"`
	LastSeenAt       time.Time `json:"last_seen_at" gorm:"autoUpdateTime"`
}
