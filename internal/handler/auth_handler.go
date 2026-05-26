package handler

import (
	"github.com/gin-gonic/gin"

	"colink-server/internal/service"
)

type AuthHandler struct {
	authService *service.AuthService
}

func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required"`
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if !bindJSON(c, &req) {
		return
	}

	result, err := h.authService.Register(req.Email, req.Username, req.Password)
	if err != nil {
		writeError(c, err)
		return
	}

	success(c, result)
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req struct {
		Identifier string `json:"identifier" binding:"required"`
		Password   string `json:"password" binding:"required"`
	}
	if !bindJSON(c, &req) {
		return
	}

	result, err := h.authService.Login(req.Identifier, req.Password)
	if err != nil {
		writeError(c, err)
		return
	}

	success(c, result)
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refreshToken" binding:"required"`
	}
	if !bindJSON(c, &req) {
		return
	}

	result, err := h.authService.Refresh(req.RefreshToken)
	if err != nil {
		writeError(c, err)
		return
	}

	success(c, result)
}

func (h *AuthHandler) Logout(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refreshToken" binding:"required"`
	}
	if !bindJSON(c, &req) {
		return
	}

	if err := h.authService.Logout(userIDFromContext(c), req.RefreshToken); err != nil {
		writeError(c, err)
		return
	}

	success(c, nil)
}

func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var req struct {
		OldPassword string `json:"oldPassword" binding:"required"`
		NewPassword string `json:"newPassword" binding:"required"`
	}
	if !bindJSON(c, &req) {
		return
	}

	if err := h.authService.ChangePassword(userIDFromContext(c), req.OldPassword, req.NewPassword); err != nil {
		writeError(c, err)
		return
	}

	success(c, nil)
}
