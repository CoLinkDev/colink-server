package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"colink-server/internal/pkg"
)

type envelope struct {
	Code    int    `json:"code"`
	Data    any    `json:"data"`
	Message string `json:"message"`
}

func success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, envelope{
		Code:    0,
		Data:    data,
		Message: "ok",
	})
}

func writeError(c *gin.Context, err error) {
	var appErr *pkg.AppError
	if errors.As(err, &appErr) {
		c.JSON(appErr.HTTPStatus, envelope{
			Code:    appErr.Code,
			Data:    nil,
			Message: appErr.Message,
		})
		return
	}

	c.JSON(http.StatusInternalServerError, envelope{
		Code:    pkg.CodeInternalError,
		Data:    nil,
		Message: "internal error",
	})
}
