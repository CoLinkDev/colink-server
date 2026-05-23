package model

import (
	"time"

	"github.com/google/uuid"
)

type Device struct {
	ID           uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	UserID       uuid.UUID  `gorm:"type:uuid;not null;index:idx_devices_user_id"`
	Name         string     `gorm:"size:100;not null"`
	Type         string     `gorm:"size:20;not null;check:chk_device_type,type IN ('windows','android','macos','linux','ios')"`
	PublicKey    string     `gorm:"type:text;not null"`
	DeviceSecret string     `gorm:"size:128;not null"`
	LastSeenAt   *time.Time `gorm:"type:timestamptz"`
	CreatedAt    time.Time  `gorm:"not null;default:now()"`
	UpdatedAt    time.Time  `gorm:"not null;default:now()"`
}
