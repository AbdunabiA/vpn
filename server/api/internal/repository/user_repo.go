package repository

import (
	"errors"

	"vpnapp/server/api/internal/model"

	"gorm.io/gorm"
)

// CreateUser inserts a new user into the database.
// Returns ErrDuplicate if the email_hash already exists.
func CreateUser(db *gorm.DB, user *model.User) error {
	result := db.Create(user)
	if result.Error != nil {
		if isDuplicateError(result.Error) {
			return ErrDuplicate
		}
		return result.Error
	}
	return nil
}

// FindUserByEmailHash looks up a user by their SHA-256 email hash.
func FindUserByEmailHash(db *gorm.DB, emailHash string) (*model.User, error) {
	var user model.User
	result := db.Where("email_hash = ?", emailHash).First(&user)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, result.Error
	}
	return &user, nil
}

// FindUserByID looks up a user by UUID.
func FindUserByID(db *gorm.DB, id string) (*model.User, error) {
	var user model.User
	result := db.First(&user, "id = ?", id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, result.Error
	}
	return &user, nil
}
