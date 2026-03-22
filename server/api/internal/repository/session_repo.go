package repository

import (
	"errors"
	"time"

	"vpnapp/server/api/internal/model"

	"gorm.io/gorm"
)

// CreateSession stores a new refresh token session.
func CreateSession(db *gorm.DB, session *model.Session) error {
	return db.Create(session).Error
}

// FindSessionByTokenHash finds a valid (non-expired) session by refresh token hash.
func FindSessionByTokenHash(db *gorm.DB, tokenHash string) (*model.Session, error) {
	var session model.Session
	result := db.Where("refresh_token_hash = ? AND expires_at > ?", tokenHash, time.Now()).First(&session)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, result.Error
	}
	return &session, nil
}

// DeleteSession removes a session by ID (used during token refresh).
func DeleteSession(db *gorm.DB, id string) error {
	return db.Delete(&model.Session{}, "id = ?", id).Error
}

// DeleteExpiredSessions removes all sessions past their expiry time.
func DeleteExpiredSessions(db *gorm.DB) (int64, error) {
	result := db.Where("expires_at < ?", time.Now()).Delete(&model.Session{})
	return result.RowsAffected, result.Error
}
