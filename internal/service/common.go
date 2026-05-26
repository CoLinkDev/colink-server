package service

import (
	"encoding/base64"
	"errors"
	"net/http"
	"net/mail"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"colink-server/internal/model"
	"colink-server/internal/pkg"
	"colink-server/internal/repository"
)

var deviceTypes = map[string]struct{}{
	"windows": {},
	"android": {},
	"macos":   {},
	"linux":   {},
	"ios":     {},
}

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_.@-]+$`)

func parseUUID(value string) (uuid.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidParameter, "invalid parameter")
	}

	return id, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func normalizeUsername(username string) string {
	return strings.TrimSpace(username)
}

func validateEmail(email string) bool {
	parsed, err := mail.ParseAddress(email)
	if err != nil {
		return false
	}

	return parsed.Address == email
}

func validateUsername(username string) error {
	if length := len(username); length < 3 || length > 255 {
		return pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidUsername, "invalid username")
	}
	if !usernamePattern.MatchString(username) {
		return pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidUsername, "invalid username")
	}

	return nil
}

func validatePassword(password string) error {
	if len(password) < 8 {
		return pkg.NewAppError(http.StatusBadRequest, pkg.CodePasswordTooShort, "password too short")
	}

	return nil
}

func validateDeviceType(deviceType string) error {
	if _, ok := deviceTypes[deviceType]; !ok {
		return pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidDeviceType, "invalid device type")
	}

	return nil
}

func validateDeviceName(name string) error {
	if strings.TrimSpace(name) == "" {
		return pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidParameter, "invalid parameter")
	}

	return nil
}

func validatePublicKey(publicKey string) error {
	value := strings.TrimSpace(publicKey)
	if value == "" {
		return pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidDeviceKey, "invalid key")
	}

	if _, err := base64.StdEncoding.DecodeString(value); err == nil {
		return nil
	}
	if _, err := base64.RawStdEncoding.DecodeString(value); err == nil {
		return nil
	}

	return pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidDeviceKey, "invalid key")
}

func ensureOwnedDevice(deviceRepo *repository.DeviceRepository, userID string, deviceID string) (*model.Device, error) {
	userUUID, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}

	deviceUUID, err := parseUUID(deviceID)
	if err != nil {
		return nil, err
	}

	device, err := deviceRepo.FindByIDAndUserID(deviceUUID, userUUID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, pkg.NewAppError(http.StatusNotFound, pkg.CodeDeviceNotFound, "device not found")
		}
		return nil, pkg.InternalError(err)
	}

	return device, nil
}
