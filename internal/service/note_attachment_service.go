package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"colink-server/internal/config"
	"colink-server/internal/model"
	"colink-server/internal/pkg"
	"colink-server/internal/repository"
)

var attachmentKinds = map[string]struct{}{
	"image": {},
	"file":  {},
}

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

const attachmentTempPrefix = "colink-upload-"

type AttachmentService struct {
	db             *gorm.DB
	attachmentRepo *repository.NoteAttachmentRepository
	noteRepo       *repository.NoteRepository
	storageRepo    *repository.NoteStorageRepository
	cfg            config.NotesConfig
}

func NewAttachmentService(
	db *gorm.DB,
	attachmentRepo *repository.NoteAttachmentRepository,
	noteRepo *repository.NoteRepository,
	cfg config.NotesConfig,
) *AttachmentService {
	return &AttachmentService{
		db:             db,
		attachmentRepo: attachmentRepo,
		noteRepo:       noteRepo,
		storageRepo:    repository.NewNoteStorageRepository(),
		cfg:            cfg,
	}
}

type stagedAttachmentUpload struct {
	AttachmentID string
	Kind         string
	SHA256       string
	FileName     string
	TempPath     string
	Size         int64
	MediaType    string
	Digest       string
}

// Upload consumes multipart parts in one pass and records the staged file in
// the same transaction as its metadata and quota usage.
func (s *AttachmentService) Upload(userID string, form *multipart.Reader) (*NoteAttachmentDTO, error) {
	userUUID, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}
	input, err := s.stageUpload(userUUID, form)
	if input != nil && input.TempPath != "" {
		defer func() { _ = os.Remove(input.TempPath) }()
	}
	if err != nil {
		return nil, err
	}
	attachmentUUID, err := parseResourceUUID(input.AttachmentID)
	if err != nil {
		return nil, err
	}
	if _, ok := attachmentKinds[input.Kind]; !ok {
		return nil, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidParameter, "invalid parameter")
	}
	if !sha256Pattern.MatchString(input.SHA256) {
		return nil, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidParameter, "invalid parameter")
	}
	if !utf8.ValidString(input.FileName) {
		return nil, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidParameter, "invalid parameter")
	}
	for _, value := range input.FileName {
		if unicode.IsControl(value) {
			return nil, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidParameter, "invalid parameter")
		}
	}
	tempPath := input.TempPath
	size := input.Size
	mediaType := input.MediaType
	actualDigest := input.Digest
	if actualDigest != input.SHA256 {
		return nil, pkg.NewAppError(http.StatusUnprocessableEntity, pkg.CodeAttachmentChecksumMismatch, "attachment checksum mismatch")
	}

	relativePath := filepath.Join(userUUID.String(), attachmentUUID.String())
	finalPath := filepath.Join(s.cfg.StoragePath, relativePath)

	renamed := false
	var attachment *model.NoteAttachment
	err = s.db.Transaction(func(tx *gorm.DB) error {
		attachmentRepo := s.attachmentRepo.WithTx(tx)
		noteRepo := s.noteRepo.WithTx(tx)

		if err := noteRepo.LockUserRow(tx, userUUID); err != nil {
			return pkg.InternalError(err)
		}

		var existing model.NoteAttachment
        lookupErr := tx.Where("user_id = ? AND id = ?", userUUID, attachmentUUID).First(&existing).Error
		if lookupErr == nil {
			// Tombstones keep the id reserved forever; live attachments are
			// immutable, so any existing row rejects the upload.
			return pkg.NewAppError(http.StatusConflict, pkg.CodeAttachmentIDUnavailable, "attachment ID unavailable")
		}
		if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			return pkg.InternalError(lookupErr)
		}

		usage, err := s.storageRepo.Get(tx, userUUID)
		if err != nil {
			return pkg.InternalError(err)
		}
		if exceedsLimit(s.cfg.LimitBytes, usage.AttachmentBytes, usage.MarkdownBytes, size) {
			return pkg.NewAppError(http.StatusRequestEntityTooLarge, pkg.CodeNoteStorageLimitReached, "note storage limit reached")
		}

		createdAt := time.Now().UTC()
		attachment = &model.NoteAttachment{
			ID:          attachmentUUID,
			UserID:      userUUID,
			Kind:        input.Kind,
			FileName:    input.FileName,
			MediaType:   mediaType,
			Size:        size,
			SHA256:      actualDigest,
			StoragePath: relativePath,
			UnassociatedSince: &createdAt,
			CreatedAt:   createdAt,
		}
		if err := attachmentRepo.Create(attachment); err != nil {
			return pkg.InternalError(err)
		}
		if err := s.storageRepo.AddAttachment(tx, userUUID, size); err != nil {
			return pkg.InternalError(err)
		}

		if err := os.Rename(tempPath, finalPath); err != nil {
			return pkg.InternalError(err)
		}
		renamed = true
		return nil
	})
	if err != nil {
		if renamed {
			committed, lookupErr := s.attachmentRepo.FindByIDAndUserID(attachmentUUID, userUUID)
			if lookupErr == nil && committed.DeletedAt == nil && committed.StoragePath == relativePath &&
				committed.Size == size && committed.SHA256 == actualDigest {
				result := buildAttachmentDTO(committed)
				return &result, nil
			}
			if errors.Is(lookupErr, gorm.ErrRecordNotFound) {
				_ = os.Remove(finalPath)
			}
		}
		return nil, err
	}

	return &NoteAttachmentDTO{
		AttachmentID: attachment.ID.String(),
		Kind:         attachment.Kind,
		FileName:     attachment.FileName,
		MediaType:    attachment.MediaType,
		Size:         attachment.Size,
		SHA256:       attachment.SHA256,
		CreatedAt:    attachment.CreatedAt,
	}, nil
}

