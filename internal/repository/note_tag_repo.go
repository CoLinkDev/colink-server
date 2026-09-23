package repository

import (
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"colink-server/internal/model"
)

type NoteTagRepository struct {
	db *gorm.DB
}

func NewNoteTagRepository(db *gorm.DB) *NoteTagRepository {
	return &NoteTagRepository{db: db}
}

func (r *NoteTagRepository) WithTx(tx *gorm.DB) *NoteTagRepository {
	return &NoteTagRepository{db: tx}
}

func (r *NoteTagRepository) FindByIDAndUserID(tagID uuid.UUID, userID uuid.UUID) (*model.NoteTag, error) {
	var tag model.NoteTag
	if err := r.db.Where("id = ? AND user_id = ?", tagID, userID).First(&tag).Error; err != nil {
		return nil, err
	}

	return &tag, nil
}

func (r *NoteTagRepository) FindByIDAndUserIDForUpdate(tx *gorm.DB, tagID uuid.UUID, userID uuid.UUID) (*model.NoteTag, error) {
	var tag model.NoteTag
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND user_id = ?", tagID, userID).
		First(&tag).Error; err != nil {
		return nil, err
	}

	return &tag, nil
}

// FindActiveByNormalizedName returns the active tag with the given
// normalized name, if any.
func (r *NoteTagRepository) FindActiveByNormalizedName(tx *gorm.DB, userID uuid.UUID, nameNormalized string) (*model.NoteTag, error) {
	var tag model.NoteTag
	err := tx.Where(
		"user_id = ? AND name_normalized = ? AND deleted_at IS NULL",
		userID, nameNormalized,
	).First(&tag).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return &tag, nil
}

// FindActiveByIDs returns the active tags among the given IDs owned by the
// account, ordered by id.
func (r *NoteTagRepository) FindActiveByIDs(userID uuid.UUID, tagIDs []uuid.UUID) ([]model.NoteTag, error) {
	if len(tagIDs) == 0 {
		return []model.NoteTag{}, nil
	}

	var tags []model.NoteTag
	if err := r.db.Where("user_id = ? AND id IN ? AND deleted_at IS NULL", userID, tagIDs).
		Order("id ASC").
		Find(&tags).Error; err != nil {
		return nil, err
	}

	return tags, nil
}

func (r *NoteTagRepository) Create(tag *model.NoteTag) error {
	return r.db.Create(tag).Error
}

func (r *NoteTagRepository) ListActiveByUserID(userID uuid.UUID) ([]model.NoteTag, error) {
	var tags []model.NoteTag
	if err := r.db.Where("user_id = ? AND deleted_at IS NULL", userID).Order("created_at ASC, id ASC").Find(&tags).Error; err != nil {
		return nil, err
	}

	return tags, nil
}

// ListSnapshotTags returns one keyset page of active tags whose most recent
// change sequence is within the fixed snapshot sequence.
func (r *NoteTagRepository) ListSnapshotTags(userID uuid.UUID, maxSeq int64, afterSeq int64, afterID uuid.UUID, limit int) ([]model.NoteTag, error) {
	query := r.db.Where(
		"user_id = ? AND deleted_at IS NULL AND change_seq <= ?",
		userID, maxSeq,
	)
	if afterSeq > 0 || afterID != uuid.Nil {
		query = query.Where("(change_seq, id) > (?, ?)", afterSeq, afterID)
	}

	var tags []model.NoteTag
	if err := query.Order("change_seq ASC, id ASC").Limit(limit).Find(&tags).Error; err != nil {
		return nil, err
	}

	return tags, nil
}

// ListNoteIDsForTag returns IDs of the account's active notes carrying the
// tag, in ascending note id order.
func (r *NoteTagRepository) ListNoteIDsForTag(tx *gorm.DB, userID uuid.UUID, tagID uuid.UUID) ([]uuid.UUID, error) {
	var noteIDs []uuid.UUID
	if err := tx.Table("note_tag_relations r").
		Joins("JOIN notes n ON n.user_id = r.user_id AND n.id = r.note_id").
		Where("r.user_id = ? AND r.tag_id = ? AND n.deleted_at IS NULL", userID, tagID).
		Order("n.id ASC").
		Pluck("n.id", &noteIDs).Error; err != nil {
		return nil, err
	}

	return noteIDs, nil
}

func (r *NoteTagRepository) RemoveTagRelations(tx *gorm.DB, userID uuid.UUID, tagID uuid.UUID) error {
	return tx.Where("user_id = ? AND tag_id = ?", userID, tagID).Delete(&model.NoteTagRelation{}).Error
}

// ExistsActiveReference reports whether any note still carries the tag.
func (r *NoteTagRepository) ExistsActiveReference(tx *gorm.DB, userID uuid.UUID, tagID uuid.UUID) (bool, error) {
	var count int64
	if err := tx.Model(&model.NoteTagRelation{}).
		Where("user_id = ? AND tag_id = ?", userID, tagID).
		Count(&count).Error; err != nil {
		return false, err
	}

	return count > 0, nil
}
