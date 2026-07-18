package model

import (
	"time"

	"github.com/google/uuid"
)

type AppRelease struct {
	ID           uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	Platform     string    `gorm:"size:20;not null;uniqueIndex:idx_release_platform_version;index:idx_app_releases_platform_published_at,priority:1"`
	Version      string    `gorm:"size:50;not null;uniqueIndex:idx_release_platform_version"`
	ReleaseNotes string    `gorm:"type:text"`
	PublishedAt  time.Time `gorm:"type:timestamptz;not null;index:idx_app_releases_platform_published_at,sort:desc,priority:2"`
	CreatedAt    time.Time `gorm:"not null;default:now()"`
}

type ReleaseAsset struct {
	ID              uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	ReleaseID       uuid.UUID `gorm:"type:uuid;not null;index:idx_asset_release_id;uniqueIndex:idx_asset_release_file;constraint:OnDelete:CASCADE"`
	FileName        string    `gorm:"size:255;not null;uniqueIndex:idx_asset_release_file"`
	FileSize        int64     `gorm:"not null"`
	FilePath        string    `gorm:"size:500;not null"`
	SourceUpdatedAt *time.Time `gorm:"type:timestamptz"`
	CreatedAt       time.Time `gorm:"not null;default:now()"`
}
