package repository

import (
	"errors"
	"fmt"

	"vpnapp/server/api/internal/model"

	"gorm.io/gorm"
)

var errNilDB = fmt.Errorf("database connection is nil")

// ListUsers returns a paginated slice of users and the total matching count.
// search is matched case-insensitively against the email_hash column when non-empty.
// page and limit must both be >= 1; the caller is responsible for validation.
func ListUsers(db *gorm.DB, page, limit int, search string) ([]model.User, int64, error) {
	if db == nil {
		return nil, 0, errNilDB
	}
	query := db.Model(&model.User{})

	if search != "" {
		query = query.Where("email_hash ILIKE ?", fmt.Sprintf("%%%s%%", search))
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("counting users: %w", err)
	}

	offset := (page - 1) * limit
	var users []model.User
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&users).Error; err != nil {
		return nil, 0, fmt.Errorf("listing users: %w", err)
	}

	return users, total, nil
}

// UpdateUser applies an arbitrary set of column updates to a single user row.
// Only columns present in updates are modified; this prevents accidental zero-value overwrites.
// Returns ErrNotFound when no row matches userID.
func UpdateUser(db *gorm.DB, userID string, updates map[string]interface{}) error {
	if db == nil {
		return errNilDB
	}
	result := db.Model(&model.User{}).Where("id = ?", userID).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("updating user %s: %w", userID, result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateServer inserts a new VPN server record.
// Returns ErrDuplicate when the hostname already exists.
func CreateServer(db *gorm.DB, server *model.VPNServer) error {
	if db == nil {
		return errNilDB
	}
	result := db.Create(server)
	if result.Error != nil {
		if isDuplicateError(result.Error) {
			return ErrDuplicate
		}
		return fmt.Errorf("creating server: %w", result.Error)
	}
	return nil
}

// UpdateServer applies an arbitrary set of column updates to a single VPN server row.
// Returns ErrNotFound when no row matches serverID.
func UpdateServer(db *gorm.DB, serverID string, updates map[string]interface{}) error {
	if db == nil {
		return errNilDB
	}
	result := db.Model(&model.VPNServer{}).Where("id = ?", serverID).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("updating server %s: %w", serverID, result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteServer performs a soft delete by setting is_active = false.
// Returns ErrNotFound when no row matches serverID.
func DeleteServer(db *gorm.DB, serverID string) error {
	if db == nil {
		return errNilDB
	}
	result := db.Model(&model.VPNServer{}).Where("id = ?", serverID).Update("is_active", false)
	if result.Error != nil {
		return fmt.Errorf("soft-deleting server %s: %w", serverID, result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ListAllServers returns every VPN server row, including inactive ones, ordered by hostname.
// This is the admin view; the public ListActiveServers only returns active servers.
func ListAllServers(db *gorm.DB) ([]model.VPNServer, error) {
	if db == nil {
		return nil, errNilDB
	}
	var servers []model.VPNServer
	if err := db.Order("hostname ASC").Find(&servers).Error; err != nil {
		return nil, fmt.Errorf("listing all servers: %w", err)
	}
	return servers, nil
}

// GetGlobalStats returns dashboard-level aggregate counts.
// Keys in the returned map: total_users, active_subscriptions, server_count, active_server_count.
func GetGlobalStats(db *gorm.DB) (map[string]interface{}, error) {
	if db == nil {
		return nil, errNilDB
	}

	var totalUsers int64
	if err := db.Model(&model.User{}).Count(&totalUsers).Error; err != nil {
		return nil, fmt.Errorf("counting users: %w", err)
	}

	var activeSubscriptions int64
	if err := db.Model(&model.Subscription{}).
		Where("is_active = ?", true).
		Count(&activeSubscriptions).Error; err != nil {
		return nil, fmt.Errorf("counting active subscriptions: %w", err)
	}

	var serverCount int64
	if err := db.Model(&model.VPNServer{}).Count(&serverCount).Error; err != nil {
		return nil, fmt.Errorf("counting servers: %w", err)
	}

	var activeServerCount int64
	if err := db.Model(&model.VPNServer{}).
		Where("is_active = ?", true).
		Count(&activeServerCount).Error; err != nil {
		return nil, fmt.Errorf("counting active servers: %w", err)
	}

	return map[string]interface{}{
		"total_users":          totalUsers,
		"active_subscriptions": activeSubscriptions,
		"server_count":         serverCount,
		"active_server_count":  activeServerCount,
	}, nil
}

// FindUserByIDAdmin looks up any user by UUID for admin use.
// Wraps the sentinel error so callers can use errors.Is(err, ErrNotFound).
func FindUserByIDAdmin(db *gorm.DB, id string) (*model.User, error) {
	if db == nil {
		return nil, errNilDB
	}
	var user model.User
	result := db.First(&user, "id = ?", id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("finding user %s: %w", id, result.Error)
	}
	return &user, nil
}
