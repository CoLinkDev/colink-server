package repository

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"colink-server/internal/model"
)

type DeviceRepository struct {
	db *gorm.DB
}

func NewDeviceRepository(db *gorm.DB) *DeviceRepository {
	return &DeviceRepository{db: db}
}

func (r *DeviceRepository) CountByUserID(userID uuid.UUID) (int64, error) {
	var count int64
	if err := r.db.Model(&model.Device{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		return 0, err
	}

	return count, nil
}

func (r *DeviceRepository) ExistsByID(deviceID uuid.UUID) (bool, error) {
	var count int64
	if err := r.db.Model(&model.Device{}).Where("id = ?", deviceID).Count(&count).Error; err != nil {
		return false, err
	}

	return count > 0, nil
}

func (r *DeviceRepository) Create(device *model.Device) error {
	return r.db.Create(device).Error
}

func (r *DeviceRepository) ListByUserID(userID uuid.UUID) ([]model.Device, error) {
	var devices []model.Device
	if err := r.db.Where("user_id = ?", userID).Order("created_at asc").Find(&devices).Error; err != nil {
		return nil, err
	}

	return devices, nil
}

func (r *DeviceRepository) FindByIDAndUserID(deviceID uuid.UUID, userID uuid.UUID) (*model.Device, error) {
	var device model.Device
	if err := r.db.Where("id = ? AND user_id = ?", deviceID, userID).First(&device).Error; err != nil {
		return nil, err
	}

	return &device, nil
}

func (r *DeviceRepository) UpdateName(deviceID uuid.UUID, userID uuid.UUID, name string) error {
	return r.db.Model(&model.Device{}).
		Where("id = ? AND user_id = ?", deviceID, userID).
		Updates(map[string]any{"name": name}).
		Error
}

func (r *DeviceRepository) Delete(deviceID uuid.UUID, userID uuid.UUID) error {
	return r.db.Where("id = ? AND user_id = ?", deviceID, userID).Delete(&model.Device{}).Error
}

func (r *DeviceRepository) UpdatePublicKey(deviceID uuid.UUID, userID uuid.UUID, publicKey string) error {
	return r.db.Model(&model.Device{}).
		Where("id = ? AND user_id = ?", deviceID, userID).
		Updates(map[string]any{"public_key": publicKey}).
		Error
}

func (r *DeviceRepository) UpdateLastSeen(deviceID uuid.UUID, at time.Time) error {
	return r.db.Model(&model.Device{}).
		Where("id = ?", deviceID).
		Update("last_seen_at", at).
		Error
}
