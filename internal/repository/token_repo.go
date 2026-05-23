package repository

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"colink-server/internal/model"
)

type TokenRepository struct {
	db *gorm.DB
}

func NewTokenRepository(db *gorm.DB) *TokenRepository {
	return &TokenRepository{db: db}
}

func (r *TokenRepository) WithTx(tx *gorm.DB) *TokenRepository {
	return &TokenRepository{db: tx}
}

func (r *TokenRepository) Create(token *model.RefreshToken) error {
	return r.db.Create(token).Error
}

func (r *TokenRepository) FindByTokenHash(tokenHash string) (*model.RefreshToken, error) {
	var token model.RefreshToken
	if err := r.db.Where("token_hash = ?", tokenHash).First(&token).Error; err != nil {
		return nil, err
	}

	return &token, nil
}

func (r *TokenRepository) RevokeByTokenHash(tokenHash string) error {
	return r.db.Model(&model.RefreshToken{}).
		Where("token_hash = ?", tokenHash).
		Update("revoked", true).
		Error
}

func (r *TokenRepository) RevokeAllByUserID(userID uuid.UUID) error {
	return r.db.Model(&model.RefreshToken{}).
		Where("user_id = ?", userID).
		Update("revoked", true).
		Error
}

func (r *TokenRepository) DeleteExpired(now time.Time) error {
	return r.db.Where("expires_at <= ?", now).Delete(&model.RefreshToken{}).Error
}
