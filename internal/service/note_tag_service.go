package service

import (
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"colink-server/internal/model"
	"colink-server/internal/pkg"
	"colink-server/internal/repository"
)

type TagService struct {
	db         *gorm.DB
	tagRepo    *repository.NoteTagRepository
	noteRepo   *repository.NoteRepository
	changeRepo *repository.NoteChangeLogRepository
}

func NewTagService(
	db *gorm.DB,
	tagRepo *repository.NoteTagRepository,
	noteRepo *repository.NoteRepository,
	changeRepo *repository.NoteChangeLogRepository,
) *TagService {
	return &TagService{
		db:         db,
		tagRepo:    tagRepo,
		noteRepo:   noteRepo,
		changeRepo: changeRepo,
	}
}

type TagDeleteResult struct {
	TagID     string    `json:"tagId"`
	Revision  int64     `json:"revision"`
	DeletedAt time.Time `json:"deletedAt"`
}

type TagListResult struct {
	Tags []TagDTO `json:"tags"`
}

func (s *TagService) CreateTag(userID string, tagID string, name string) (*TagDTO, error) {
	userUUID, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}
	tagUUID, err := parseResourceUUID(tagID)
	if err != nil {
		return nil, err
	}
	if err := validateTagValue(name); err != nil {
		return nil, err
	}

	normalized := normalizeTagName(name)
	now := time.Now().UTC()
	tag := &model.NoteTag{
		ID:             tagUUID,
		UserID:         userUUID,
		Name:           name,
		NameNormalized: normalized,
		Revision:       1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	var result *TagDTO
	err = s.db.Transaction(func(tx *gorm.DB) error {
		tagRepo := s.tagRepo.WithTx(tx)
		noteRepo := s.noteRepo.WithTx(tx)
		changeRepo := s.changeRepo.WithTx(tx)

		if err := noteRepo.LockUserRow(tx, userUUID); err != nil {
			return pkg.InternalError(err)
		}

		var existing model.NoteTag
        lookupErr := tx.Where("user_id = ? AND id = ?", userUUID, tagUUID).First(&existing).Error
		if lookupErr == nil {
			return pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidParameter, "invalid parameter")
		}
		if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			return pkg.InternalError(lookupErr)
		}

		if err := tagRepo.Create(tag); err != nil {
			if appErr := mapTagUniqueViolation(err); appErr != nil {
				return appErr
			}
			return pkg.InternalError(err)
		}
		seq, err := appendChange(tx, changeRepo, userUUID, "tag", tagUUID, "upsert", tag.Revision)
		if err != nil {
			return err
		}
        if err := tx.Model(&model.NoteTag{}).Where("user_id = ? AND id = ?", userUUID, tagUUID).Update("change_seq", seq).Error; err != nil {
			return pkg.InternalError(err)
		}
		tag.ChangeSeq = seq

		result = buildTagDTO(tag)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *TagService) ListTags(userID string) (*TagListResult, error) {
	userUUID, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}

	tags, err := s.tagRepo.ListActiveByUserID(userUUID)
	if err != nil {
		return nil, pkg.InternalError(err)
	}

	dtos := make([]TagDTO, 0, len(tags))
	for _, tag := range tags {
		dtos = append(dtos, *buildTagDTO(&tag))
	}

	return &TagListResult{Tags: dtos}, nil
}

