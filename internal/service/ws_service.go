package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"colink-server/internal/model"
	"colink-server/internal/pkg"
	"colink-server/internal/repository"
	"colink-server/internal/ws"
)

const (
	lastSeenUpdateInterval       = time.Minute
	cloudWebSocketProtocolMajor  = 1
	defaultCloudWebSocketVersion = "1.0.0"
	pushAcknowledgementTimeout   = 10 * time.Second
)

type TicketResult struct {
	Ticket    string `json:"ticket"`
	ExpiresIn int64  `json:"expiresIn"`
}

type WsSession struct {
	UserID     string
	DeviceID   string
	DeviceName string
	DeviceType string
}

type pendingPush struct {
	client *ws.Client
	done   chan struct{}
}

type WsService struct {
	deviceRepo      *repository.DeviceRepository
	ticketRepo      *repository.TicketRepository
	hub             *ws.Hub
	ticketTTL       time.Duration
	ticketRateLimit int
	ticketLimitMu   sync.Mutex
	ticketLimitByDeviceID map[string][]time.Time
	lastSeenMu      sync.Mutex
	lastSeenByID    map[uuid.UUID]time.Time
	pendingPushMu   sync.Mutex
	pendingPushes   map[string]pendingPush
	connectedAtMu   sync.Mutex
	connectedAt     map[*ws.Client]time.Time
	log             *zap.Logger
}

func NewWsService(
	deviceRepo *repository.DeviceRepository,
	ticketRepo *repository.TicketRepository,
	hub *ws.Hub,
	ticketTTL time.Duration,
	ticketRateLimit int,
	log *zap.Logger,
) *WsService {
	return &WsService{
		deviceRepo:      deviceRepo,
		ticketRepo:      ticketRepo,
		hub:             hub,
		ticketTTL:       ticketTTL,
		ticketRateLimit: ticketRateLimit,
		ticketLimitByDeviceID: make(map[string][]time.Time),
		lastSeenByID:    make(map[uuid.UUID]time.Time),
		pendingPushes:   make(map[string]pendingPush),
		connectedAt:     make(map[*ws.Client]time.Time),
		log:             log,
	}
}

func (s *WsService) IssueTicket(userID string, deviceID string) (*TicketResult, error) {
	device, err := ensureOwnedDevice(s.deviceRepo, userID, deviceID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	if !s.allowTicketIssue(device.ID.String(), now) {
		s.logger().Warn(
			"websocket ticket rate limited",
			zap.String("user_id", shortID(userID)),
			zap.String("device_id", shortID(deviceID)),
		)
		return nil, pkg.NewAppError(http.StatusTooManyRequests, pkg.CodeRateLimited, "rate limited")
	}

	ticketValue, err := pkg.GenerateOpaqueToken(48)
	if err != nil {
		return nil, pkg.InternalError(err)
	}

	record := &model.WsTicket{
		UserID:    device.UserID,
		DeviceID:  device.ID,
		Ticket:    ticketValue,
		ExpiresAt: now.Add(s.ticketTTL),
	}
	if err := s.ticketRepo.Create(record); err != nil {
		return nil, pkg.InternalError(err)
	}
	s.logger().Info(
		"websocket ticket issued",
		zap.String("user_id", shortID(userID)),
		zap.String("device_id", shortID(deviceID)),
		zap.Duration("ttl", s.ticketTTL),
	)

	return &TicketResult{
		Ticket:    ticketValue,
		ExpiresIn: int64(s.ticketTTL / time.Second),
	}, nil
}

func (s *WsService) ConsumeTicket(ticket string) (*WsSession, error) {
	if ticket == "" {
		s.logger().Warn("websocket ticket rejected", zap.String("reason", "missing"))
		return nil, pkg.NewAppError(http.StatusUnauthorized, pkg.CodeUnauthorized, "unauthorized")
	}

	record, err := s.ticketRepo.ConsumeValid(ticket, time.Now().UTC())
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			s.logger().Warn("websocket ticket rejected", zap.String("reason", "invalid_or_expired"))
			return nil, pkg.NewAppError(http.StatusUnauthorized, pkg.CodeUnauthorized, "unauthorized")
		}
		return nil, pkg.InternalError(err)
	}

	device, err := s.deviceRepo.FindByIDAndUserID(record.DeviceID, record.UserID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			s.logger().Warn("websocket ticket rejected", zap.String("reason", "device_missing"))
			return nil, pkg.NewAppError(http.StatusUnauthorized, pkg.CodeUnauthorized, "unauthorized")
		}
		return nil, pkg.InternalError(err)
	}

	return &WsSession{
		UserID:     record.UserID.String(),
		DeviceID:   record.DeviceID.String(),
		DeviceName: device.Name,
		DeviceType: device.Type,
	}, nil
}

