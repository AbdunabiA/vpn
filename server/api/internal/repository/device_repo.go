package repository

import (
	"errors"
	"time"

	"vpnapp/server/api/internal/model"

	"gorm.io/gorm"
)

// FindDeviceByDeviceID looks up a device row by its OS-issued device_id.
// Returns ErrNotFound when the device has never authenticated before.
func FindDeviceByDeviceID(db *gorm.DB, deviceID string) (*model.Device, error) {
	var device model.Device
	result := db.Where("device_id = ?", deviceID).First(&device)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, result.Error
	}
	return &device, nil
}

// CreateDevice inserts a new device row.
// Returns ErrDuplicate if the device_id already exists (caller should
// FindDeviceByDeviceID first or handle the error).
func CreateDevice(db *gorm.DB, device *model.Device) error {
	result := db.Create(device)
	if result.Error != nil {
		if isDuplicateError(result.Error) {
			return ErrDuplicate
		}
		return result.Error
	}
	return nil
}

// TouchDevice updates last_seen_at to NOW() for the given device row.
// Idempotent — safe to call on every guest login.
func TouchDevice(db *gorm.DB, id string) error {
	result := db.Model(&model.Device{}).
		Where("id = ?", id).
		Update("last_seen_at", time.Now())
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ReassignDeviceUser updates a device row to belong to a different user_id.
// Used by the share-code link flow: when a friend redeems a code, their
// existing device row is rebound to the plan owner so that future
// connections count against the owner's quota.
func ReassignDeviceUser(db *gorm.DB, deviceID, newUserID string) error {
	result := db.Model(&model.Device{}).
		Where("device_id = ?", deviceID).
		Updates(map[string]interface{}{
			"user_id":      newUserID,
			"last_seen_at": time.Now(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// CountDevicesByUser returns the number of devices currently bound to a user.
// Used by the share-code endpoint to refuse generating a code when the owner
// is already at their device cap (no point sharing if there is no slot left).
func CountDevicesByUser(db *gorm.DB, userID string) (int64, error) {
	var count int64
	result := db.Model(&model.Device{}).Where("user_id = ?", userID).Count(&count)
	return count, result.Error
}

// ListDevicesByUser returns all devices bound to a user, newest first.
// Exposed via /admin/users/:id/devices and the in-app "My devices" screen.
func ListDevicesByUser(db *gorm.DB, userID string) ([]model.Device, error) {
	var devices []model.Device
	result := db.Where("user_id = ?", userID).
		Order("last_seen_at DESC").
		Find(&devices)
	return devices, result.Error
}
