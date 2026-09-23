package repository

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"colink-server/internal/model"
)

type NoteRepository struct {
	db *gorm.DB
}

const noteSummaryColumns = "notes.user_id, notes.id, notes.title, notes.revision, notes.created_at, notes.updated_at"

func NewNoteRepository(db *gorm.DB) *NoteRepository {
	return &NoteRepository{db: db}
}

func (r *NoteRepository) WithTx(tx *gorm.DB) *NoteRepository {
	return &NoteRepository{db: tx}
}

// LockTags locks the account's tag rows in ascending id order so that
// every notes write transaction acquires tag locks before note locks.
func (r *NoteRepository) LockTags(tx *gorm.DB, userID uuid.UUID, tagIDs []uuid.UUID) error {
	if len(tagIDs) == 0 {
		return nil
	}
	return tx.Raw(
		`SELECT id FROM note_tags WHERE user_id = ? AND id IN ? ORDER BY id ASC FOR UPDATE`,
		userID, tagIDs,
	).Scan(&[]uuid.UUID{}).Error
}

// LockNotes locks the account's note rows in ascending id order.
func (r *NoteRepository) LockNotes(tx *gorm.DB, userID uuid.UUID, noteIDs []uuid.UUID) error {
	if len(noteIDs) == 0 {
		return nil
	}
	return tx.Raw(
		`SELECT id FROM notes WHERE user_id = ? AND id IN ? ORDER BY id ASC FOR UPDATE`,
		userID, noteIDs,
	).Scan(&[]uuid.UUID{}).Error
}

// LockAttachments locks the account's attachment rows in ascending id order.
func (r *NoteRepository) LockAttachments(tx *gorm.DB, userID uuid.UUID, attachmentIDs []uuid.UUID) error {
	if len(attachmentIDs) == 0 {
		return nil
	}
	return tx.Raw(
		`SELECT id FROM note_attachments WHERE user_id = ? AND id IN ? ORDER BY id ASC FOR UPDATE`,
		userID, attachmentIDs,
	).Scan(&[]uuid.UUID{}).Error
}

// LockUserRow serializes account-level quota checks.
func (r *NoteRepository) LockUserRow(tx *gorm.DB, userID uuid.UUID) error {
	return tx.Raw(`SELECT id FROM users WHERE id = ? FOR UPDATE`, userID).Scan(&[]uuid.UUID{}).Error
}

func (r *NoteRepository) FindByIDAndUserID(noteID uuid.UUID, userID uuid.UUID) (*model.Note, error) {
	var note model.Note
	if err := r.db.Where("id = ? AND user_id = ?", noteID, userID).First(&note).Error; err != nil {
		return nil, err
	}

	return &note, nil
}

func (r *NoteRepository) FindByIDAndUserIDForUpdate(tx *gorm.DB, noteID uuid.UUID, userID uuid.UUID) (*model.Note, error) {
	var note model.Note
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND user_id = ?", noteID, userID).
		First(&note).Error; err != nil {
		return nil, err
	}

	return &note, nil
}

func (r *NoteRepository) Create(note *model.Note) error {
	return r.db.Create(note).Error
}

func (r *NoteRepository) ListActiveByUserID(userID uuid.UUID) ([]model.Note, error) {
	var notes []model.Note
	if err := r.db.Where("user_id = ? AND deleted_at IS NULL", userID).Find(&notes).Error; err != nil {
		return nil, err
	}

	return notes, nil
}

// ListNotesPage returns one keyset page of active notes ordered by
// updated_at desc, id asc.
func (r *NoteRepository) ListNotesPage(userID uuid.UUID, beforeUpdatedAt *time.Time, beforeID *uuid.UUID, limit int) ([]model.Note, error) {
	query := r.db.Where("user_id = ? AND deleted_at IS NULL", userID)
	if beforeUpdatedAt != nil && beforeID != nil {
		query = query.Where("updated_at < ? OR (updated_at = ? AND id > ?)", *beforeUpdatedAt, *beforeUpdatedAt, *beforeID)
	}

	var notes []model.Note
	if err := query.Select(noteSummaryColumns).Order("updated_at DESC, id ASC").Limit(limit).Find(&notes).Error; err != nil {
		return nil, err
	}

	return notes, nil
}

