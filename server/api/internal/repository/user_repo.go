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

// DeleteUser permanently removes a user record by UUID.
// Used to roll back user creation when a subsequent operation (e.g. creating
// the default subscription) fails and the registration must be treated as atomic.
func DeleteUser(db *gorm.DB, userID string) error {
	result := db.Delete(&model.User{}, "id = ?", userID)
	return result.Error
}

// UpdateUserTier sets the subscription_tier on the users row identified by id.
func UpdateUserTier(db *gorm.DB, userID, tier string) error {
	result := db.Model(&model.User{}).
		Where("id = ?", userID).
		Update("subscription_tier", tier)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteOrphanGuestUser removes a user row IF it is provably safe:
//   - it has no email_hash (was an anonymous guest)
//   - it is not an admin
//   - its subscription_tier is 'free' (no paid plan to lose)
//   - it has no active subscription rows in the subscriptions table
//   - it has no remaining devices bound to it
//
// Used by LinkDevice to clean up the previous owner of a device row that
// was just rebound to a plan owner via a share code. Refusing to delete
// paid guests prevents a leaked device_id from becoming a plan-deletion
// primitive (a guest who paid for premium then had their device row
// re-linked must NOT be silently deleted along with their Stripe data).
//
// Returns ErrNotFound when the user does not exist OR does not match the
// "safe orphan" pattern — callers must treat the not-found case as a soft
// signal that no cleanup happened, not as a real error.
//
// The cascading FKs on devices, sessions, and subscriptions take care of
// removing the dependent rows; we do not need to delete them by hand.
func DeleteOrphanGuestUser(db *gorm.DB, userID string) error {
	if db == nil {
		return errNilDB
	}
	// Filter on every safety condition we can express in SQL up front.
	var user model.User
	if err := db.Where(
		"id = ? AND email_hash IS NULL AND role <> 'admin' AND subscription_tier = 'free'",
		userID,
	).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	// Defence in depth: refuse if any subscription row claims to be active
	// (covers historical data where subscription_tier might be 'free' but
	// a Stripe subscription is still on file).
	var subCount int64
	if err := db.Model(&model.Subscription{}).
		Where("user_id = ? AND is_active = ?", userID, true).
		Count(&subCount).Error; err != nil {
		return err
	}
	if subCount > 0 {
		return ErrNotFound
	}
	var deviceCount int64
	if err := db.Model(&model.Device{}).Where("user_id = ?", userID).Count(&deviceCount).Error; err != nil {
		return err
	}
	if deviceCount > 0 {
		return ErrNotFound
	}
	if err := db.Delete(&model.User{}, "id = ?", userID).Error; err != nil {
		return err
	}
	return nil
}

// UpdateUserName sets the full_name on the users row identified by id.
func UpdateUserName(db *gorm.DB, userID, fullName string) error {
	result := db.Model(&model.User{}).
		Where("id = ?", userID).
		Update("full_name", fullName)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
