package service

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"colink-server/internal/model"
	"colink-server/internal/pkg"
	"colink-server/internal/repository"
)

const syncSnapshotPhaseNotes = "notes"
const syncSnapshotPhaseTags = "tags"

type SyncService struct {
	db             *gorm.DB
	noteRepo       *repository.NoteRepository
	tagRepo        *repository.NoteTagRepository
	attachmentRepo *repository.NoteAttachmentRepository
	changeRepo     *repository.NoteChangeLogRepository
}

func NewSyncService(
	db *gorm.DB,
	noteRepo *repository.NoteRepository,
	tagRepo *repository.NoteTagRepository,
	attachmentRepo *repository.NoteAttachmentRepository,
	changeRepo *repository.NoteChangeLogRepository,
) *SyncService {
	return &SyncService{
		db:             db,
		noteRepo:       noteRepo,
		tagRepo:        tagRepo,
		attachmentRepo: attachmentRepo,
		changeRepo:     changeRepo,
	}
}

type SnapshotResult struct {
	Notes         []*NoteDTO `json:"notes"`
	Tags          []*TagDTO  `json:"tags"`
	NextPageToken *string    `json:"nextPageToken"`
	Cursor        string     `json:"cursor"`
}

type ChangeEntry struct {
	Type      string    `json:"type"`
	Operation string    `json:"operation"`
	Note      *NoteDTO  `json:"note,omitempty"`
	Tag       *TagDTO   `json:"tag,omitempty"`
	NoteID    *string   `json:"noteId,omitempty"`
	TagID     *string   `json:"tagId,omitempty"`
	Revision  int64     `json:"revision,omitempty"`
}

type ChangesResult struct {
	Changes    []ChangeEntry `json:"changes"`
	NextCursor string        `json:"nextCursor"`
	HasMore    bool          `json:"hasMore"`
}

// Snapshot returns one page of the account-wide full snapshot. The change
// sequence is fixed on the first request of the pagination chain and every
// page of the chain is served against that fixed sequence, so resources
// modified mid-pagination fall through to the incremental stream.
func (s *SyncService) Snapshot(userID string, pageToken string, limitRaw string) (*SnapshotResult, error) {
	userUUID, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}
	limit, err := parsePageLimit(limitRaw)
	if err != nil {
		return nil, err
	}

	var result *SnapshotResult
	err = s.db.Transaction(func(tx *gorm.DB) error {
		transactional := *s
		transactional.db = tx
		transactional.noteRepo = s.noteRepo.WithTx(tx)
		transactional.tagRepo = s.tagRepo.WithTx(tx)
		transactional.attachmentRepo = s.attachmentRepo.WithTx(tx)
		transactional.changeRepo = s.changeRepo.WithTx(tx)

		result, err = transactional.snapshot(userUUID, pageToken, limit)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *SyncService) snapshot(userUUID uuid.UUID, pageToken string, limit int) (*SnapshotResult, error) {
	var snapshotSeq int64
	var err error
	phase := syncSnapshotPhaseNotes
	var afterSeq int64
	var afterID uuid.UUID
	if pageToken == "" {
		snapshotSeq, err = s.changeRepo.MaxSeq(s.db, userUUID)
		if err != nil {
			return nil, pkg.InternalError(err)
		}
	} else {
		payload, err := decodeOpaqueToken(pageToken)
        if err != nil || payload.Kind != tokenKindPage || payload.UserID != userUUID.String() || payload.LastID == "" {
			return nil, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidParameter, "invalid parameter")
		}
		if payload.Phase != syncSnapshotPhaseNotes && payload.Phase != syncSnapshotPhaseTags {
			return nil, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidParameter, "invalid parameter")
		}
		afterID, err = uuid.Parse(payload.LastID)
		if err != nil {
			return nil, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidParameter, "invalid parameter")
		}
		snapshotSeq = payload.Seq
		phase = payload.Phase
		afterSeq = payload.LastSeq
		compacted, err := s.changeRepo.CompactedThrough(s.db, userUUID)
		if err != nil {
			return nil, pkg.InternalError(err)
		}
		if snapshotSeq < compacted {
			return nil, pkg.NewAppError(http.StatusGone, pkg.CodeSyncCursorExpired, "sync cursor expired")
		}
	}

	notes := make([]*NoteDTO, 0)
	tags := make([]*TagDTO, 0)
	remaining := limit

	if phase == syncSnapshotPhaseNotes {
		// Fetch one extra row to learn whether more notes remain, so the
		// final page is detected without an extra empty page.
		noteRows, err := s.noteRepo.ListSnapshotNotes(userUUID, snapshotSeq, afterSeq, afterID, remaining+1)
		if err != nil {
			return nil, pkg.InternalError(err)
		}
		hasMoreNotes := len(noteRows) > remaining
		if hasMoreNotes {
			noteRows = noteRows[:remaining]
		}
        noteDTOs, err := s.buildSnapshotNoteDTOs(userUUID, noteRows)
		if err != nil {
			return nil, err
		}
		notes = append(notes, noteDTOs...)

		if hasMoreNotes {
			last := noteRows[len(noteRows)-1]
            return s.snapshotPage(userUUID, notes, tags, snapshotSeq, syncSnapshotPhaseNotes, last.ChangeSeq, last.ID)
		}

		remaining -= len(noteRows)
		if remaining == 0 {
			// The page filled exactly; continue from the last note's keyset.
			// The next page returns any remaining notes, then tags.
			last := noteRows[len(noteRows)-1]
            return s.snapshotPage(userUUID, notes, tags, snapshotSeq, syncSnapshotPhaseNotes, last.ChangeSeq, last.ID)
		}
		phase = syncSnapshotPhaseTags
		afterSeq = 0
		afterID = uuid.Nil
	}

	tagRows, err := s.tagRepo.ListSnapshotTags(userUUID, snapshotSeq, afterSeq, afterID, remaining+1)
	if err != nil {
		return nil, pkg.InternalError(err)
	}
	hasMoreTags := len(tagRows) > remaining
	if hasMoreTags {
		tagRows = tagRows[:remaining]
	}
	for i := range tagRows {
		tags = append(tags, buildTagDTO(&tagRows[i]))
	}
	if hasMoreTags {
		last := tagRows[len(tagRows)-1]
        return s.snapshotPage(userUUID, notes, tags, snapshotSeq, syncSnapshotPhaseTags, last.ChangeSeq, last.ID)
	}

    return s.snapshotPage(userUUID, notes, tags, snapshotSeq, "", 0, uuid.Nil)
}