func (s *WsService) ValidateBusinessVersion(version string) error {
	if version == "" || len(version) > 64 {
		return pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidParameter, "invalid businessVersion")
	}
	return nil
}

func (s *WsService) ValidateCloudWebSocketVersion(version string) (string, error) {
	parsed, ok := ws.ParseSemver(version)
	if !ok {
		return defaultCloudWebSocketVersion, nil
	}
	if parsed.Major != cloudWebSocketProtocolMajor {
		return "", pkg.NewAppError(http.StatusBadRequest, pkg.CodeInvalidParameter, "incompatible wsVersion")
	}
	return version, nil
}

func (s *WsService) HandleConnected(client *ws.Client) {
	s.connectedAtMu.Lock()
	s.connectedAt[client] = time.Now().UTC()
	s.connectedAtMu.Unlock()
	s.logger().Info(
		"websocket connected",
		zap.String("user_id", shortID(client.UserID())),
		zap.String("device_id", shortID(client.DeviceID())),
		zap.String("device_type", client.DeviceType()),
		zap.String("business_version", client.BusinessVersion()),
		zap.String("ws_version", client.WsVersion()),
	)
	s.refreshLastSeen(client.DeviceUUID(), time.Now().UTC(), true)
	s.hub.Register(client)
	s.broadcastOnline(client)
	s.sendOnlineCatchup(client)
}

func (s *WsService) HandleDisconnect(client *ws.Client) {
	s.connectedAtMu.Lock()
	connectedAt := s.connectedAt[client]
	delete(s.connectedAt, client)
	s.connectedAtMu.Unlock()
	if !s.hub.Unregister(client) {
		return
	}
	fields := []zap.Field{
		zap.String("user_id", shortID(client.UserID())),
		zap.String("device_id", shortID(client.DeviceID())),
	}
	if !connectedAt.IsZero() {
		fields = append(fields, zap.Duration("duration", time.Since(connectedAt)))
	}
	s.logger().Info("websocket disconnected", fields...)

	s.refreshLastSeen(client.DeviceUUID(), time.Now().UTC(), true)
	s.broadcastOffline(client)
}

func (s *WsService) HandleMessage(client *ws.Client, message ws.ClientMessage) {
	switch message.Type {
	case "ping":
		now := time.Now().UTC()
		s.refreshLastSeen(client.DeviceUUID(), now, false)
		client.Send(ws.MessageEnvelope{
			ID:        message.ID,
			Type:      "pong",
			Timestamp: now.UnixMilli(),
		})
	case "relay":
		s.handleRelay(client, message)
	case "broadcast":
		s.handleBroadcast(client, message)
	case "notification.push-ack":
		s.handlePushAcknowledgement(client, message.CorrelationID)
	}
}

