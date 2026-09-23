package service

import (
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"

	"colink-server/internal/pkg"
)

const (
	noteMaxTitleRunes = 2000
	tagMaxNameRunes   = 200
)

var tagFolder = cases.Fold()

const attachmentURIPrefix = "colink-attachment://"

var attachmentURIIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
var attachmentSchemePattern = regexp.MustCompile(`(?i)colink-attachment:`)

type NoteAttachmentDTO struct {
	AttachmentID string    `json:"attachmentId"`
	Kind         string    `json:"kind"`
	FileName     string    `json:"fileName"`
	MediaType    string    `json:"mediaType"`
	Size         int64     `json:"size"`
	SHA256       string    `json:"sha256"`
	CreatedAt    time.Time `json:"createdAt"`
}

type NoteDTO struct {
	NoteID      string               `json:"noteId"`
	Title       string               `json:"title"`
	Markdown    string               `json:"markdown"`
	TagIDs      []string             `json:"tagIds"`
	Attachments []NoteAttachmentDTO  `json:"attachments"`
	Revision    int64                `json:"revision"`
	CreatedAt   time.Time            `json:"createdAt"`
	UpdatedAt   time.Time            `json:"updatedAt"`
}

type NoteSummaryDTO struct {
	NoteID          string    `json:"noteId"`
	Title           string    `json:"title"`
	TagIDs          []string  `json:"tagIds"`
	AttachmentCount int       `json:"attachmentCount"`
	Revision        int64     `json:"revision"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type TagDTO struct {
	TagID     string    `json:"tagId"`
	Name      string    `json:"name"`
	Revision  int64     `json:"revision"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// parseResourceUUID parses a client-generated resource ID. Only UUID v4 is
// accepted per the notes protocol.
func parseResourceUUID(value string) (uuid.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil || id.Version() != 4 {
		return uuid.Nil, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidParameter, "invalid parameter")
	}

	return id, nil
}

func validateNoteTitle(title string) error {
	if !utf8.ValidString(title) || utf8.RuneCountInString(title) > noteMaxTitleRunes {
		return pkg.NewAppError(http.StatusRequestEntityTooLarge, pkg.CodeNoteStorageLimitReached, "note storage limit reached")
	}
	for _, value := range title {
		if unicode.IsControl(value) {
			return pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidParameter, "invalid parameter")
		}
	}

	return nil
}

func validateTagValue(name string) error {
	if strings.TrimSpace(name) == "" || !utf8.ValidString(name) {
		return pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidParameter, "invalid parameter")
	}
	if utf8.RuneCountInString(name) > tagMaxNameRunes {
		return pkg.NewAppError(http.StatusRequestEntityTooLarge, pkg.CodeNoteStorageLimitReached, "note storage limit reached")
	}
	for _, value := range name {
		if unicode.IsControl(value) {
			return pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidParameter, "invalid parameter")
		}
	}

	return nil
}

func normalizeTagName(name string) string {
	folded := tagFolder.String(norm.NFC.String(strings.TrimSpace(name)))
	return norm.NFC.String(folded)
}

// dedupeUUIDs removes duplicates while preserving ascending order so that
// callers can lock resources in stable order.
func dedupeUUIDs(ids []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(ids))
	unique := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}

	return unique
}

func uuidStrings(ids []uuid.UUID) []string {
	values := make([]string, 0, len(ids))
	for _, id := range ids {
		values = append(values, id.String())
	}

	return values
}

func parseUUIDList(values []string) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0, len(values))
	seen := make(map[uuid.UUID]struct{}, len(values))
	for _, value := range values {
		id, err := parseResourceUUID(value)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[id]; exists {
			return nil, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidParameter, "invalid parameter")
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	return ids, nil
}

func validateAttachmentURIs(markdown string, attachmentIDs []uuid.UUID) error {
	allowed := make(map[string]struct{}, len(attachmentIDs))
	for _, id := range attachmentIDs {
		allowed[id.String()] = struct{}{}
	}

	remaining := markdown
	for {
		location := attachmentSchemePattern.FindStringIndex(remaining)
		if location == nil {
			return nil
		}
		index := location[0]
		if !strings.HasPrefix(remaining[index:], attachmentURIPrefix) {
			return pkg.NewAppError(http.StatusConflict, pkg.CodeInvalidNoteReference, "invalid note reference")
		}

		value := remaining[index+len(attachmentURIPrefix):]
		if len(value) < 36 {
			return pkg.NewAppError(http.StatusConflict, pkg.CodeInvalidNoteReference, "invalid note reference")
		}
		id := value[:36]
		if !attachmentURIIDPattern.MatchString(id) {
			return pkg.NewAppError(http.StatusConflict, pkg.CodeInvalidNoteReference, "invalid note reference")
		}
		if len(value) > 36 && !isAttachmentURITerminator(value[36]) {
			return pkg.NewAppError(http.StatusConflict, pkg.CodeInvalidNoteReference, "invalid note reference")
		}
		if _, exists := allowed[id]; !exists {
			return pkg.NewAppError(http.StatusConflict, pkg.CodeInvalidNoteReference, "invalid note reference")
		}

		remaining = value[36:]
	}
}

func isAttachmentURITerminator(value byte) bool {
	if value >= 0x80 {
		return true
	}
	if value == ' ' || value == '\t' || value == '\r' || value == '\n' {
		return true
	}

	return strings.ContainsRune(")]}>\"'`", rune(value))
}