// snapshotPage assembles the result. A non-empty phase indicates that more
// resources remain and a continuation token must be issued.
func (s *SyncService) snapshotPage(userID uuid.UUID, notes []*NoteDTO, tags []*TagDTO, snapshotSeq int64, phase string, lastSeq int64, lastID uuid.UUID) (*SnapshotResult, error) {
    cursor, err := encodeOpaqueToken(opaqueToken{Kind: tokenKindCursor, UserID: userID.String(), Seq: snapshotSeq})
	if err != nil {
		return nil, pkg.InternalError(err)
	}

	var nextPageToken *string
	if phase != "" {
        token, err := encodeOpaqueToken(opaqueToken{
            Kind:    tokenKindPage,
            UserID:  userID.String(),
			Seq:     snapshotSeq,
			Phase:   phase,
			LastSeq: lastSeq,
			LastID:  lastID.String(),
		})
		if err != nil {
			return nil, pkg.InternalError(err)
		}
		nextPageToken = &token
	}

	return &SnapshotResult{
		Notes:         notes,
		Tags:          tags,
		NextPageToken: nextPageToken,
		Cursor:        cursor,
	}, nil
}

func (s *SyncService) buildSnapshotNoteDTOs(userID uuid.UUID, rows []model.Note) ([]*NoteDTO, error) {
	if len(rows) == 0 {
		return nil, nil
	}

	noteIDs := make([]uuid.UUID, 0, len(rows))
	for _, note := range rows {
		noteIDs = append(noteIDs, note.ID)
	}
    tagIDsByNote, err := s.noteRepo.ListTagIDsForNotes(userID, noteIDs)
	if err != nil {
		return nil, pkg.InternalError(err)
	}
    attachmentsByNote, err := s.noteRepo.ListAttachmentsForNotes(userID, noteIDs)
	if err != nil {
		return nil, pkg.InternalError(err)
	}

	dtos := make([]*NoteDTO, 0, len(rows))
	for i := range rows {
		note := rows[i]
		attachments := attachmentsByNote[note.ID]
		if attachments == nil {
			attachments = []model.NoteAttachment{}
		}
		tagIDs := tagIDsByNote[note.ID]
		if tagIDs == nil {
			tagIDs = []uuid.UUID{}
		}
		dtos = append(dtos, buildNoteDTO(&note, uuidStrings(tagIDs), attachments))
	}

	return dtos, nil
}

