package repository

import (
	"errors"
	"fmt"
	"time"

	"vpnapp/server/api/internal/model"

	"gorm.io/gorm"
)

var errNilDB = fmt.Errorf("database connection is nil")

// ListUsers returns a paginated slice of users and the total matching count.
// search is matched case-insensitively against user id, email_hash, or full_name.
// Useful for the admin panel's user search — a user_id prefix pasted by a user
// contacting support will resolve to the correct account.
// page and limit must both be >= 1; the caller is responsible for validation.
func ListUsers(db *gorm.DB, page, limit int, search string) ([]model.User, int64, error) {
	if db == nil {
		return nil, 0, errNilDB
	}
	query := db.Model(&model.User{})

	if search != "" {
		like := fmt.Sprintf("%%%s%%", search)
		query = query.Where(
			"CAST(id AS TEXT) ILIKE ? OR email_hash ILIKE ? OR full_name ILIKE ?",
			like, like, like,
		)
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

// TimeseriesBucket is a single day in a dashboard timeseries. The date
// is formatted as YYYY-MM-DD in UTC so the frontend can plot without
// further parsing, and the count is whatever the caller asked for
// (signups, connections, etc.).
type TimeseriesBucket struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

// GetTimeseries returns per-day signup and connection counts for the
// last `days` calendar days (UTC), padded with zero-count entries so
// the frontend always receives a contiguous series. The fixed window
// keeps query time bounded and the resulting JSON small.
func GetTimeseries(db *gorm.DB, days int) (signups, connections []TimeseriesBucket, err error) {
	if db == nil {
		return nil, nil, errNilDB
	}
	if days <= 0 || days > 180 {
		days = 30
	}

	// Build the zero-filled skeleton up front. We key by YYYY-MM-DD so
	// that Postgres's date_trunc results slot straight in.
	now := time.Now().UTC()
	startDay := now.AddDate(0, 0, -(days - 1))
	signupMap := make(map[string]int64, days)
	connectMap := make(map[string]int64, days)
	orderedDays := make([]string, 0, days)
	for i := 0; i < days; i++ {
		day := startDay.AddDate(0, 0, i).Format("2006-01-02")
		signupMap[day] = 0
		connectMap[day] = 0
		orderedDays = append(orderedDays, day)
	}

	// Signups — users.created_at grouped by day.
	type row struct {
		Day   string
		Count int64
	}
	var signupRows []row
	if err := db.Model(&model.User{}).
		Select("TO_CHAR(DATE_TRUNC('day', created_at AT TIME ZONE 'UTC'), 'YYYY-MM-DD') AS day, COUNT(*) AS count").
		Where("created_at >= ?", startDay).
		Group("day").
		Scan(&signupRows).Error; err != nil {
		return nil, nil, fmt.Errorf("querying signups timeseries: %w", err)
	}
	for _, r := range signupRows {
		if _, ok := signupMap[r.Day]; ok {
			signupMap[r.Day] = r.Count
		}
	}

	// Connections — count every connection row that started within the
	// window, regardless of whether it's still active. This matches the
	// "new connections per day" intuition the dashboard card will show.
	var connectRows []row
	if err := db.Model(&model.Connection{}).
		Select("TO_CHAR(DATE_TRUNC('day', connected_at AT TIME ZONE 'UTC'), 'YYYY-MM-DD') AS day, COUNT(*) AS count").
		Where("connected_at >= ?", startDay).
		Group("day").
		Scan(&connectRows).Error; err != nil {
		return nil, nil, fmt.Errorf("querying connections timeseries: %w", err)
	}
	for _, r := range connectRows {
		if _, ok := connectMap[r.Day]; ok {
			connectMap[r.Day] = r.Count
		}
	}

	signups = make([]TimeseriesBucket, 0, days)
	connections = make([]TimeseriesBucket, 0, days)
	for _, day := range orderedDays {
		signups = append(signups, TimeseriesBucket{Date: day, Count: signupMap[day]})
		connections = append(connections, TimeseriesBucket{Date: day, Count: connectMap[day]})
	}
	return signups, connections, nil
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
