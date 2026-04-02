package repository

import (
	"errors"
	"time"

	"vpnapp/server/api/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CreateConnection inserts a new active connection record.
// The ID is generated in Go so the function works with any database backend.
func CreateConnection(db *gorm.DB, conn *model.Connection) error {
	if conn.ID == "" {
		conn.ID = uuid.NewString()
	}
	if conn.ConnectedAt.IsZero() {
		conn.ConnectedAt = time.Now()
	}
	result := db.Create(conn)
	return result.Error
}

// CreateConnectionAtomic inserts a connection record only when the caller's
// current active-connection count is below maxDevices.  The check and the
// insert are performed in a single statement so there is no TOCTOU window
// between counting and inserting.
//
// Returns (true, nil) when the row was inserted successfully.
// Returns (false, nil) when the device limit has already been reached (the
// caller should return HTTP 429).
// Returns (false, err) on any other database error.
func CreateConnectionAtomic(db *gorm.DB, conn *model.Connection, maxDevices int) (bool, error) {
	if conn.ID == "" {
		conn.ID = uuid.NewString()
	}
	if conn.ConnectedAt.IsZero() {
		conn.ConnectedAt = time.Now()
	}

	// The INSERT … SELECT pattern makes the limit check and the insert atomic.
	// No row is written when the sub-query count equals or exceeds maxDevices.
	result := db.Exec(
		`INSERT INTO connections (id, user_id, server_id, connected_at, bytes_up, bytes_down)
		 SELECT ?, ?, ?, ?, 0, 0
		 WHERE (
		   SELECT COUNT(*) FROM connections
		   WHERE user_id = ? AND disconnected_at IS NULL
		 ) < ?`,
		conn.ID, conn.UserID, conn.ServerID, conn.ConnectedAt,
		conn.UserID, maxDevices,
	)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 0 {
		// The sub-query returned count >= maxDevices; no row was inserted.
		return false, nil
	}
	return true, nil
}

// DisconnectConnection marks a connection as disconnected and records final byte counts.
// Returns ErrNotFound if the connection does not exist.
func DisconnectConnection(db *gorm.DB, id string, bytesUp, bytesDown int64) error {
	now := time.Now()
	result := db.Model(&model.Connection{}).
		Where("id = ? AND disconnected_at IS NULL", id).
		Updates(map[string]interface{}{
			"disconnected_at": now,
			"bytes_up":        bytesUp,
			"bytes_down":      bytesDown,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// CountActiveConnections returns the number of connections for a user that have no
// disconnected_at timestamp — i.e. connections that are still live.
func CountActiveConnections(db *gorm.DB, userID string) (int64, error) {
	var count int64
	result := db.Model(&model.Connection{}).
		Where("user_id = ? AND disconnected_at IS NULL", userID).
		Count(&count)
	return count, result.Error
}

// ListActiveConnectionsByUser returns all live connections for a given user.
func ListActiveConnectionsByUser(db *gorm.DB, userID string) ([]model.Connection, error) {
	var connections []model.Connection
	result := db.Where("user_id = ? AND disconnected_at IS NULL", userID).
		Order("connected_at DESC").
		Find(&connections)
	return connections, result.Error
}

// CleanupStaleConnections marks connections as disconnected when their last heartbeat
// (connected_at) is older than staleDuration and they still have no disconnected_at.
// Returns the number of rows affected.
func CleanupStaleConnections(db *gorm.DB, staleDuration time.Duration) (int64, error) {
	cutoff := time.Now().Add(-staleDuration)
	now := time.Now()

	result := db.Model(&model.Connection{}).
		Where("disconnected_at IS NULL AND connected_at < ?", cutoff).
		Updates(map[string]interface{}{
			"disconnected_at": now,
		})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// FindConnectionByID looks up a connection by UUID.
// Returns ErrNotFound if no row exists.
func FindConnectionByID(db *gorm.DB, id string) (*model.Connection, error) {
	var conn model.Connection
	result := db.First(&conn, "id = ?", id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, result.Error
	}
	return &conn, nil
}
