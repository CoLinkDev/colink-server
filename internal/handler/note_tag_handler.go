package handler

import (
	"github.com/gin-gonic/gin"

	"colink-server/internal/service"
)

type NoteTagHandler struct {
	tagService *service.TagService
}

func NewNoteTagHandler(tagService *service.TagService) *NoteTagHandler {
	return &NoteTagHandler{tagService: tagService}
}

type createTagRequest struct {
	TagID *string `json:"tagId"`
	Name  *string `json:"name"`
}

func (h *NoteTagHandler) Create(c *gin.Context) {
	var req createTagRequest
	if !bindJSON(c, &req) {
		return
	}
	if req.TagID == nil || req.Name == nil {
		writeError(c, newInvalidRequestBodyError())
		return
	}

	result, err := h.tagService.CreateTag(userIDFromContext(c), *req.TagID, *req.Name)
	if err != nil {
		writeError(c, err)
		return
	}

	success(c, result)
}

func (h *NoteTagHandler) List(c *gin.Context) {
	result, err := h.tagService.ListTags(userIDFromContext(c))
	if err != nil {
		writeError(c, err)
		return
	}

	success(c, result)
}

type updateTagRequest struct {
	BaseRevision *int64  `json:"baseRevision"`
	Name         *string `json:"name"`
}

func (h *NoteTagHandler) Update(c *gin.Context) {
	var req updateTagRequest
	if !bindJSON(c, &req) {
		return
	}

	if req.BaseRevision == nil || req.Name == nil {
		writeError(c, newInvalidRequestBodyError())
		return
	}

	result, err := h.tagService.UpdateTag(userIDFromContext(c), c.Param("tagId"), *req.BaseRevision, *req.Name)
	if err != nil {
		writeError(c, err)
		return
	}

	success(c, result)
}

func (h *NoteTagHandler) Delete(c *gin.Context) {
	baseRevision, ok := parseBaseRevisionQuery(c)
	if !ok {
		return
	}

	result, err := h.tagService.DeleteTag(userIDFromContext(c), c.Param("tagId"), baseRevision)
	if err != nil {
		writeError(c, err)
		return
	}

	success(c, result)
}
