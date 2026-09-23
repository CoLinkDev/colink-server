package service

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"colink-server/internal/config"
	"colink-server/internal/model"
	"colink-server/internal/pkg"
	"colink-server/internal/repository"
)

type NoteService struct {
	db             *gorm.DB
	noteRepo       *repository.NoteRepository
	tagRepo        *repository.NoteTagRepository
	attachmentRepo *repository.NoteAttachmentRepository
	changeRepo     *repository.NoteChangeLogRepository
	storageRepo    *repository.NoteStorageRepository
	cfg            config.NotesConfig
}

func NewNoteService(
	db *gorm.DB,
	noteRepo *repository.NoteRepository,
	tagRepo *repository.NoteTagRepository,
	attachmentRepo *repository.NoteAttachmentRepository,
	changeRepo *repository.NoteChangeLogRepository,
	cfg config.NotesConfig,
) *NoteService {
	return &NoteService{
		db:             db,
		noteRepo:       noteRepo,
		tagRepo:        tagRepo,
		attachmentRepo: attachmentRepo,
		changeRepo:     changeRepo,
		storageRepo:    repository.NewNoteStorageRepository(),
		cfg:            cfg,
	}
}

// NoteWriteInput carries the client-provided payload of a note write.
type NoteWriteInput struct {
	Title         string
	Markdown      string
	TagIDs        []string
	AttachmentIDs []string
}

type NoteDeleteResult struct {
	NoteID    string    `json:"noteId"`
	Revision  int64     `json:"revision"`
	DeletedAt time.Time `json:"deletedAt"`
}

func (s *NoteService) MaxWriteRequestBytes() int64 {
	const jsonOverhead = int64(1 << 20)
	if s.cfg.MaxMarkdownBytes > int64(^uint64(0)>>1)-jsonOverhead {
		return int64(^uint64(0) >> 1)
	}
	return s.cfg.MaxMarkdownBytes + jsonOverhead
}

