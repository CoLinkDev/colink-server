package handler

import (
	"github.com/gin-gonic/gin"

	"colink-server/internal/service"
)

type MeHandler struct {
	authService *service.AuthService
}

func NewMeHandler(authService *service.AuthService) *MeHandler {
	return &MeHandler{authService: authService}
}

func (h *MeHandler) Get(c *gin.Context) {
	result, err := h.authService.Me(userIDFromContext(c))
	if err != nil {
		writeError(c, err)
		return
	}

	success(c, result)
}

func (h *MeHandler) UpdateUsername(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
	}
	if !bindJSON(c, &req) {
		return
	}

	if err := h.authService.UpdateUsername(userIDFromContext(c), req.Username); err != nil {
		writeError(c, err)
		return
	}

	success(c, nil)
}
