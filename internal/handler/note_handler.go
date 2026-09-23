package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"colink-server/internal/pkg"
	"colink-server/internal/service"
)

type NoteHandler struct {
	noteService *service.NoteService
}

func NewNoteHandler(noteService *service.NoteService) *NoteHandler {
	return &NoteHandler{noteService: noteService}
}

type createNoteRequest struct {
	NoteID        *string  `json:"noteId"`
	Title         *string  `json:"title"`
	Markdown      *string  `json:"markdown"`
	TagIDs        []string `json:"tagIds"`
	AttachmentIDs []string `json:"attachmentIds"`
}

func (h *NoteHandler) Create(c *gin.Context) {
	var req createNoteRequest
	if !h.bindWriteJSON(c, &req) {
		return
	}
	if req.NoteID == nil || req.Title == nil || req.Markdown == nil || req.TagIDs == nil || req.AttachmentIDs == nil {
		writeError(c, newInvalidRequestBodyError())
		return
	}

	result, err := h.noteService.CreateNote(userIDFromContext(c), *req.NoteID, service.NoteWriteInput{
		Title:         *req.Title,
		Markdown:      *req.Markdown,
		TagIDs:        req.TagIDs,
		AttachmentIDs: req.AttachmentIDs,
	})
	if err != nil {
		writeError(c, err)
		return
	}

	success(c, result)
}

func (h *NoteHandler) Get(c *gin.Context) {
	result, err := h.noteService.GetNote(userIDFromContext(c), c.Param("noteId"))
	if err != nil {
		writeError(c, err)
		return
	}

	success(c, result)
}

type updateNoteRequest struct {
	BaseRevision  *int64   `json:"baseRevision"`
	Title         *string  `json:"title"`
	Markdown      *string  `json:"markdown"`
	TagIDs        []string `json:"tagIds"`
	AttachmentIDs []string `json:"attachmentIds"`
}

func (h *NoteHandler) Update(c *gin.Context) {
	var req updateNoteRequest
	if !h.bindWriteJSON(c, &req) {
		return
	}

	if req.BaseRevision == nil || req.Title == nil || req.Markdown == nil || req.TagIDs == nil || req.AttachmentIDs == nil {
		writeError(c, newInvalidRequestBodyError())
		return
	}

	result, err := h.noteService.UpdateNote(userIDFromContext(c), c.Param("noteId"), *req.BaseRevision, service.NoteWriteInput{
		Title:         *req.Title,
		Markdown:      *req.Markdown,
		TagIDs:        req.TagIDs,
		AttachmentIDs: req.AttachmentIDs,
	})
	if err != nil {
		writeError(c, err)
		return
	}

	success(c, result)
}

func (h *NoteHandler) bindWriteJSON(c *gin.Context, req any) bool {
	controller := http.NewResponseController(c.Writer)
	_ = controller.SetReadDeadline(time.Now().Add(time.Minute))
	defer func() {
		_ = controller.SetReadDeadline(time.Time{})
	}()

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.noteService.MaxWriteRequestBytes())
	if err := c.ShouldBindJSON(req); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			writeError(c, pkg.NewAppError(http.StatusRequestEntityTooLarge, pkg.CodeNoteStorageLimitReached, "note storage limit reached"))
			return false
		}
		writeError(c, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidRequestBody, "invalid request body"))
		return false
	}
	return true
}

func (h *NoteHandler) Delete(c *gin.Context) {
	baseRevision, ok := parseBaseRevisionQuery(c)
	if !ok {
		return
	}

	result, err := h.noteService.DeleteNote(userIDFromContext(c), c.Param("noteId"), baseRevision)
	if err != nil {
		writeError(c, err)
		return
	}

	success(c, result)
}

func (h *NoteHandler) List(c *gin.Context) {
	result, err := h.noteService.ListNotes(
		userIDFromContext(c),
		c.Query("tagId"),
		c.Query("pageToken"),
		c.Query("limit"),
	)
	if err != nil {
		writeError(c, err)
		return
	}

	success(c, result)
}

// parseBaseRevisionQuery reads the required baseRevision query parameter.
func parseBaseRevisionQuery(c *gin.Context) (int64, bool) {
	raw := c.Query("baseRevision")
	if raw == "" {
		writeError(c, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidParameter, "invalid parameter"))
		return 0, false
	}

	value, err := parseInt64(raw)
	if err != nil {
		writeError(c, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidParameter, "invalid parameter"))
		return 0, false
	}

	return value, true
}
