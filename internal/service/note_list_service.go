package service

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"colink-server/internal/model"
	"colink-server/internal/pkg"
)

const notesPageDefaultLimit = 100
const notesPageMaxLimit = 500

// parsePageLimit validates the limit query parameter of the notes list
// endpoints (1..500, default 100).
func parsePageLimit(raw string) (int, error) {
	if raw == "" {
		return notesPageDefaultLimit, nil
	}

	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > notesPageMaxLimit {
		return 0, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidParameter, "invalid parameter")
	}

	return limit, nil
}

// notesCursor is the decoded keyset position of a notes list page.
type notesCursor struct {
	UpdatedAt *time.Time
	ID        *uuid.UUID
}

func parseNotesPageToken(raw string, userID uuid.UUID, tagID string, limit int) (*notesCursor, error) {
	if raw == "" {
		return &notesCursor{}, nil
	}

	payload, err := decodeOpaqueToken(raw)
	if err != nil || payload.Kind != tokenKindList || payload.ID == "" || payload.UpdatedAt == "" ||
        payload.UserID != userID.String() || payload.TagID != tagID || payload.Limit != limit {
		return nil, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidParameter, "invalid parameter")
	}

	updatedAt, err := time.Parse(time.RFC3339Nano, payload.UpdatedAt)
	if err != nil {
		return nil, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidParameter, "invalid parameter")
	}
	id, err := uuid.Parse(payload.ID)
	if err != nil {
		return nil, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidParameter, "invalid parameter")
	}

	return &notesCursor{
		UpdatedAt: &updatedAt,
		ID:        &id,
	}, nil
}

func encodeNotesPageToken(note *model.Note, userID uuid.UUID, tagID string, limit int) (string, error) {
    return encodeOpaqueToken(opaqueToken{
        Kind:      tokenKindList,
        UserID:    userID.String(),
		UpdatedAt: note.UpdatedAt.UTC().Format(time.RFC3339Nano),
		ID:        note.ID.String(),
		TagID:     tagID,
		Limit:     limit,
	})
}

// ListNotes returns one page of NoteSummary objects for UI browsing.
func (s *NoteService) ListNotes(userID string, tagID string, pageToken string, limitRaw string) (*NotesListResult, error) {
	userUUID, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}
	limit, err := parsePageLimit(limitRaw)
	if err != nil {
		return nil, err
	}
	var tagUUID uuid.UUID
	canonicalTagID := ""
	if tagID != "" {
		tagUUID, err = parseResourceUUID(tagID)
		if err != nil {
			return nil, err
		}
		canonicalTagID = tagUUID.String()
	}
	cursor, err := parseNotesPageToken(pageToken, userUUID, canonicalTagID, limit)
	if err != nil {
		return nil, err
	}

	var result *NotesListResult
	err = s.db.Transaction(func(tx *gorm.DB) error {
		noteRepo := s.noteRepo.WithTx(tx)
		tagRepo := s.tagRepo.WithTx(tx)

		if tagID != "" {
			tag, err := tagRepo.FindByIDAndUserID(tagUUID, userUUID)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return pkg.NewAppError(http.StatusNotFound, pkg.CodeTagNotFound, "tag not found")
				}
				return pkg.InternalError(err)
			}
			if tag.DeletedAt != nil {
				return pkg.NewAppError(http.StatusNotFound, pkg.CodeTagNotFound, "tag not found")
			}
		}

		var notes []model.Note
		if tagID != "" {
			notes, err = noteRepo.ListNoteIDsByTag(userUUID, tagUUID, cursor.UpdatedAt, cursor.ID, limit+1)
		} else {
			notes, err = noteRepo.ListNotesPage(userUUID, cursor.UpdatedAt, cursor.ID, limit+1)
		}
		if err != nil {
			return pkg.InternalError(err)
		}

		hasMore := len(notes) > limit
		if hasMore {
			notes = notes[:limit]
		}

		noteIDs := make([]uuid.UUID, 0, len(notes))
		for _, note := range notes {
			noteIDs = append(noteIDs, note.ID)
		}
		tagIDsByNote, err := noteRepo.ListTagIDsForNotes(userUUID, noteIDs)
		if err != nil {
			return pkg.InternalError(err)
		}
		attachmentCounts, err := noteRepo.ListAttachmentCountsForNotes(userUUID, noteIDs)
		if err != nil {
			return pkg.InternalError(err)
		}

		summaries := make([]NoteSummaryDTO, 0, len(notes))
		for _, note := range notes {
			summaries = append(summaries, NoteSummaryDTO{
				NoteID:          note.ID.String(),
				Title:           note.Title,
				TagIDs:          uuidStrings(tagIDsByNote[note.ID]),
				AttachmentCount: int(attachmentCounts[note.ID]),
				Revision:        note.Revision,
				CreatedAt:       note.CreatedAt,
				UpdatedAt:       note.UpdatedAt,
			})
		}

		var nextPageToken *string
		if hasMore {
			token, err := encodeNotesPageToken(&notes[len(notes)-1], userUUID, canonicalTagID, limit)
			if err != nil {
				return pkg.InternalError(err)
			}
			nextPageToken = &token
		}

		result = &NotesListResult{Notes: summaries, NextPageToken: nextPageToken}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}

	return result, nil
}

type NotesListResult struct {
	Notes         []NoteSummaryDTO `json:"notes"`
	NextPageToken *string          `json:"nextPageToken"`
}
