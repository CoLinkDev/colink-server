package pkg

import "net/http"

const (
	CodeInternalError      = -1
	CodeUnauthorized       = 1030
	CodeRateLimited        = 3001
	CodeInvalidRequestBody = 4001
	CodeInvalidParameter   = 4002

	CodeEmailAlreadyExists    = 1001
	CodeInvalidEmailFormat    = 1002
	CodePasswordTooShort      = 1003
	CodeUsernameAlreadyExists = 1004
	CodeInvalidUsername       = 1005
	CodeInvalidCredentials    = 1010
	CodeAccountDisabled       = 1011
	CodeInvalidRefreshToken   = 1020
	CodeRefreshTokenRevoked   = 1021

	CodeDeviceLimitReached = 2001
	CodeInvalidDeviceType  = 2002
	CodeInvalidDeviceKey   = 2003
	CodeDeviceIDConflict   = 2004
	CodeDeviceNotFound     = 2010
)

type AppError struct {
	Code       int
	Message    string
	HTTPStatus int
	Err        error
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}

	return e.Message
}

func (e *AppError) Unwrap() error {
	return e.Err
}

func NewAppError(status int, code int, message string) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		HTTPStatus: status,
	}
}

func WrapError(err error, status int, code int, message string) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		HTTPStatus: status,
		Err:        err,
	}
}

func InternalError(err error) *AppError {
	return WrapError(err, http.StatusInternalServerError, CodeInternalError, "internal error")
}
