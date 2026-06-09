package repository

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	"colink-server/internal/model"
)

type ReleaseRepository struct {
	db *gorm.DB
}

func NewReleaseRepository(db *gorm.DB) *ReleaseRepository {
	return &ReleaseRepository{db: db}
}

func (r *ReleaseRepository) FindLatestByPlatform(platform string) (*model.AppRelease, error) {
	var release model.AppRelease
	if err := r.db.
		Where("platform = ?", platform).
		Order("published_at desc, created_at desc").
		First(&release).Error; err != nil {
		return nil, err
	}

	return &release, nil
}

func (r *ReleaseRepository) FindByPlatformAndVersion(platform, version string) (*model.AppRelease, error) {
	var release model.AppRelease
	if err := r.db.
		Where("platform = ? AND version = ?", platform, version).
		First(&release).Error; err != nil {
		return nil, err
	}

	return &release, nil
}

func (r *ReleaseRepository) ExistsByPlatformAndVersion(platform, version string) (bool, error) {
	var count int64
	if err := r.db.Model(&model.AppRelease{}).
		Where("platform = ? AND version = ?", platform, version).
		Count(&count).Error; err != nil {
		return false, err
	}

	return count > 0, nil
}

func (r *ReleaseRepository) Create(release *model.AppRelease) error {
	return r.db.Create(release).Error
}

func (r *ReleaseRepository) CreateWithAssets(release *model.AppRelease, assets []model.ReleaseAsset) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(release).Error; err != nil {
			return err
		}
		for index := range assets {
			assets[index].ReleaseID = release.ID
			if err := tx.Create(&assets[index]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *ReleaseRepository) FindAssetsByReleaseID(releaseID uuid.UUID) ([]model.ReleaseAsset, error) {
	var assets []model.ReleaseAsset
	if err := r.db.
		Where("release_id = ?", releaseID).
		Order("file_name asc").
		Find(&assets).Error; err != nil {
		return nil, err
	}

	return assets, nil
}

func (r *ReleaseRepository) FindAssetByReleaseIDAndFileName(releaseID uuid.UUID, fileName string) (*model.ReleaseAsset, error) {
	var asset model.ReleaseAsset
	if err := r.db.
		Where("release_id = ? AND file_name = ?", releaseID, fileName).
		First(&asset).Error; err != nil {
		return nil, err
	}

	return &asset, nil
}

func (r *ReleaseRepository) CreateAsset(asset *model.ReleaseAsset) error {
	return r.db.Create(asset).Error
}
