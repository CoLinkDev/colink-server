package model

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID           uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	Email        string    `gorm:"size:255;not null;uniqueIndex:idx_users_email"`
	Username     string    `gorm:"size:255;not null;uniqueIndex:idx_users_username"`
	PasswordHash string    `gorm:"size:255;not null"`
	Disabled     bool      `gorm:"not null;default:false"`
	CreatedAt    time.Time `gorm:"not null;default:now()"`
	UpdatedAt    time.Time `gorm:"not null;default:now()"`
}