func (s *AttachmentService) stageUpload(userID uuid.UUID, form *multipart.Reader) (*stagedAttachmentUpload, error) {
	input := &stagedAttachmentUpload{}
	seen := make(map[string]bool, 4)
	for {
		part, err := form.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return input, multipartReadError(err)
		}
		name := part.FormName()
		if name != "attachmentId" && name != "kind" && name != "sha256" && name != "file" {
			if _, err := io.Copy(io.Discard, part); err != nil {
				return input, multipartReadError(err)
			}
			continue
		}
		if seen[name] {
			return input, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidRequestBody, "invalid request body")
		}
		seen[name] = true
		if name != "file" {
			value, err := io.ReadAll(io.LimitReader(part, 1<<20+1))
			if err != nil {
				return input, multipartReadError(err)
			}
			if len(value) > 1<<20 {
				return input, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidRequestBody, "invalid request body")
			}
			switch name {
			case "attachmentId":
				input.AttachmentID = string(value)
			case "kind":
				input.Kind = string(value)
			case "sha256":
				input.SHA256 = string(value)
			}
			continue
		}
		input.FileName = part.FileName()
		if err := os.MkdirAll(s.accountDir(userID), 0o755); err != nil {
			return input, pkg.InternalError(err)
		}
		tempFile, err := os.CreateTemp(s.accountDir(userID), attachmentTempPrefix+"-*")
		if err != nil {
			return input, pkg.InternalError(err)
		}
		input.TempPath = tempFile.Name()
		digest := sha256.New()
		input.Size, input.MediaType, err = s.streamToTemp(tempFile, io.TeeReader(part, digest))
		if err == nil {
			err = tempFile.Sync()
		}
		if closeErr := tempFile.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return input, err
		}
		input.Digest = hex.EncodeToString(digest.Sum(nil))
	}
	if !seen["attachmentId"] || !seen["kind"] || !seen["sha256"] || !seen["file"] {
		return input, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidRequestBody, "invalid request body")
	}
	return input, nil
}

func multipartReadError(err error) error {
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		return pkg.NewAppError(http.StatusRequestEntityTooLarge, pkg.CodeNoteStorageLimitReached, "note storage limit reached")
	}
	return pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidRequestBody, "invalid request body")
}

// streamToTemp copies the upload into the temp file, enforcing the maximum
// attachment size while streaming.
func (s *AttachmentService) streamToTemp(tempFile *os.File, reader io.Reader) (int64, string, error) {
	copyLimit := s.cfg.MaxAttachmentBytes
	if copyLimit < int64(^uint64(0)>>1) {
		copyLimit++
	}
	size, err := io.Copy(tempFile, io.LimitReader(reader, copyLimit))
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return 0, "", pkg.NewAppError(http.StatusRequestEntityTooLarge, pkg.CodeNoteStorageLimitReached, "note storage limit reached")
		}
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return 0, "", multipartReadError(err)
		}
		return 0, "", pkg.InternalError(err)
	}
	if size > s.cfg.MaxAttachmentBytes {
		return 0, "", pkg.NewAppError(http.StatusRequestEntityTooLarge, pkg.CodeNoteStorageLimitReached, "note storage limit reached")
	}

	if _, err := tempFile.Seek(0, io.SeekStart); err != nil {
		return 0, "", pkg.InternalError(err)
	}
	header := make([]byte, 512)
	headerSize, err := tempFile.Read(header)
	if err != nil && !errors.Is(err, io.EOF) {
		return 0, "", pkg.InternalError(err)
	}

	detected := http.DetectContentType(header[:headerSize])
	mediaType, _, parseErr := mime.ParseMediaType(detected)
	if parseErr != nil {
		return 0, "", pkg.InternalError(parseErr)
	}

	return size, mediaType, nil
}

