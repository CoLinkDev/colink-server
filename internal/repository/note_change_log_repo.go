package repository

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"colink-server/internal/model"
)

type NoteChangeLogRepository struct {
	db *gorm.DB
}

func NewNoteChangeLogRepository(db *gorm.DB) *NoteChangeLogRepository {
	return &NoteChangeLogRepository{db: db}
}

func (r *NoteChangeLogRepository) WithTx(tx *gorm.DB) *NoteChangeLogRepository {
	return &NoteChangeLogRepository{db: tx}
}

// Append writes one change event in the current transaction and returns the
// assigned account change sequence number.
func (r *NoteChangeLogRepository) Append(tx *gorm.DB, entry *model.NoteChangeLog) (int64, error) {
	if err := tx.Create(entry).Error; err != nil {
		return 0, err
	}

	return entry.Seq, nil
}

// MaxSeq returns the account's committed high-water mark, including events
// removed by compaction. Active resources retain their original change_seq.
func (r *NoteChangeLogRepository) MaxSeq(tx *gorm.DB, userID uuid.UUID) (int64, error) {
	var maxSeq int64
	if err := tx.Raw(`SELECT GREATEST(
		COALESCE((SELECT MAX(seq) FROM note_change_log WHERE user_id = ?), 0),
		COALESCE((SELECT compacted_through_seq FROM note_sync_state WHERE user_id = ?), 0)
	)`, userID, userID).Scan(&maxSeq).Error; err != nil {
		return 0, err
	}
	return maxSeq, nil
}

// CompactedThrough returns the highest sequence number compacted away for
// the account. Incremental cursors below this value are expired.
func (r *NoteChangeLogRepository) CompactedThrough(tx *gorm.DB, userID uuid.UUID) (int64, error) {
	var state model.NoteSyncState
	if err := tx.Where("user_id = ?", userID).First(&state).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return 0, nil
		}
		return 0, err
	}

	return state.CompactedThroughSeq, nil
}

// ListEventsAfter returns at most limit change events after the cursor in
// strictly increasing sequence order.
func (r *NoteChangeLogRepository) ListEventsAfter(tx *gorm.DB, userID uuid.UUID, cursor int64, limit int) ([]model.NoteChangeLog, error) {
	var events []model.NoteChangeLog
	if err := tx.Where("user_id = ? AND seq > ?", userID, cursor).
		Order("seq ASC").
		Limit(limit).
		Find(&events).Error; err != nil {
		return nil, err
	}

	return events, nil
}

// CompactBefore deletes change events created before the cutoff and raises
// each affected account's compacted watermark so cursors pointing at the
// removed entries expire.
func (r *NoteChangeLogRepository) CompactBefore(tx *gorm.DB, cutoff time.Time) error {
	var compacted []model.NoteSyncState
	if err := tx.Raw(
		`WITH deleted AS (
		     DELETE FROM note_change_log WHERE created_at < ? RETURNING user_id, seq
		 ) SELECT user_id, MAX(seq) AS compacted_through_seq FROM deleted GROUP BY user_id`,
		cutoff,
	).Scan(&compacted).Error; err != nil {
		return err
	}

	for _, state := range compacted {
		if err := tx.Exec(
			`INSERT INTO note_sync_state (user_id, compacted_through_seq)
			 VALUES (?, ?)
			 ON CONFLICT (user_id) DO UPDATE SET compacted_through_seq = GREATEST(note_sync_state.compacted_through_seq, EXCLUDED.compacted_through_seq)`,
			state.UserID, state.CompactedThroughSeq,
		).Error; err != nil {
			return err
		}
	}

	return nil
}
