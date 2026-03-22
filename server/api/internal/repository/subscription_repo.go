package repository

import (
	"errors"

	"vpnapp/server/api/internal/model"

	"gorm.io/gorm"
)

// FindSubscriptionByUserID returns the active subscription for a user.
func FindSubscriptionByUserID(db *gorm.DB, userID string) (*model.Subscription, error) {
	var sub model.Subscription
	result := db.Where("user_id = ? AND is_active = ?", userID, true).
		Order("started_at DESC").
		First(&sub)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, result.Error
	}
	return &sub, nil
}

// CreateSubscription inserts a new subscription record.
func CreateSubscription(db *gorm.DB, sub *model.Subscription) error {
	return db.Create(sub).Error
}