func (s *TagService) UpdateTag(userID string, tagID string, baseRevision int64, name string) (*TagDTO, error) {
	userUUID, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}
	tagUUID, err := parseResourceUUID(tagID)
	if err != nil {
		return nil, err
	}
	if err := validateTagValue(name); err != nil {
		return nil, err
	}

	var result *TagDTO
	err = s.db.Transaction(func(tx *gorm.DB) error {
		tagRepo := s.tagRepo.WithTx(tx)
		noteRepo := s.noteRepo.WithTx(tx)
		changeRepo := s.changeRepo.WithTx(tx)

		if err := noteRepo.LockUserRow(tx, userUUID); err != nil {
			return pkg.InternalError(err)
		}

		tag, err := tagRepo.FindByIDAndUserIDForUpdate(tx, tagUUID, userUUID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return pkg.NewAppError(http.StatusNotFound, pkg.CodeTagNotFound, "tag not found")
			}
			return pkg.InternalError(err)
		}
		if tag.DeletedAt != nil {
			return pkg.NewAppError(http.StatusNotFound, pkg.CodeTagNotFound, "tag not found")
		}
		if tag.Revision != baseRevision {
			return pkg.NewAppError(http.StatusPreconditionFailed, pkg.CodeRevisionConflict, "revision conflict")
		}

		normalized := normalizeTagName(name)
		if duplicate, err := tagRepo.FindActiveByNormalizedName(tx, userUUID, normalized); err != nil {
			return pkg.InternalError(err)
		} else if duplicate != nil && duplicate.ID != tagUUID {
			return pkg.NewAppError(http.StatusConflict, pkg.CodeTagNameConflict, "tag name conflict")
		}

		now := time.Now().UTC()
		tag.Name = name
		tag.NameNormalized = normalized
		tag.Revision = tag.Revision + 1
		tag.UpdatedAt = now
		if err := tx.Model(&model.NoteTag{}).
			Where("id = ? AND user_id = ?", tagUUID, userUUID).
			Updates(map[string]any{
				"name":            tag.Name,
				"name_normalized": tag.NameNormalized,
				"revision":        tag.Revision,
				"updated_at":      now,
			}).Error; err != nil {
			if appErr := mapTagUniqueViolation(err); appErr != nil {
				return appErr
			}
			return pkg.InternalError(err)
		}
		seq, err := appendChange(tx, changeRepo, userUUID, "tag", tagUUID, "upsert", tag.Revision)
		if err != nil {
			return err
		}
        if err := tx.Model(&model.NoteTag{}).Where("user_id = ? AND id = ?", userUUID, tagUUID).Update("change_seq", seq).Error; err != nil {
			return pkg.InternalError(err)
		}
		tag.ChangeSeq = seq

		result = buildTagDTO(tag)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

// DeleteTag removes the tag and unlinks every associated note in one
// transaction. Locks are acquired tag-first then notes in ascending id
// order, matching the lock-ordering rule shared by all note writes.
func (s *TagService) DeleteTag(userID string, tagID string, baseRevision int64) (*TagDeleteResult, error) {
	userUUID, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}
	tagUUID, err := parseResourceUUID(tagID)
	if err != nil {
		return nil, err
	}

	var result *TagDeleteResult
	err = s.db.Transaction(func(tx *gorm.DB) error {
		tagRepo := s.tagRepo.WithTx(tx)
		noteRepo := s.noteRepo.WithTx(tx)
		changeRepo := s.changeRepo.WithTx(tx)

		if err := noteRepo.LockUserRow(tx, userUUID); err != nil {
			return pkg.InternalError(err)
		}

		tag, err := tagRepo.FindByIDAndUserIDForUpdate(tx, tagUUID, userUUID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return pkg.NewAppError(http.StatusNotFound, pkg.CodeTagNotFound, "tag not found")
			}
			return pkg.InternalError(err)
		}
		if tag.DeletedAt != nil {
			return pkg.NewAppError(http.StatusNotFound, pkg.CodeTagNotFound, "tag not found")
		}
		if tag.Revision != baseRevision {
			return pkg.NewAppError(http.StatusPreconditionFailed, pkg.CodeRevisionConflict, "revision conflict")
		}

		noteIDs, err := tagRepo.ListNoteIDsForTag(tx, userUUID, tagUUID)
		if err != nil {
			return pkg.InternalError(err)
		}
        if err := noteRepo.LockNotes(tx, userUUID, noteIDs); err != nil {
			return pkg.InternalError(err)
		}

		now := time.Now().UTC()
		tagRevision := tag.Revision + 1
		previousSeq, err := changeRepo.MaxSeq(tx, userUUID)
		if err != nil {
			return pkg.InternalError(err)
		}
		if len(noteIDs) > 0 {
			if err := tx.Exec(`UPDATE notes n
				SET revision = n.revision + 1, updated_at = ?
				FROM note_tag_relations r
				WHERE r.user_id = ? AND r.tag_id = ?
				  AND n.user_id = r.user_id AND n.id = r.note_id AND n.deleted_at IS NULL`,
				now, userUUID, tagUUID).Error; err != nil {
				return pkg.InternalError(err)
			}
			if err := tx.Exec(`INSERT INTO note_change_log
				(user_id, resource_type, resource_id, operation, revision, created_at)
				SELECT n.user_id, 'note', n.id, 'upsert', n.revision, ?
				FROM notes n JOIN note_tag_relations r
				  ON r.user_id = n.user_id AND r.note_id = n.id
				WHERE r.user_id = ? AND r.tag_id = ? AND n.deleted_at IS NULL
				ORDER BY n.id`, now, userUUID, tagUUID).Error; err != nil {
				return pkg.InternalError(err)
			}
			if err := tx.Exec(`UPDATE notes n SET change_seq = e.seq
				FROM note_change_log e
				WHERE e.user_id = ? AND e.seq > ? AND e.resource_type = 'note'
				  AND e.user_id = n.user_id AND e.resource_id = n.id`,
				userUUID, previousSeq).Error; err != nil {
				return pkg.InternalError(err)
			}
		}
		if err := tagRepo.RemoveTagRelations(tx, userUUID, tagUUID); err != nil {
			return pkg.InternalError(err)
		}
		if err := tx.Model(&model.NoteTag{}).
            Where("user_id = ? AND id = ?", userUUID, tagUUID).
			Updates(map[string]any{
				"revision":   tagRevision,
				"deleted_at": now,
				"updated_at": now,
			}).Error; err != nil {
			return pkg.InternalError(err)
		}
		seq, err := appendChange(tx, changeRepo, userUUID, "tag", tagUUID, "delete", tagRevision)
		if err != nil {
			return err
		}
        if err := tx.Model(&model.NoteTag{}).Where("user_id = ? AND id = ?", userUUID, tagUUID).Update("change_seq", seq).Error; err != nil {
			return pkg.InternalError(err)
		}

		result = &TagDeleteResult{
			TagID:     tagUUID.String(),
			Revision:  tagRevision,
			DeletedAt: now,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

func mapTagUniqueViolation(err error) *pkg.AppError {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return nil
	}
	if pgErr.ConstraintName == "idx_note_tags_user_normalized_active" {
		return pkg.NewAppError(http.StatusConflict, pkg.CodeTagNameConflict, "tag name conflict")
	}

	return nil
}

func buildTagDTO(tag *model.NoteTag) *TagDTO {
	return &TagDTO{
		TagID:     tag.ID.String(),
		Name:      tag.Name,
		Revision:  tag.Revision,
		CreatedAt: tag.CreatedAt,
		UpdatedAt: tag.UpdatedAt,
	}
}
