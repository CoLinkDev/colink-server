package repository

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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

func (r *TokenRepository) FindByTokenHashForUpdate(tokenHash string) (*model.RefreshToken, error) {
	var token model.RefreshToken
	if err := r.db.
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("token_hash = ?", tokenHash).
		First(&token).Error; err != nil {
		return nil, err
	}

	return &token, nil
}

func (r *TokenRepository) MarkReused(
	tokenHash string,
	rotatedAt time.Time,
	reuseExpiresAt time.Time,
	replacementAccessToken string,
	replacementRefreshToken string,
	replacementAccessExpiresAt time.Time,
	replacementRefreshExpiresAt time.Time,
) error {
	return r.db.Model(&model.RefreshToken{}).
		Where("token_hash = ?", tokenHash).
		Updates(map[string]any{
			"rotated_at":                     rotatedAt,
			"reuse_expires_at":               reuseExpiresAt,
			"replacement_access_token":       replacementAccessToken,
			"replacement_refresh_token":      replacementRefreshToken,
			"replacement_access_expires_at":  replacementAccessExpiresAt,
			"replacement_refresh_expires_at": replacementRefreshExpiresAt,
		}).
		Error
}

func (r *TokenRepository) ExpireReuseByTokenHash(tokenHash string) error {
	return r.db.Model(&model.RefreshToken{}).
		Where("token_hash = ?", tokenHash).
		Updates(expiredReuseUpdates()).
		Error
}

func (r *TokenRepository) ExpireReusableTokens(now time.Time) error {
	return r.db.Model(&model.RefreshToken{}).
		Where("rotated_at IS NOT NULL").
		Where("reuse_expires_at <= ?", now).
		Where("revoked = ?", false).
		Updates(expiredReuseUpdates()).
		Error
}

func (r *TokenRepository) RevokeByTokenHash(tokenHash string) error {
	return r.db.Model(&model.RefreshToken{}).
		Where("token_hash = ?", tokenHash).
		Updates(expiredReuseUpdates()).
		Error
}

func (r *TokenRepository) RevokeByTokenHashOrReplacementRefreshToken(
	tokenHash string,
	replacementRefreshToken string,
) error {
	return r.db.Model(&model.RefreshToken{}).
		Where("token_hash = ? OR replacement_refresh_token = ?", tokenHash, replacementRefreshToken).
		Updates(expiredReuseUpdates()).
		Error
}

func (r *TokenRepository) RevokeAllByUserID(userID uuid.UUID) error {
	return r.db.Model(&model.RefreshToken{}).
		Where("user_id = ?", userID).
		Updates(expiredReuseUpdates()).
		Error
}

func (r *TokenRepository) DeleteExpired(now time.Time) error {
	return r.db.Where("expires_at <= ?", now).Delete(&model.RefreshToken{}).Error
}

func expiredReuseUpdates() map[string]any {
	return map[string]any{
		"revoked":                        true,
		"replacement_access_token":       nil,
		"replacement_refresh_token":      nil,
		"replacement_access_expires_at":  nil,
		"replacement_refresh_expires_at": nil,
	}
}
