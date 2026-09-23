package model

import (
	"time"

	"github.com/google/uuid"
)

type Note struct {
	UserID    uuid.UUID  `gorm:"type:uuid;primaryKey;autoIncrement:false;index:idx_notes_user_id"`
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey;autoIncrement:false"`
	Title     string     `gorm:"type:text;not null"`
	Markdown  string     `gorm:"type:text;not null"`
	Revision  int64      `gorm:"not null;default:1"`
	ChangeSeq int64      `gorm:"not null;default:0"`
	DeletedAt *time.Time `gorm:"type:timestamptz;index"`
	CreatedAt time.Time  `gorm:"not null;default:now()"`
	UpdatedAt time.Time  `gorm:"not null;default:now()"`
}

func (Note) TableName() string { return "notes" }

type NoteTag struct {
	UserID         uuid.UUID  `gorm:"type:uuid;primaryKey;autoIncrement:false;index:idx_note_tags_user_id"`
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey;autoIncrement:false"`
	Name           string     `gorm:"type:text;not null"`
	NameNormalized string     `gorm:"type:text;not null"`
	Revision       int64      `gorm:"not null;default:1"`
	ChangeSeq      int64      `gorm:"not null;default:0"`
	DeletedAt      *time.Time `gorm:"type:timestamptz"`
	CreatedAt      time.Time  `gorm:"not null;default:now()"`
	UpdatedAt      time.Time  `gorm:"not null;default:now()"`
}

func (NoteTag) TableName() string { return "note_tags" }

type NoteTagRelation struct {
	UserID uuid.UUID `gorm:"type:uuid;primaryKey;autoIncrement:false;index:idx_note_tag_relations_user_tag"`
	NoteID uuid.UUID `gorm:"type:uuid;primaryKey;autoIncrement:false"`
	TagID  uuid.UUID `gorm:"type:uuid;primaryKey;autoIncrement:false"`
}

func (NoteTagRelation) TableName() string { return "note_tag_relations" }

type NoteAttachment struct {
	UserID            uuid.UUID  `gorm:"type:uuid;primaryKey;autoIncrement:false;index:idx_note_attachments_user"`
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey;autoIncrement:false"`
	Kind              string     `gorm:"column:kind;size:10;not null;check:chk_note_attachment_kind,kind IN ('image','file')"`
	FileName          string     `gorm:"type:text;not null"`
	MediaType         string     `gorm:"size:255;not null"`
	Size              int64      `gorm:"not null"`
	SHA256            string     `gorm:"column:sha256;size:64;not null"`
	StoragePath       string     `gorm:"type:text;not null"`
	UnassociatedSince *time.Time `gorm:"type:timestamptz;index"`
	DeletedAt         *time.Time `gorm:"type:timestamptz"`
	CreatedAt         time.Time  `gorm:"not null;default:now()"`
}

func (NoteAttachment) TableName() string { return "note_attachments" }

type NoteAttachmentRelation struct {
	UserID       uuid.UUID `gorm:"type:uuid;primaryKey;autoIncrement:false"`
	NoteID       uuid.UUID `gorm:"type:uuid;primaryKey;autoIncrement:false"`
	AttachmentID uuid.UUID `gorm:"type:uuid;primaryKey;autoIncrement:false"`
}

func (NoteAttachmentRelation) TableName() string { return "note_attachment_relations" }

type NoteChangeLog struct {
	Seq          int64     `gorm:"primaryKey;autoIncrement"`
	UserID       uuid.UUID `gorm:"type:uuid;not null;index:idx_note_change_log_user_seq"`
	ResourceType string    `gorm:"size:10;not null"`
	ResourceID   uuid.UUID `gorm:"type:uuid;not null"`
	Operation    string    `gorm:"size:10;not null"`
	Revision     int64     `gorm:"not null"`
	CreatedAt    time.Time `gorm:"not null;default:now()"`
}

func (NoteChangeLog) TableName() string { return "note_change_log" }

type NoteSyncState struct {
	UserID              uuid.UUID `gorm:"type:uuid;primaryKey"`
	CompactedThroughSeq int64     `gorm:"column:compacted_through_seq;not null;default:0"`
}

func (NoteSyncState) TableName() string { return "note_sync_state" }

type NoteStorageUsage struct {
	UserID          uuid.UUID `gorm:"type:uuid;primaryKey"`
	MarkdownBytes   int64     `gorm:"not null;default:0"`
	AttachmentBytes int64     `gorm:"not null;default:0"`
}

func (NoteStorageUsage) TableName() string { return "note_storage_usage" }
