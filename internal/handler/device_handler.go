package handler

import (
	"github.com/gin-gonic/gin"

	"colink-server/internal/service"
)

type DeviceHandler struct {
	deviceService *service.DeviceService
}

func NewDeviceHandler(deviceService *service.DeviceService) *DeviceHandler {
	return &DeviceHandler{deviceService: deviceService}
}

func (h *DeviceHandler) Register(c *gin.Context) {
	var req struct {
		DeviceID  string `json:"deviceId" binding:"required"`
		Name      string `json:"name" binding:"required"`
		Type      string `json:"type" binding:"required"`
		PublicKey string `json:"publicKey" binding:"required"`
	}
	if !bindJSON(c, &req) {
		return
	}

	result, err := h.deviceService.Register(userIDFromContext(c), req.DeviceID, req.Name, req.Type, req.PublicKey)
	if err != nil {
		writeError(c, err)
		return
	}

	success(c, result)
}

func (h *DeviceHandler) List(c *gin.Context) {
	result, err := h.deviceService.List(userIDFromContext(c))
	if err != nil {
		writeError(c, err)
		return
	}

	success(c, result)
}

func (h *DeviceHandler) Update(c *gin.Context) {
	var req struct {
		Name *string `json:"name"`
	}
	if !bindJSON(c, &req) {
		return
	}

	if err := h.deviceService.UpdateName(userIDFromContext(c), c.Param("deviceId"), req.Name); err != nil {
		writeError(c, err)
		return
	}

	success(c, nil)
}

func (h *DeviceHandler) Delete(c *gin.Context) {
	if err := h.deviceService.Delete(userIDFromContext(c), c.Param("deviceId")); err != nil {
		writeError(c, err)
		return
	}

	success(c, nil)
}

func (h *DeviceHandler) RotateKey(c *gin.Context) {
	var req struct {
		PublicKey string `json:"publicKey" binding:"required"`
	}
	if !bindJSON(c, &req) {
		return
	}

	if err := h.deviceService.RotateKey(userIDFromContext(c), c.Param("deviceId"), req.PublicKey); err != nil {
		writeError(c, err)
		return
	}

	success(c, nil)
}
