package repository

import (
	"errors"

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

func (r *ReleaseRepository) CreateOrUpdateWithAssets(release *model.AppRelease, assets []model.ReleaseAsset) (bool, []model.ReleaseAsset, error) {
	created := false
	staleAssets := make([]model.ReleaseAsset, 0)
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var stored model.AppRelease
		err := tx.Where("platform = ? AND version = ?", release.Platform, release.Version).First(&stored).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.Create(release).Error; err != nil {
				return err
			}
			for index := range assets {
				assets[index].ReleaseID = release.ID
				if err := tx.Create(&assets[index]).Error; err != nil {
					return err
				}
			}
			created = true
			return nil
		}
		if err != nil {
			return err
		}

		if err := tx.Model(&stored).Updates(map[string]any{
			"release_notes": release.ReleaseNotes,
			"published_at":  release.PublishedAt,
		}).Error; err != nil {
			return err
		}

		var storedAssets []model.ReleaseAsset
		if err := tx.Where("release_id = ?", stored.ID).Find(&storedAssets).Error; err != nil {
			return err
		}
		assetsByName := make(map[string]model.ReleaseAsset, len(assets))
		for _, asset := range assets {
			assetsByName[asset.FileName] = asset
		}

		for _, storedAsset := range storedAssets {
			asset, ok := assetsByName[storedAsset.FileName]
			if !ok {
				if err := tx.Delete(&storedAsset).Error; err != nil {
					return err
				}
				staleAssets = append(staleAssets, storedAsset)
				continue
			}
			if err := tx.Model(&storedAsset).Updates(map[string]any{
				"file_size":         asset.FileSize,
				"file_path":         asset.FilePath,
				"source_updated_at": asset.SourceUpdatedAt,
			}).Error; err != nil {
				return err
			}
			delete(assetsByName, storedAsset.FileName)
		}

		for _, asset := range assetsByName {
			asset.ReleaseID = stored.ID
			if err := tx.Create(&asset).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return created, staleAssets, err
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
