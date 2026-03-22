package repository

import (
	"errors"

	"vpnapp/server/api/internal/model"

	"gorm.io/gorm"
)

// ListActiveServers returns all active VPN servers ordered by load.
func ListActiveServers(db *gorm.DB) ([]model.VPNServer, error) {
	var servers []model.VPNServer
	result := db.Where("is_active = ?", true).Order("current_load ASC").Find(&servers)
	return servers, result.Error
}

// FindServerByID looks up a VPN server by UUID.
func FindServerByID(db *gorm.DB, id string) (*model.VPNServer, error) {
	var server model.VPNServer
	result := db.First(&server, "id = ?", id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, result.Error
	}
	return &server, nil
}
