package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"colink-server/internal/middleware"
	"colink-server/internal/pkg"
)

func bindJSON(c *gin.Context, req any) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		pkg.Error(c, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidRequestBody, "invalid request body"))
		return false
	}

	return true
}

func userIDFromContext(c *gin.Context) string {
	value, _ := c.Get(middleware.ContextUserIDKey)
	userID, _ := value.(string)
	return userID
}
