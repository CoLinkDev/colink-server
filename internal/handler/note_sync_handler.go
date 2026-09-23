package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"colink-server/internal/service"
)

type SyncHandler struct {
	syncService *service.SyncService
}

func NewSyncHandler(syncService *service.SyncService) *SyncHandler {
	return &SyncHandler{syncService: syncService}
}

func (h *SyncHandler) Snapshot(c *gin.Context) {
	result, err := h.syncService.Snapshot(
		userIDFromContext(c),
		c.Query("pageToken"),
		c.Query("limit"),
	)
	if err != nil {
		writeError(c, err)
		return
	}

	success(c, result)
}

func (h *SyncHandler) Changes(c *gin.Context) {
	result, err := h.syncService.Changes(
		userIDFromContext(c),
		c.Query("cursor"),
		c.Query("limit"),
	)
	if err != nil {
		writeError(c, err)
		return
	}

	success(c, result)
}

func parseInt64(raw string) (int64, error) {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, err
	}

	return value, nil
}