func (s *WsService) DeliverPush(userID string, deviceID string, payload ws.PushNotificationPayload) error {
	if _, err := ensureOwnedDevice(s.deviceRepo, userID, deviceID); err != nil {
		return err
	}

	client := s.hub.ClientForDevice(userID, deviceID)
	if client == nil {
		return pkg.NewAppError(http.StatusOK, pkg.CodePushDeviceOffline, "device offline")
	}
	if !client.SupportsPushNotifications() {
		return pkg.NewAppError(http.StatusOK, pkg.CodePushNotSupported, "push not supported")
	}

	pushID := uuid.NewString()
	pending := pendingPush{
		client: client,
		done:   make(chan struct{}),
	}
	s.pendingPushMu.Lock()
	s.pendingPushes[pushID] = pending
	s.pendingPushMu.Unlock()

	if !client.Send(ws.PushEnvelope{
		ID:            pushID,
		Type:          "notification.push",
		From:          nil,
		To:            deviceID,
		CorrelationID: nil,
		Payload:       payload,
		Timestamp:     time.Now().UTC().UnixMilli(),
	}) {
		s.removePendingPush(pushID, pending)
		s.logger().Warn("push delivery failed", zap.String("device_id", shortID(deviceID)), zap.String("reason", "send_queue_closed"))
		return pkg.NewAppError(http.StatusOK, pkg.CodePushDeviceOffline, "device offline")
	}

	timer := time.NewTimer(pushAcknowledgementTimeout)
	defer timer.Stop()
	select {
	case <-pending.done:
		s.logger().Info("push delivered", zap.String("device_id", shortID(deviceID)))
		return nil
	case <-timer.C:
		s.removePendingPush(pushID, pending)
		s.logger().Warn("push delivery timed out", zap.String("device_id", shortID(deviceID)))
		return pkg.NewAppError(http.StatusOK, pkg.CodePushTimeout, "push timeout")
	}
}

func (s *WsService) handlePushAcknowledgement(client *ws.Client, correlationID *string) {
	if correlationID == nil || *correlationID == "" {
		return
	}

	s.pendingPushMu.Lock()
	pending, ok := s.pendingPushes[*correlationID]
	if !ok || pending.client != client {
		s.pendingPushMu.Unlock()
		return
	}
	delete(s.pendingPushes, *correlationID)
	close(pending.done)
	s.pendingPushMu.Unlock()
}

func (s *WsService) removePendingPush(pushID string, expected pendingPush) {
	s.pendingPushMu.Lock()
	current, ok := s.pendingPushes[pushID]
	if ok && current.client == expected.client && current.done == expected.done {
		delete(s.pendingPushes, pushID)
	}
	s.pendingPushMu.Unlock()
}

func (s *WsService) refreshLastSeen(deviceID uuid.UUID, at time.Time, force bool) {
	if !force {
		s.lastSeenMu.Lock()
		previous, ok := s.lastSeenByID[deviceID]
		if ok && at.Sub(previous) < lastSeenUpdateInterval {
			s.lastSeenMu.Unlock()
			return
		}
		s.lastSeenByID[deviceID] = at
		s.lastSeenMu.Unlock()
	} else {
		s.lastSeenMu.Lock()
		s.lastSeenByID[deviceID] = at
		s.lastSeenMu.Unlock()
	}

	if err := s.deviceRepo.UpdateLastSeen(deviceID, at); err != nil {
		s.logger().Warn("device last seen update failed", zap.String("device_id", shortID(deviceID.String())), zap.Error(err))
	}
}

func (s *WsService) allowTicketIssue(deviceID string, now time.Time) bool {
	s.ticketLimitMu.Lock()
	defer s.ticketLimitMu.Unlock()

	windowStart := now.Add(-time.Minute)
	history := s.ticketLimitByDeviceID[deviceID]
	filtered := make([]time.Time, 0, len(history))
	for _, item := range history {
		if item.After(windowStart) {
			filtered = append(filtered, item)
		}
	}
	if len(filtered) == 0 {
		delete(s.ticketLimitByDeviceID, deviceID)
	}
	if len(filtered) >= s.ticketRateLimit {
		s.ticketLimitByDeviceID[deviceID] = filtered
		return false
	}

	filtered = append(filtered, now)
	s.ticketLimitByDeviceID[deviceID] = filtered
	return true
}

