package model

import (
	"time"

	"github.com/google/uuid"
)

type RefreshToken struct {
	ID                          uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	UserID                      uuid.UUID  `gorm:"type:uuid;not null;index:idx_refresh_tokens_user_id"`
	TokenHash                   string     `gorm:"size:255;not null;uniqueIndex:idx_refresh_tokens_token_hash"`
	ExpiresAt                   time.Time  `gorm:"not null;index:idx_refresh_tokens_expires_at"`
	Revoked                     bool       `gorm:"not null;default:false"`
	RotatedAt                   *time.Time `gorm:"index:idx_refresh_tokens_reuse_expires_at"`
	ReuseExpiresAt              *time.Time `gorm:"index:idx_refresh_tokens_reuse_expires_at"`
	ReplacementAccessToken      *string
	ReplacementRefreshToken     *string
	ReplacementAccessExpiresAt  *time.Time
	ReplacementRefreshExpiresAt *time.Time
	CreatedAt                   time.Time `gorm:"not null;default:now()"`
}