// Changes returns the next page of account change events after the cursor.
func (s *SyncService) Changes(userID string, cursorRaw string, limitRaw string) (*ChangesResult, error) {
	userUUID, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}
	limit, err := parsePageLimit(limitRaw)
	if err != nil {
		return nil, err
	}

	if cursorRaw == "" {
		return nil, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidParameter, "invalid parameter")
	}
	payload, err := decodeOpaqueToken(cursorRaw)
    if err != nil || payload.Kind != tokenKindCursor || payload.UserID != userUUID.String() {
		return nil, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidParameter, "invalid parameter")
	}
	cursor := payload.Seq

	var entries []ChangeEntry
	var nextCursor = cursor
	var hasMore bool
	err = s.db.Transaction(func(tx *gorm.DB) error {
		compacted, err := s.changeRepo.CompactedThrough(tx, userUUID)
		if err != nil {
			return pkg.InternalError(err)
		}
		if cursor < compacted {
			return pkg.NewAppError(http.StatusGone, pkg.CodeSyncCursorExpired, "sync cursor expired")
		}

		events, err := s.changeRepo.ListEventsAfter(tx, userUUID, cursor, limit+1)
		if err != nil {
			return pkg.InternalError(err)
		}
		hasMore = len(events) > limit
		if hasMore {
			events = events[:limit]
		}

		entries = make([]ChangeEntry, 0, len(events))
		transactional := *s
		transactional.db = tx
		transactional.noteRepo = s.noteRepo.WithTx(tx)
		transactional.tagRepo = s.tagRepo.WithTx(tx)
		transactional.attachmentRepo = s.attachmentRepo.WithTx(tx)
		for _, event := range events {
			nextCursor = event.Seq
			entry, ok, err := transactional.materializeEvent(userUUID, event)
			if err != nil {
				return err
			}
			if ok {
				entries = append(entries, *entry)
			}
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}

    nextCursorToken, err := encodeOpaqueToken(opaqueToken{Kind: tokenKindCursor, UserID: userUUID.String(), Seq: nextCursor})
	if err != nil {
		return nil, pkg.InternalError(err)
	}

	return &ChangesResult{
		Changes:    entries,
		NextCursor: nextCursorToken,
		HasMore:    hasMore,
	}, nil
}

// materializeEvent converts a change-log event into a response entry.
// Upsert events whose resource is gone are skipped: their delete events are
// always later in the stream, so skipping an upsert never loses data.
func (s *SyncService) materializeEvent(userID uuid.UUID, event model.NoteChangeLog) (*ChangeEntry, bool, error) {
	switch {
	case event.ResourceType == "note" && event.Operation == "upsert":
		note, err := s.noteRepo.FindByIDAndUserID(event.ResourceID, userID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, false, nil
			}
			return nil, false, pkg.InternalError(err)
		}
		if note.DeletedAt != nil {
			return nil, false, nil
		}
        tagIDs, err := s.noteRepo.ListTagIDsForNote(userID, note.ID)
		if err != nil {
			return nil, false, pkg.InternalError(err)
		}
        attachmentIDs, err := s.noteRepo.ListAttachmentIDsForNote(userID, note.ID)
		if err != nil {
			return nil, false, pkg.InternalError(err)
		}
		attachments, err := s.attachmentRepo.FindActiveByIDs(userID, attachmentIDs)
		if err != nil {
			return nil, false, pkg.InternalError(err)
		}
		dto := buildNoteDTO(note, uuidStrings(tagIDs), attachments)
		return &ChangeEntry{
			Type:      "note",
			Operation: "upsert",
			Note:      dto,
		}, true, nil

	case event.ResourceType == "note" && event.Operation == "delete":
		noteID := event.ResourceID.String()
		return &ChangeEntry{
			Type:      "note",
			Operation: "delete",
			NoteID:    &noteID,
			Revision:  event.Revision,
		}, true, nil

	case event.ResourceType == "tag" && event.Operation == "upsert":
		tag, err := s.tagRepo.FindByIDAndUserID(event.ResourceID, userID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, false, nil
			}
			return nil, false, pkg.InternalError(err)
		}
		if tag.DeletedAt != nil {
			return nil, false, nil
		}
		return &ChangeEntry{
			Type:      "tag",
			Operation: "upsert",
			Tag:       buildTagDTO(tag),
		}, true, nil

	case event.ResourceType == "tag" && event.Operation == "delete":
		tagID := event.ResourceID.String()
		return &ChangeEntry{
			Type:      "tag",
			Operation: "delete",
			TagID:     &tagID,
			Revision:  event.Revision,
		}, true, nil
	}

	return nil, false, pkg.InternalError(errors.New("unknown change log entry"))
}
