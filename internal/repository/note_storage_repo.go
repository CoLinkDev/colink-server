package repository

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"colink-server/internal/model"
)

type NoteStorageRepository struct{}

func NewNoteStorageRepository() *NoteStorageRepository {
	return &NoteStorageRepository{}
}

func (r *NoteStorageRepository) Get(tx *gorm.DB, userID uuid.UUID) (*model.NoteStorageUsage, error) {
	var usage model.NoteStorageUsage
	if err := tx.Where("user_id = ?", userID).Take(&usage).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &model.NoteStorageUsage{UserID: userID}, nil
		}
		return nil, err
	}
	return &usage, nil
}

func (r *NoteStorageRepository) AddMarkdown(tx *gorm.DB, userID uuid.UUID, delta int64) error {
	if delta < 0 {
		result := tx.Exec(`UPDATE note_storage_usage SET markdown_bytes = markdown_bytes + ? WHERE user_id = ?`, delta, userID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("missing notes storage usage for account %s", userID)
		}
		return nil
	}
	return tx.Exec(`INSERT INTO note_storage_usage (user_id, markdown_bytes)
		VALUES (?, ?)
		ON CONFLICT (user_id) DO UPDATE
		SET markdown_bytes = note_storage_usage.markdown_bytes + EXCLUDED.markdown_bytes`,
		userID, delta).Error
}

func (r *NoteStorageRepository) AddAttachment(tx *gorm.DB, userID uuid.UUID, delta int64) error {
	if delta < 0 {
		result := tx.Exec(`UPDATE note_storage_usage SET attachment_bytes = attachment_bytes + ? WHERE user_id = ?`, delta, userID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("missing notes storage usage for account %s", userID)
		}
		return nil
	}
	return tx.Exec(`INSERT INTO note_storage_usage (user_id, attachment_bytes)
		VALUES (?, ?)
		ON CONFLICT (user_id) DO UPDATE
		SET attachment_bytes = note_storage_usage.attachment_bytes + EXCLUDED.attachment_bytes`,
		userID, delta).Error
}