func (s *WsService) handleRelay(client *ws.Client, message ws.ClientMessage) {
	if message.To == nil {
		s.logger().Warn("websocket relay rejected", zap.String("from_device_id", shortID(client.DeviceID())), zap.String("reason", "missing_target"))
		return
	}
	if _, err := parseUUID(*message.To); err != nil {
		s.logger().Warn("websocket relay rejected", zap.String("from_device_id", shortID(client.DeviceID())), zap.String("reason", "invalid_target"))
		return
	}

	from := client.DeviceID()
	to := *message.To
	if !s.hub.SendToDevice(client.UserID(), to, ws.MessageEnvelope{
		ID:            message.ID,
		Type:          "relay",
		From:          &from,
		To:            &to,
		CorrelationID: message.CorrelationID,
		Payload:       json.RawMessage(message.Payload),
		Timestamp:     time.Now().UTC().UnixMilli(),
	}) {
		s.logger().Debug("websocket relay target offline", zap.String("from_device_id", shortID(from)), zap.String("to_device_id", shortID(to)), zap.Int("payload_bytes", len(message.Payload)))
	}
}

func (s *WsService) handleBroadcast(client *ws.Client, message ws.ClientMessage) {
	if len(message.Payload) == 0 {
		s.logger().Warn("websocket broadcast rejected", zap.String("from_device_id", shortID(client.DeviceID())), zap.String("reason", "empty_payload"))
		return
	}

	from := client.DeviceID()
	s.hub.Broadcast(client.UserID(), client.DeviceID(), ws.MessageEnvelope{
		ID:            message.ID,
		Type:          "broadcast",
		From:          &from,
		To:            nil,
		CorrelationID: message.CorrelationID,
		Payload:       json.RawMessage(message.Payload),
		Timestamp:     time.Now().UTC().UnixMilli(),
	})
}

func (s *WsService) LogUpgradeFailure(err error) {
	s.logger().Warn("websocket upgrade failed", zap.Error(err))
}

func (s *WsService) LogClientCreationFailure(err error) {
	s.logger().Warn("websocket client initialization failed", zap.Error(err))
}

func (s *WsService) logger() *zap.Logger {
	if s.log == nil {
		return zap.NewNop()
	}
	return s.log
}

func shortID(value string) string {
	if len(value) <= 8 {
		return value
	}
	return value[:8]
}

func (s *WsService) broadcastOnline(client *ws.Client) {
	from := client.DeviceID()
	s.hub.Broadcast(client.UserID(), client.DeviceID(), ws.MessageEnvelope{
		ID:   uuid.NewString(),
		Type: "device.online",
		From: &from,
		Payload: ws.DeviceOnlinePayload{
			Name:            client.DeviceName(),
			Type:            client.DeviceType(),
			BusinessVersion: client.BusinessVersion(),
			WsVersion:       client.AdvertisedWsVersion(),
		},
		Timestamp: time.Now().UTC().UnixMilli(),
	})
}

func (s *WsService) sendOnlineCatchup(client *ws.Client) {
	now := time.Now().UTC().UnixMilli()
	for _, peer := range s.hub.ClientsForUser(client.UserID(), client.DeviceID()) {
		from := peer.DeviceID()
		client.Send(ws.MessageEnvelope{
			ID:   uuid.NewString(),
			Type: "device.online",
			From: &from,
			Payload: ws.DeviceOnlinePayload{
				Name:            peer.DeviceName(),
				Type:            peer.DeviceType(),
				BusinessVersion: peer.BusinessVersion(),
				WsVersion:       peer.AdvertisedWsVersion(),
			},
			Timestamp: now,
		})
	}
}

func (s *WsService) broadcastOffline(client *ws.Client) {
	from := client.DeviceID()
	s.hub.Broadcast(client.UserID(), client.DeviceID(), ws.MessageEnvelope{
		ID:        uuid.NewString(),
		Type:      "device.offline",
		From:      &from,
		Timestamp: time.Now().UTC().UnixMilli(),
	})
}