func (s *AttachmentService) MaxUploadRequestBytes() int64 {
	const multipartOverhead = int64(1 << 20)
	if s.cfg.MaxAttachmentBytes > int64(^uint64(0)>>1)-multipartOverhead {
		return int64(^uint64(0) >> 1)
	}
	return s.cfg.MaxAttachmentBytes + multipartOverhead
}

func (s *AttachmentService) GetAttachment(userID string, attachmentID string) (*NoteAttachmentDTO, error) {
	attachment, err := s.findLiveAttachment(userID, attachmentID)
	if err != nil {
		return nil, err
	}

	dto := buildAttachmentDTO(attachment)
	return &dto, nil
}

type AttachmentContent struct {
	Attachment model.NoteAttachment
	Path       string
}

// GetContent resolves a live attachment for download.
func (s *AttachmentService) GetContent(userID string, attachmentID string) (*AttachmentContent, error) {
	attachment, err := s.findLiveAttachment(userID, attachmentID)
	if err != nil {
		return nil, err
	}
	if attachment.StoragePath == "" {
		return nil, pkg.InternalError(fmt.Errorf("attachment %s has empty storage path", attachment.ID))
	}

	path := filepath.Join(s.cfg.StoragePath, attachment.StoragePath)
	if !s.pathIsInsideStorage(path) {
		return nil, pkg.InternalError(fmt.Errorf("attachment %s escapes storage root", attachment.ID))
	}
	if _, err := os.Stat(path); err != nil {
		return nil, pkg.InternalError(err)
	}

	return &AttachmentContent{Attachment: *attachment, Path: path}, nil
}

// DeleteAttachment tombstones an unreferenced attachment. The id stays
// reserved forever; the content file is removed.
func (s *AttachmentService) DeleteAttachment(userID string, attachmentID string) error {
	userUUID, err := parseUUID(userID)
	if err != nil {
		return err
	}
	attachmentUUID, err := parseResourceUUID(attachmentID)
	if err != nil {
		return err
	}

	var storagePath string
	err = s.db.Transaction(func(tx *gorm.DB) error {
		attachmentRepo := s.attachmentRepo.WithTx(tx)
		noteRepo := s.noteRepo.WithTx(tx)

        attachment, err := attachmentRepo.LockAttachment(tx, userUUID, attachmentUUID)
		if err != nil {
			return pkg.InternalError(err)
		}
        if attachment == nil || attachment.DeletedAt != nil {
			return pkg.NewAppError(http.StatusNotFound, pkg.CodeAttachmentNotFound, "attachment not found")
		}

		references, err := noteRepo.CountActiveReferences(tx, userUUID, attachmentUUID)
		if err != nil {
			return pkg.InternalError(err)
		}
		if references > 0 {
			return pkg.NewAppError(http.StatusConflict, pkg.CodeAttachmentInUse, "attachment in use")
		}

        if err := attachmentRepo.Tombstone(tx, userUUID, attachmentUUID, time.Now().UTC()); err != nil {
			return pkg.InternalError(err)
		}
		if err := s.storageRepo.AddAttachment(tx, userUUID, -attachment.Size); err != nil {
			return pkg.InternalError(err)
		}
		storagePath = attachment.StoragePath
		return nil
	})
	if err != nil {
		return err
	}

	if storagePath != "" {
		path := filepath.Join(s.cfg.StoragePath, storagePath)
		if s.pathIsInsideStorage(path) {
			removeErr := os.Remove(path)
			if removeErr == nil || errors.Is(removeErr, os.ErrNotExist) {
				_ = s.attachmentRepo.ClearStoragePath(userUUID, attachmentUUID)
			}
		}
	}

	return nil
}