func (s *NoteService) CreateNote(userID string, noteID string, input NoteWriteInput) (*NoteDTO, error) {
	userUUID, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}
	noteUUID, err := parseResourceUUID(noteID)
	if err != nil {
		return nil, err
	}
	tagIDs, err := parseUUIDList(input.TagIDs)
	if err != nil {
		return nil, err
	}
	attachmentIDs, err := parseUUIDList(input.AttachmentIDs)
	if err != nil {
		return nil, err
	}
	if err := validateNoteTitle(input.Title); err != nil {
		return nil, err
	}
	if err := validateAttachmentURIs(input.Markdown, attachmentIDs); err != nil {
		return nil, err
	}
	if int64(len(input.Markdown)) > s.cfg.MaxMarkdownBytes {
		return nil, pkg.NewAppError(http.StatusRequestEntityTooLarge, pkg.CodeNoteStorageLimitReached, "note storage limit reached")
	}

	var result *NoteDTO
	err = s.db.Transaction(func(tx *gorm.DB) error {
		noteRepo := s.noteRepo.WithTx(tx)
		tagRepo := s.tagRepo.WithTx(tx)
		attachmentRepo := s.attachmentRepo.WithTx(tx)
		changeRepo := s.changeRepo.WithTx(tx)

		if err := noteRepo.LockUserRow(tx, userUUID); err != nil {
			return pkg.InternalError(err)
		}
        if err := noteRepo.LockTags(tx, userUUID, tagIDs); err != nil {
			return pkg.InternalError(err)
		}
        if err := noteRepo.LockAttachments(tx, userUUID, attachmentIDs); err != nil {
			return pkg.InternalError(err)
		}

		var existing model.Note
        lookupErr := tx.Where("user_id = ? AND id = ?", userUUID, noteUUID).First(&existing).Error
		if lookupErr == nil {
			return pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidParameter, "invalid parameter")
		}
		if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			return pkg.InternalError(lookupErr)
		}

		if err := validateTagRefs(tagRepo, userUUID, tagIDs); err != nil {
			return err
		}
		if err := validateAttachmentRefs(attachmentRepo, userUUID, attachmentIDs); err != nil {
			return err
		}
		if err := s.ensureMarkdownQuota(tx, userUUID, 0, int64(len(input.Markdown))); err != nil {
			return err
		}

		now := time.Now().UTC()
		note := &model.Note{
			ID:        noteUUID,
			UserID:    userUUID,
			Title:     input.Title,
			Markdown:  input.Markdown,
			Revision:  1,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := noteRepo.Create(note); err != nil {
			return pkg.InternalError(err)
		}
		if err := s.storageRepo.AddMarkdown(tx, userUUID, int64(len(input.Markdown))); err != nil {
			return pkg.InternalError(err)
		}
		if err := noteRepo.ReplaceTagRelations(noteUUID, userUUID, tagIDs); err != nil {
			return pkg.InternalError(err)
		}
		if err := noteRepo.ReplaceAttachmentRelations(noteUUID, userUUID, attachmentIDs); err != nil {
			return pkg.InternalError(err)
		}
		if err := attachmentRepo.MarkAssociated(tx, userUUID, attachmentIDs); err != nil {
			return pkg.InternalError(err)
		}
		seq, err := appendChange(tx, changeRepo, userUUID, "note", noteUUID, "upsert", note.Revision)
		if err != nil {
			return err
		}
        if err := tx.Model(&model.Note{}).Where("user_id = ? AND id = ?", userUUID, noteUUID).Update("change_seq", seq).Error; err != nil {
			return pkg.InternalError(err)
		}
		note.ChangeSeq = seq

		attachments, err := attachmentRepo.FindActiveByIDs(userUUID, attachmentIDs)
		if err != nil {
			return pkg.InternalError(err)
		}
		result = buildNoteDTO(note, uuidStrings(tagIDs), attachments)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *NoteService) GetNote(userID string, noteID string) (*NoteDTO, error) {
	userUUID, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}
	noteUUID, err := parseResourceUUID(noteID)
	if err != nil {
		return nil, err
	}

	var result *NoteDTO
	err = s.db.Transaction(func(tx *gorm.DB) error {
		transactional := *s
		transactional.db = tx
		transactional.noteRepo = s.noteRepo.WithTx(tx)
		transactional.attachmentRepo = s.attachmentRepo.WithTx(tx)

		note, err := transactional.noteRepo.FindByIDAndUserID(noteUUID, userUUID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return pkg.NewAppError(http.StatusNotFound, pkg.CodeNoteNotFound, "note not found")
			}
			return pkg.InternalError(err)
		}
		if note.DeletedAt != nil {
			return pkg.NewAppError(http.StatusNotFound, pkg.CodeNoteNotFound, "note not found")
		}

		tagIDs, err := transactional.noteRepo.ListTagIDsForNote(userUUID, note.ID)
		if err != nil {
			return pkg.InternalError(err)
		}

		result, err = transactional.buildCurrentNoteDTO(note, tagIDs)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *NoteService) UpdateNote(userID string, noteID string, baseRevision int64, input NoteWriteInput) (*NoteDTO, error) {
	userUUID, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}
	noteUUID, err := parseResourceUUID(noteID)
	if err != nil {
		return nil, err
	}
	tagIDs, err := parseUUIDList(input.TagIDs)
	if err != nil {
		return nil, err
	}
	attachmentIDs, err := parseUUIDList(input.AttachmentIDs)
	if err != nil {
		return nil, err
	}
	if err := validateNoteTitle(input.Title); err != nil {
		return nil, err
	}
	if err := validateAttachmentURIs(input.Markdown, attachmentIDs); err != nil {
		return nil, err
	}
	if int64(len(input.Markdown)) > s.cfg.MaxMarkdownBytes {
		return nil, pkg.NewAppError(http.StatusRequestEntityTooLarge, pkg.CodeNoteStorageLimitReached, "note storage limit reached")
	}

	var result *NoteDTO
	err = s.db.Transaction(func(tx *gorm.DB) error {
		noteRepo := s.noteRepo.WithTx(tx)
		tagRepo := s.tagRepo.WithTx(tx)
		attachmentRepo := s.attachmentRepo.WithTx(tx)
		changeRepo := s.changeRepo.WithTx(tx)

		if err := noteRepo.LockUserRow(tx, userUUID); err != nil {
			return pkg.InternalError(err)
		}

		// Read the note without locking to discover its current tags, then
		// lock tags first and the note row second, as the lock-ordering rule
		// requires. The locked re-read below is authoritative for the
		// revision check.
		_, err := noteRepo.FindByIDAndUserID(noteUUID, userUUID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return pkg.NewAppError(http.StatusNotFound, pkg.CodeNoteNotFound, "note not found")
			}
			return pkg.InternalError(err)
		}
        currentTagIDs, err := noteRepo.ListTagIDsForNote(userUUID, noteUUID)
		if err != nil {
			return pkg.InternalError(err)
		}
		lockTagIDs := dedupeUUIDs(append(append([]uuid.UUID{}, tagIDs...), currentTagIDs...))
        if err := noteRepo.LockTags(tx, userUUID, lockTagIDs); err != nil {
			return pkg.InternalError(err)
		}

		note, err := noteRepo.FindByIDAndUserIDForUpdate(tx, noteUUID, userUUID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return pkg.NewAppError(http.StatusNotFound, pkg.CodeNoteNotFound, "note not found")
			}
			return pkg.InternalError(err)
		}
		if note.DeletedAt != nil {
			return pkg.NewAppError(http.StatusNotFound, pkg.CodeNoteNotFound, "note not found")
		}
		if note.Revision != baseRevision {
			return pkg.NewAppError(http.StatusPreconditionFailed, pkg.CodeRevisionConflict, "revision conflict")
		}

		currentAttachmentIDs, err := noteRepo.ListAttachmentIDsForNote(userUUID, noteUUID)
		if err != nil {
			return pkg.InternalError(err)
		}
		lockAttachmentIDs := dedupeUUIDs(append(append([]uuid.UUID{}, attachmentIDs...), currentAttachmentIDs...))
		if err := noteRepo.LockAttachments(tx, userUUID, lockAttachmentIDs); err != nil {
			return pkg.InternalError(err)
		}
		if err := validateTagRefs(tagRepo, userUUID, tagIDs); err != nil {
			return err
		}
		if err := validateAttachmentRefs(attachmentRepo, userUUID, attachmentIDs); err != nil {
			return err
		}
		oldMarkdownBytes := int64(len(note.Markdown))
		if err := s.ensureMarkdownQuota(tx, userUUID, oldMarkdownBytes, int64(len(input.Markdown))); err != nil {
			return err
		}

		now := time.Now().UTC()
		note.Title = input.Title
		note.Markdown = input.Markdown
		note.Revision = note.Revision + 1
		note.UpdatedAt = now
		if err := tx.Model(&model.Note{}).
			Where("id = ? AND user_id = ?", noteUUID, userUUID).
			Updates(map[string]any{
				"title":      note.Title,
				"markdown":   note.Markdown,
				"revision":   note.Revision,
				"updated_at": now,
			}).Error; err != nil {
			return pkg.InternalError(err)
		}
		if err := s.storageRepo.AddMarkdown(tx, userUUID, int64(len(input.Markdown))-oldMarkdownBytes); err != nil {
			return pkg.InternalError(err)
		}
		if err := noteRepo.ReplaceTagRelations(noteUUID, userUUID, tagIDs); err != nil {
			return pkg.InternalError(err)
		}
		if err := noteRepo.ReplaceAttachmentRelations(noteUUID, userUUID, attachmentIDs); err != nil {
			return pkg.InternalError(err)
		}
		if err := attachmentRepo.MarkAssociated(tx, userUUID, attachmentIDs); err != nil {
			return pkg.InternalError(err)
		}
		if err := attachmentRepo.MarkUnassociated(tx, userUUID, currentAttachmentIDs, now); err != nil {
			return pkg.InternalError(err)
		}
		seq, err := appendChange(tx, changeRepo, userUUID, "note", noteUUID, "upsert", note.Revision)
		if err != nil {
			return err
		}
        if err := tx.Model(&model.Note{}).Where("user_id = ? AND id = ?", userUUID, noteUUID).Update("change_seq", seq).Error; err != nil {
			return pkg.InternalError(err)
		}
		note.ChangeSeq = seq

		attachments, err := attachmentRepo.FindActiveByIDs(userUUID, attachmentIDs)
		if err != nil {
			return pkg.InternalError(err)
		}
		result = buildNoteDTO(note, uuidStrings(tagIDs), attachments)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *NoteService) DeleteNote(userID string, noteID string, baseRevision int64) (*NoteDeleteResult, error) {
	userUUID, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}
	noteUUID, err := parseResourceUUID(noteID)
	if err != nil {
		return nil, err
	}

	var result *NoteDeleteResult
	err = s.db.Transaction(func(tx *gorm.DB) error {
		noteRepo := s.noteRepo.WithTx(tx)
		changeRepo := s.changeRepo.WithTx(tx)

		if err := noteRepo.LockUserRow(tx, userUUID); err != nil {
			return pkg.InternalError(err)
		}
		currentTagIDs, err := noteRepo.ListTagIDsForNote(userUUID, noteUUID)
		if err != nil {
			return pkg.InternalError(err)
		}
		if err := noteRepo.LockTags(tx, userUUID, currentTagIDs); err != nil {
			return pkg.InternalError(err)
		}

		note, err := noteRepo.FindByIDAndUserIDForUpdate(tx, noteUUID, userUUID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return pkg.NewAppError(http.StatusNotFound, pkg.CodeNoteNotFound, "note not found")
			}
			return pkg.InternalError(err)
		}
		if note.DeletedAt != nil {
			return pkg.NewAppError(http.StatusNotFound, pkg.CodeNoteNotFound, "note not found")
		}
		if note.Revision != baseRevision {
			return pkg.NewAppError(http.StatusPreconditionFailed, pkg.CodeRevisionConflict, "revision conflict")
		}
		attachmentIDs, err := noteRepo.ListAttachmentIDsForNote(userUUID, noteUUID)
		if err != nil {
			return pkg.InternalError(err)
		}
		if err := noteRepo.LockAttachments(tx, userUUID, attachmentIDs); err != nil {
			return pkg.InternalError(err)
		}

		now := time.Now().UTC()
		revision := note.Revision + 1
		if err := tx.Model(&model.Note{}).
            Where("user_id = ? AND id = ?", userUUID, noteUUID).
			Updates(map[string]any{
				"revision":   revision,
				"deleted_at": now,
				"updated_at": now,
			}).Error; err != nil {
			return pkg.InternalError(err)
		}
		if err := s.storageRepo.AddMarkdown(tx, userUUID, -int64(len(note.Markdown))); err != nil {
			return pkg.InternalError(err)
		}
        if err := tx.Where("user_id = ? AND note_id = ?", userUUID, noteUUID).Delete(&model.NoteTagRelation{}).Error; err != nil {
			return pkg.InternalError(err)
		}
		if err := tx.Where("user_id = ? AND note_id = ?", userUUID, noteUUID).Delete(&model.NoteAttachmentRelation{}).Error; err != nil {
			return pkg.InternalError(err)
		}
		if err := s.attachmentRepo.MarkUnassociated(tx, userUUID, attachmentIDs, now); err != nil {
			return pkg.InternalError(err)
		}
		seq, err := appendChange(tx, changeRepo, userUUID, "note", noteUUID, "delete", revision)
		if err != nil {
			return err
		}
        if err := tx.Model(&model.Note{}).Where("user_id = ? AND id = ?", userUUID, noteUUID).Update("change_seq", seq).Error; err != nil {
			return pkg.InternalError(err)
		}

		result = &NoteDeleteResult{
			NoteID:    noteUUID.String(),
			Revision:  revision,
			DeletedAt: now,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

// Callers hold the user row lock while checking and updating account usage.
func (s *NoteService) ensureMarkdownQuota(tx *gorm.DB, userID uuid.UUID, oldBytes int64, newBytes int64) error {
	usage, err := s.storageRepo.Get(tx, userID)
	if err != nil {
		return pkg.InternalError(err)
	}
	if usage.MarkdownBytes < oldBytes || exceedsLimit(s.cfg.LimitBytes, usage.AttachmentBytes, usage.MarkdownBytes-oldBytes, newBytes) {
		return pkg.NewAppError(http.StatusRequestEntityTooLarge, pkg.CodeNoteStorageLimitReached, "note storage limit reached")
	}

	return nil
}

func exceedsLimit(limit int64, values ...int64) bool {
	remaining := limit
	for _, value := range values {
		if value < 0 || value > remaining {
			return true
		}
		remaining -= value
	}
	return false
}

func (s *NoteService) buildCurrentNoteDTO(note *model.Note, tagIDs []uuid.UUID) (*NoteDTO, error) {
    attachmentIDs, err := s.noteRepo.ListAttachmentIDsForNote(note.UserID, note.ID)
	if err != nil {
		return nil, pkg.InternalError(err)
	}
	attachments, err := s.attachmentRepo.FindActiveByIDs(note.UserID, attachmentIDs)
	if err != nil {
		return nil, pkg.InternalError(err)
	}

	return buildNoteDTO(note, uuidStrings(tagIDs), attachments), nil
}

func buildNoteDTO(note *model.Note, tagIDs []string, attachments []model.NoteAttachment) *NoteDTO {
	dtos := make([]NoteAttachmentDTO, 0, len(attachments))
	for _, attachment := range attachments {
		dtos = append(dtos, buildAttachmentDTO(&attachment))
	}

	return &NoteDTO{
		NoteID:      note.ID.String(),
		Title:       note.Title,
		Markdown:    note.Markdown,
		TagIDs:      tagIDs,
		Attachments: dtos,
		Revision:    note.Revision,
		CreatedAt:   note.CreatedAt,
		UpdatedAt:   note.UpdatedAt,
	}
}

func buildAttachmentDTO(attachment *model.NoteAttachment) NoteAttachmentDTO {
	return NoteAttachmentDTO{
		AttachmentID: attachment.ID.String(),
		Kind:         attachment.Kind,
		FileName:     attachment.FileName,
		MediaType:    attachment.MediaType,
		Size:         attachment.Size,
		SHA256:       attachment.SHA256,
		CreatedAt:    attachment.CreatedAt,
	}
}

func validateTagRefs(tagRepo *repository.NoteTagRepository, userID uuid.UUID, tagIDs []uuid.UUID) error {
	if len(tagIDs) == 0 {
		return nil
	}

	tags, err := tagRepo.FindActiveByIDs(userID, tagIDs)
	if err != nil {
		return pkg.InternalError(err)
	}
	found := make(map[uuid.UUID]struct{}, len(tags))
	for _, tag := range tags {
		found[tag.ID] = struct{}{}
	}
	for _, tagID := range tagIDs {
		if _, ok := found[tagID]; !ok {
			return pkg.NewAppError(http.StatusConflict, pkg.CodeInvalidNoteReference, "invalid note reference")
		}
	}

	return nil
}

func validateAttachmentRefs(attachmentRepo *repository.NoteAttachmentRepository, userID uuid.UUID, attachmentIDs []uuid.UUID) error {
	if len(attachmentIDs) == 0 {
		return nil
	}

	attachments, err := attachmentRepo.FindActiveByIDs(userID, attachmentIDs)
	if err != nil {
		return pkg.InternalError(err)
	}
	found := make(map[uuid.UUID]struct{}, len(attachments))
	for _, attachment := range attachments {
		found[attachment.ID] = struct{}{}
	}
	for _, attachmentID := range attachmentIDs {
		if _, ok := found[attachmentID]; !ok {
			return pkg.NewAppError(http.StatusConflict, pkg.CodeInvalidNoteReference, "invalid note reference")
		}
	}

	return nil
}

func appendChange(tx *gorm.DB, changeRepo *repository.NoteChangeLogRepository, userID uuid.UUID, resourceType string, resourceID uuid.UUID, operation string, revision int64) (int64, error) {
	seq, err := changeRepo.Append(tx, &model.NoteChangeLog{
		UserID:       userID,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Operation:    operation,
		Revision:     revision,
	})
	if err != nil {
		return 0, pkg.InternalError(err)
	}

	return seq, nil
}