func (r *NoteRepository) ListNoteIDsByTag(userID uuid.UUID, tagID uuid.UUID, beforeUpdatedAt *time.Time, beforeID *uuid.UUID, limit int) ([]model.Note, error) {
	query := r.db.
		Joins("JOIN note_tag_relations r ON r.user_id = notes.user_id AND r.note_id = notes.id").
		Where("notes.user_id = ? AND notes.deleted_at IS NULL AND r.tag_id = ?", userID, tagID)
	if beforeUpdatedAt != nil && beforeID != nil {
		query = query.Where("notes.updated_at < ? OR (notes.updated_at = ? AND notes.id > ?)", *beforeUpdatedAt, *beforeUpdatedAt, *beforeID)
	}

	var notes []model.Note
	if err := query.Select(noteSummaryColumns).Order("notes.updated_at DESC, notes.id ASC").Limit(limit).Find(&notes).Error; err != nil {
		return nil, err
	}

	return notes, nil
}

func (r *NoteRepository) ListTagIDsForNote(userID uuid.UUID, noteID uuid.UUID) ([]uuid.UUID, error) {
	var tagIDs []uuid.UUID
	if err := r.db.Model(&model.NoteTagRelation{}).
		Where("user_id = ? AND note_id = ?", userID, noteID).
		Order("tag_id ASC").
		Pluck("tag_id", &tagIDs).Error; err != nil {
		return nil, err
	}

	return tagIDs, nil
}

