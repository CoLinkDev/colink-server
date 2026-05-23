package pkg

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Envelope struct {
	Code    int    `json:"code"`
	Data    any    `json:"data"`
	Message string `json:"message"`
}

func Success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Envelope{
		Code:    0,
		Data:    data,
		Message: "ok",
	})
}

func Error(c *gin.Context, err error) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		c.JSON(appErr.HTTPStatus, Envelope{
			Code:    appErr.Code,
			Data:    nil,
			Message: appErr.Message,
		})
		return
	}

	c.JSON(http.StatusInternalServerError, Envelope{
		Code:    CodeInternalError,
		Data:    nil,
		Message: "internal error",
	})
}
