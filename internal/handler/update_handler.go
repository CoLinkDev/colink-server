package handler

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"

	"colink-server/internal/pkg"
	"colink-server/internal/service"
)

type UpdateHandler struct {
	updateService *service.UpdateService
}

func NewUpdateHandler(updateService *service.UpdateService) *UpdateHandler {
	return &UpdateHandler{updateService: updateService}
}

func (h *UpdateHandler) CheckUpdate(c *gin.Context) {
	result, err := h.updateService.GetLatestRelease(c.Query("platform"), c.Query("version"))
	if err != nil {
		writeError(c, err)
		return
	}

	success(c, result)
}

func (h *UpdateHandler) DownloadAsset(c *gin.Context) {
	filePath, err := h.updateService.GetAssetFilePath(
		c.Param("platform"),
		c.Param("version"),
		c.Param("filename"),
	)
	if err != nil {
		writeError(c, err)
		return
	}

	if _, err := os.Stat(filePath); err != nil {
		if os.IsNotExist(err) {
			writeError(c, pkg.NewAppError(http.StatusNotFound, pkg.CodeUpdateAssetNotFound, "asset not found"))
			return
		}
		writeError(c, pkg.InternalError(err))
		return
	}

	c.Header("Content-Type", "application/octet-stream")
	c.FileAttachment(filePath, c.Param("filename"))
}