func (r *NoteRepository) ListTagIDsForNotes(userID uuid.UUID, noteIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	if len(noteIDs) == 0 {
		return map[uuid.UUID][]uuid.UUID{}, nil
	}

	var rows []model.NoteTagRelation
	if err := r.db.Where("user_id = ? AND note_id IN ?", userID, noteIDs).Order("tag_id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}

	result := make(map[uuid.UUID][]uuid.UUID, len(rows))
	for _, row := range rows {
		result[row.NoteID] = append(result[row.NoteID], row.TagID)
	}

	return result, nil
}

func (r *NoteRepository) ListAttachmentCountsForNotes(userID uuid.UUID, noteIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	if len(noteIDs) == 0 {
		return map[uuid.UUID]int64{}, nil
	}

	var rows []struct {
		NoteID uuid.UUID `gorm:"column:note_id"`
		Count  int64     `gorm:"column:count"`
	}
	if err := r.db.Model(&model.NoteAttachmentRelation{}).
		Select("note_id, COUNT(*) AS count").
		Where("user_id = ? AND note_id IN ?", userID, noteIDs).
		Group("note_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	result := make(map[uuid.UUID]int64, len(rows))
	for _, row := range rows {
		result[row.NoteID] = row.Count
	}

	return result, nil
}

func (r *NoteRepository) ListAttachmentIDsForNote(userID uuid.UUID, noteID uuid.UUID) ([]uuid.UUID, error) {
	var attachmentIDs []uuid.UUID
	if err := r.db.Model(&model.NoteAttachmentRelation{}).
		Where("user_id = ? AND note_id = ?", userID, noteID).
		Order("attachment_id ASC").
		Pluck("attachment_id", &attachmentIDs).Error; err != nil {
		return nil, err
	}

	return attachmentIDs, nil
}

func (r *NoteRepository) ListAttachmentsForNotes(userID uuid.UUID, noteIDs []uuid.UUID) (map[uuid.UUID][]model.NoteAttachment, error) {
	if len(noteIDs) == 0 {
		return map[uuid.UUID][]model.NoteAttachment{}, nil
	}

	var rows []struct {
		model.NoteAttachment
		NoteID uuid.UUID `gorm:"column:note_id"`
	}
	if err := r.db.
		Table("note_attachments a").
		Select("a.*, r.note_id AS note_id").
		Joins("JOIN note_attachment_relations r ON r.user_id = a.user_id AND r.attachment_id = a.id").
		Where("r.user_id = ? AND r.note_id IN ?", userID, noteIDs).
		Order("a.id ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	result := make(map[uuid.UUID][]model.NoteAttachment, len(rows))
	for _, row := range rows {
		result[row.NoteID] = append(result[row.NoteID], row.NoteAttachment)
	}

	return result, nil
}

func (r *NoteRepository) ReplaceTagRelations(noteID uuid.UUID, userID uuid.UUID, tagIDs []uuid.UUID) error {
	if err := r.db.Where("user_id = ? AND note_id = ?", userID, noteID).Delete(&model.NoteTagRelation{}).Error; err != nil {
		return err
	}
	if len(tagIDs) == 0 {
		return nil
	}

	relations := make([]model.NoteTagRelation, 0, len(tagIDs))
	for _, tagID := range tagIDs {
		relations = append(relations, model.NoteTagRelation{NoteID: noteID, TagID: tagID, UserID: userID})
	}

	return r.db.Create(&relations).Error
}

func (r *NoteRepository) ReplaceAttachmentRelations(noteID uuid.UUID, userID uuid.UUID, attachmentIDs []uuid.UUID) error {
	if err := r.db.Where("user_id = ? AND note_id = ?", userID, noteID).Delete(&model.NoteAttachmentRelation{}).Error; err != nil {
		return err
	}
	if len(attachmentIDs) == 0 {
		return nil
	}

	relations := make([]model.NoteAttachmentRelation, 0, len(attachmentIDs))
	for _, attachmentID := range attachmentIDs {
		relations = append(relations, model.NoteAttachmentRelation{NoteID: noteID, AttachmentID: attachmentID, UserID: userID})
	}

	return r.db.Create(&relations).Error
}

// ListReferencedNoteIDs returns the active notes of the account that
// reference the given attachment, ordered lexicographically.
func (r *NoteRepository) ListReferencedNoteIDs(userID uuid.UUID, attachmentID uuid.UUID, beforeID *uuid.UUID, limit int) ([]uuid.UUID, error) {
	query := r.db.
		Table("note_attachment_relations r").
		Joins("JOIN notes n ON n.user_id = r.user_id AND n.id = r.note_id").
		Where("r.user_id = ? AND r.attachment_id = ? AND n.deleted_at IS NULL", userID, attachmentID)
	if beforeID != nil {
		query = query.Where("r.note_id > ?", *beforeID)
	}

	var noteIDs []uuid.UUID
	if err := query.Order("r.note_id ASC").Limit(limit).Pluck("r.note_id", &noteIDs).Error; err != nil {
		return nil, err
	}

	return noteIDs, nil
}

// ListSnapshotNotes returns one keyset page of active notes whose most
// recent change sequence is within the fixed snapshot sequence.
func (r *NoteRepository) ListSnapshotNotes(userID uuid.UUID, maxSeq int64, afterSeq int64, afterID uuid.UUID, limit int) ([]model.Note, error) {
	query := r.db.Where(
		"user_id = ? AND deleted_at IS NULL AND change_seq <= ?",
		userID, maxSeq,
	)
	if afterSeq > 0 || afterID != uuid.Nil {
		query = query.Where("(change_seq, id) > (?, ?)", afterSeq, afterID)
	}

	var notes []model.Note
	if err := query.Order("change_seq ASC, id ASC").Limit(limit).Find(&notes).Error; err != nil {
		return nil, err
	}

	return notes, nil
}

// CountActiveReferences returns how many active notes of the account
// reference the given attachment.
func (r *NoteRepository) CountActiveReferences(tx *gorm.DB, userID uuid.UUID, attachmentID uuid.UUID) (int64, error) {
	var count int64
	if err := tx.Table("note_attachment_relations r").
		Joins("JOIN notes n ON n.user_id = r.user_id AND n.id = r.note_id").
		Where("r.user_id = ? AND r.attachment_id = ? AND n.deleted_at IS NULL", userID, attachmentID).
		Count(&count).Error; err != nil {
		return 0, err
	}

	return count, nil
}