func (s *AttachmentService) ListReferences(userID string, attachmentID string, pageToken string, limitRaw string) (*AttachmentReferencesResult, error) {
	userUUID, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}
	attachmentUUID, err := parseResourceUUID(attachmentID)
	if err != nil {
		return nil, err
	}
	if _, err := s.findLiveAttachment(userID, attachmentID); err != nil {
		return nil, err
	}
	limit, err := parsePageLimit(limitRaw)
	if err != nil {
		return nil, err
	}

	var beforeID *uuid.UUID
	if pageToken != "" {
		payload, err := decodeOpaqueToken(pageToken)
        if err != nil || payload.Kind != tokenKindList || payload.UserID != userUUID.String() ||
            payload.AttachmentID != attachmentUUID.String() || payload.Limit != limit || payload.ID == "" {
			return nil, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidParameter, "invalid parameter")
		}
		id, err := uuid.Parse(payload.ID)
		if err != nil {
			return nil, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidParameter, "invalid parameter")
		}
		beforeID = &id
	}

	noteIDs, err := s.noteRepo.ListReferencedNoteIDs(userUUID, attachmentUUID, beforeID, limit+1)
	if err != nil {
		return nil, pkg.InternalError(err)
	}

	hasMore := len(noteIDs) > limit
	if hasMore {
		noteIDs = noteIDs[:limit]
	}

	var nextPageToken *string
	if hasMore && len(noteIDs) > 0 {
        token, err := encodeOpaqueToken(opaqueToken{
            Kind:         tokenKindList,
            UserID:       userUUID.String(),
            ID:           noteIDs[len(noteIDs)-1].String(),
            AttachmentID: attachmentUUID.String(),
            Limit:        limit,
        })
		if err != nil {
			return nil, pkg.InternalError(err)
		}
		nextPageToken = &token
	}

	ids := make([]string, 0, len(noteIDs))
	for _, noteID := range noteIDs {
		ids = append(ids, noteID.String())
	}

	return &AttachmentReferencesResult{NoteIDs: ids, NextPageToken: nextPageToken}, nil
}

type AttachmentReferencesResult struct {
	NoteIDs       []string `json:"noteIds"`
	NextPageToken *string  `json:"nextPageToken"`
}

type NotesStorageResult struct {
	UsedBytes          int64 `json:"usedBytes"`
	LimitBytes         int64 `json:"limitBytes"`
	RemainingBytes     int64 `json:"remainingBytes"`
	AttachmentBytes    int64 `json:"attachmentBytes"`
	MarkdownBytes      int64 `json:"markdownBytes"`
	MaxAttachmentBytes int64 `json:"maxAttachmentBytes"`
	MaxMarkdownBytes   int64 `json:"maxMarkdownBytes"`
}

func (s *AttachmentService) StorageUsage(userID string) (*NotesStorageResult, error) {
	userUUID, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}

	usage, err := s.storageRepo.Get(s.db, userUUID)
	if err != nil {
		return nil, pkg.InternalError(err)
	}

	usedBytes := usage.AttachmentBytes + usage.MarkdownBytes
	remainingBytes := s.cfg.LimitBytes - usedBytes
	if remainingBytes < 0 {
		remainingBytes = 0
	}

	return &NotesStorageResult{
		UsedBytes:          usedBytes,
		LimitBytes:         s.cfg.LimitBytes,
		RemainingBytes:     remainingBytes,
		AttachmentBytes:    usage.AttachmentBytes,
		MarkdownBytes:      usage.MarkdownBytes,
		MaxAttachmentBytes: s.cfg.MaxAttachmentBytes,
		MaxMarkdownBytes:   s.cfg.MaxMarkdownBytes,
	}, nil
}

func (s *AttachmentService) findLiveAttachment(userID string, attachmentID string) (*model.NoteAttachment, error) {
	userUUID, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}
	attachmentUUID, err := parseResourceUUID(attachmentID)
	if err != nil {
		return nil, err
	}

	attachment, err := s.attachmentRepo.FindByIDAndUserID(attachmentUUID, userUUID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, pkg.NewAppError(http.StatusNotFound, pkg.CodeAttachmentNotFound, "attachment not found")
		}
		return nil, pkg.InternalError(err)
	}
	if attachment.DeletedAt != nil {
		return nil, pkg.NewAppError(http.StatusNotFound, pkg.CodeAttachmentNotFound, "attachment not found")
	}

	return attachment, nil
}

func (s *AttachmentService) accountDir(userID uuid.UUID) string {
	return filepath.Join(s.cfg.StoragePath, userID.String())
}

func (s *AttachmentService) pathIsInsideStorage(path string) bool {
	root, err := filepath.Abs(s.cfg.StoragePath)
	if err != nil {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return false
	}

	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
