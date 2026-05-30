package service

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"colink-server/internal/model"
	"colink-server/internal/pkg"
	"colink-server/internal/repository"
	"colink-server/internal/ws"
)

type RegisterDeviceResult struct {
	DeviceID     string `json:"deviceId"`
	DeviceSecret string `json:"deviceSecret"`
}

type DeviceItem struct {
	DeviceID           string     `json:"deviceId"`
	Name               string     `json:"name"`
	Type               string     `json:"type"`
	Online             bool       `json:"online"`
	LastSeen           *time.Time `json:"lastSeen"`
	PublicKey          string     `json:"publicKey"`
	PublicKeyUpdatedAt *time.Time `json:"publicKeyUpdatedAt"`
}

type DeviceListResult struct {
	Devices []DeviceItem `json:"devices"`
}

type DeviceService struct {
	deviceRepo *repository.DeviceRepository
	hub        *ws.Hub
}

func NewDeviceService(deviceRepo *repository.DeviceRepository, hub *ws.Hub) *DeviceService {
	return &DeviceService{
		deviceRepo: deviceRepo,
		hub:        hub,
	}
}

func (s *DeviceService) Register(userID string, deviceID string, name string, deviceType string, publicKey string) (*RegisterDeviceResult, error) {
	userUUID, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}

	deviceUUID, err := parseDeviceUUID(deviceID)
	if err != nil {
		return nil, err
	}

	name = strings.TrimSpace(name)
	deviceType = strings.TrimSpace(deviceType)
	if err := validateDeviceName(name); err != nil {
		return nil, err
	}
	if err := validateDeviceType(deviceType); err != nil {
		return nil, err
	}
	if err := validatePublicKey(publicKey); err != nil {
		return nil, err
	}

	count, err := s.deviceRepo.CountByUserID(userUUID)
	if err != nil {
		return nil, pkg.InternalError(err)
	}
	if count >= 10 {
		return nil, pkg.NewAppError(http.StatusConflict, pkg.CodeDeviceLimitReached, "device limit reached")
	}

	exists, err := s.deviceRepo.ExistsByID(deviceUUID)
	if err != nil {
		return nil, pkg.InternalError(err)
	}
	if exists {
		return nil, pkg.NewAppError(http.StatusConflict, pkg.CodeDeviceIDConflict, "device id conflict")
	}

	deviceSecret, err := pkg.GenerateOpaqueToken(48)
	if err != nil {
		return nil, pkg.InternalError(err)
	}
	now := time.Now().UTC()

	device := &model.Device{
		ID:                 deviceUUID,
		UserID:             userUUID,
		Name:               name,
		Type:               deviceType,
		PublicKey:          strings.TrimSpace(publicKey),
		PublicKeyUpdatedAt: now,
		DeviceSecret:       deviceSecret,
	}
	if err := s.deviceRepo.Create(device); err != nil {
		return nil, pkg.InternalError(err)
	}

	return &RegisterDeviceResult{
		DeviceID:     device.ID.String(),
		DeviceSecret: deviceSecret,
	}, nil
}

func (s *DeviceService) List(userID string) (*DeviceListResult, error) {
	userUUID, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}

	devices, err := s.deviceRepo.ListByUserID(userUUID)
	if err != nil {
		return nil, pkg.InternalError(err)
	}

	items := make([]DeviceItem, 0, len(devices))
	for _, device := range devices {
		items = append(items, DeviceItem{
			DeviceID:           device.ID.String(),
			Name:               device.Name,
			Type:               device.Type,
			Online:             s.hub.IsOnline(userID, device.ID.String()),
			LastSeen:           device.LastSeenAt,
			PublicKey:          device.PublicKey,
			PublicKeyUpdatedAt: &device.PublicKeyUpdatedAt,
		})
	}

	return &DeviceListResult{Devices: items}, nil
}

func (s *DeviceService) UpdateName(userID string, deviceID string, name *string) error {
	if name == nil {
		return nil
	}

	trimmed := strings.TrimSpace(*name)
	if err := validateDeviceName(trimmed); err != nil {
		return err
	}

	device, err := ensureOwnedDevice(s.deviceRepo, userID, deviceID)
	if err != nil {
		return err
	}

	if err := s.deviceRepo.UpdateName(device.ID, device.UserID, trimmed); err != nil {
		return pkg.InternalError(err)
	}

	return nil
}

func (s *DeviceService) Delete(userID string, deviceID string) error {
	device, err := ensureOwnedDevice(s.deviceRepo, userID, deviceID)
	if err != nil {
		return err
	}

	if err := s.deviceRepo.Delete(device.ID, device.UserID); err != nil {
		return pkg.InternalError(err)
	}

	s.hub.Disconnect(userID, deviceID)
	return nil
}

func (s *DeviceService) RotateKey(userID string, deviceID string, publicKey string) error {
	if err := validatePublicKey(publicKey); err != nil {
		return err
	}

	device, err := ensureOwnedDevice(s.deviceRepo, userID, deviceID)
	if err != nil {
		return err
	}

	if err := s.deviceRepo.UpdatePublicKey(device.ID, device.UserID, strings.TrimSpace(publicKey)); err != nil {
		return pkg.InternalError(err)
	}

	return nil
}

func parseDeviceUUID(value string) (uuid.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil || id.Version() != 4 {
		return uuid.Nil, pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidDeviceKey, "invalid device id")
	}

	return id, nil
}
