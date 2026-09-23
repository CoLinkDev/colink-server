package repository

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"colink-server/internal/model"
)

type NoteAttachmentRepository struct {
	db *gorm.DB
}

func NewNoteAttachmentRepository(db *gorm.DB) *NoteAttachmentRepository {
	return &NoteAttachmentRepository{db: db}
}

func (r *NoteAttachmentRepository) WithTx(tx *gorm.DB) *NoteAttachmentRepository {
	return &NoteAttachmentRepository{db: tx}
}

func (r *NoteAttachmentRepository) FindByIDAndUserID(attachmentID uuid.UUID, userID uuid.UUID) (*model.NoteAttachment, error) {
	var attachment model.NoteAttachment
	if err := r.db.Where("id = ? AND user_id = ?", attachmentID, userID).First(&attachment).Error; err != nil {
		return nil, err
	}

	return &attachment, nil
}

// FindActiveByIDs returns the live attachments among the given IDs owned by
// the account, ordered by id.
func (r *NoteAttachmentRepository) FindActiveByIDs(userID uuid.UUID, attachmentIDs []uuid.UUID) ([]model.NoteAttachment, error) {
	if len(attachmentIDs) == 0 {
		return []model.NoteAttachment{}, nil
	}

	var attachments []model.NoteAttachment
	if err := r.db.Where("user_id = ? AND id IN ? AND deleted_at IS NULL", userID, attachmentIDs).
		Order("id ASC").
		Find(&attachments).Error; err != nil {
		return nil, err
	}

	return attachments, nil
}

func (r *NoteAttachmentRepository) Create(attachment *model.NoteAttachment) error {
	return r.db.Create(attachment).Error
}

// Tombstone soft-deletes the attachment. StoragePath is retained until the
// content file has been removed so cleanup can retry transient failures.
func (r *NoteAttachmentRepository) Tombstone(tx *gorm.DB, userID uuid.UUID, attachmentID uuid.UUID, at time.Time) error {
	return tx.Model(&model.NoteAttachment{}).
		Where("user_id = ? AND id = ? AND deleted_at IS NULL", userID, attachmentID).
		Updates(map[string]any{
			"deleted_at": at,
		}).Error
}

func (r *NoteAttachmentRepository) MarkAssociated(tx *gorm.DB, userID uuid.UUID, attachmentIDs []uuid.UUID) error {
	if len(attachmentIDs) == 0 {
		return nil
	}

	return tx.Model(&model.NoteAttachment{}).
		Where("user_id = ? AND id IN ? AND deleted_at IS NULL", userID, attachmentIDs).
		Update("unassociated_since", nil).Error
}

// MarkUnassociated records when attachments lose their last note relation.
// Existing timestamps are preserved so staged attachments retain upload time.
func (r *NoteAttachmentRepository) MarkUnassociated(tx *gorm.DB, userID uuid.UUID, attachmentIDs []uuid.UUID, at time.Time) error {
	if len(attachmentIDs) == 0 {
		return nil
	}

	return tx.Exec(
		`UPDATE note_attachments a
		    SET unassociated_since = COALESCE(a.unassociated_since, ?)
		  WHERE a.user_id = ?
		    AND a.id IN ?
		    AND a.deleted_at IS NULL
		    AND NOT EXISTS (
		        SELECT 1 FROM note_attachment_relations r
		         WHERE r.user_id = a.user_id AND r.attachment_id = a.id
		    )`,
		at, userID, attachmentIDs,
	).Error
}

// ListUnassociatedOlderThan returns live attachments whose current
// unassociated period began before the cutoff, in ascending id order.
func (r *NoteAttachmentRepository) ListUnassociatedOlderThan(tx *gorm.DB, cutoff time.Time, limit int) ([]model.NoteAttachment, error) {
	var attachments []model.NoteAttachment
	if err := tx.Raw(
		`SELECT a.* FROM note_attachments a
		 WHERE a.deleted_at IS NULL
		   AND a.unassociated_since < ?
		 ORDER BY a.unassociated_since ASC, a.id ASC
		 LIMIT ?
		 FOR UPDATE OF a SKIP LOCKED`,
		cutoff, limit,
	).Scan(&attachments).Error; err != nil {
		return nil, err
	}

	return attachments, nil
}

// TombstoneIfUnreferenced rechecks relations after the attachment lock has
// been acquired. The separate statement receives a fresh READ COMMITTED
// snapshot and therefore sees associations committed while lock acquisition
// was waiting.
func (r *NoteAttachmentRepository) TombstoneIfUnreferenced(tx *gorm.DB, attachment model.NoteAttachment, at time.Time) (bool, error) {
	result := tx.Exec(
		`UPDATE note_attachments a
		    SET deleted_at = ?
		  WHERE a.user_id = ? AND a.id = ? AND a.deleted_at IS NULL
		    AND NOT EXISTS (
		        SELECT 1 FROM note_attachment_relations r
		         WHERE r.user_id = a.user_id AND r.attachment_id = a.id
		    )`,
		at, attachment.UserID, attachment.ID,
	)
	return result.RowsAffected == 1, result.Error
}

func (r *NoteAttachmentRepository) ClearUnassociatedState(tx *gorm.DB, userID uuid.UUID, attachmentID uuid.UUID) error {
	return tx.Model(&model.NoteAttachment{}).
		Where("user_id = ? AND id = ? AND deleted_at IS NULL", userID, attachmentID).
		Update("unassociated_since", nil).Error
}

func (r *NoteAttachmentRepository) ListTombstonedWithStoragePath(limit int) ([]model.NoteAttachment, error) {
	var attachments []model.NoteAttachment
	if err := r.db.Where("deleted_at IS NOT NULL AND storage_path <> ''").
		Order("deleted_at ASC, id ASC").
		Limit(limit).
		Find(&attachments).Error; err != nil {
		return nil, err
	}

	return attachments, nil
}

func (r *NoteAttachmentRepository) ClearStoragePath(userID uuid.UUID, attachmentID uuid.UUID) error {
	return r.db.Model(&model.NoteAttachment{}).
		Where("user_id = ? AND id = ? AND deleted_at IS NOT NULL", userID, attachmentID).
		Update("storage_path", "").Error
}

func (r *NoteAttachmentRepository) FindStoragePaths(candidates []string) ([]string, error) {
	if len(candidates) == 0 {
		return []string{}, nil
	}

	var paths []string
	if err := r.db.Model(&model.NoteAttachment{}).
		Where("storage_path IN ?", candidates).
		Pluck("storage_path", &paths).Error; err != nil {
		return nil, err
	}

	return paths, nil
}

// LockAttachment locks one attachment row for the deletion transaction.
func (r *NoteAttachmentRepository) LockAttachment(tx *gorm.DB, userID uuid.UUID, attachmentID uuid.UUID) (*model.NoteAttachment, error) {
	var attachment model.NoteAttachment
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND id = ?", userID, attachmentID).
		First(&attachment).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return &attachment, nil
}
