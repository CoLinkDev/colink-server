package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"colink-server/internal/model"
	"colink-server/internal/pkg"
	"colink-server/internal/repository"
	"colink-server/internal/ws"
)

const lastSeenUpdateInterval = time.Minute

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

type WsService struct {
	deviceRepo      *repository.DeviceRepository
	ticketRepo      *repository.TicketRepository
	hub             *ws.Hub
	ticketTTL       time.Duration
	ticketLimitMu   sync.Mutex
	ticketLimitByID map[string][]time.Time
	lastSeenMu      sync.Mutex
	lastSeenByID    map[uuid.UUID]time.Time
}

func NewWsService(
	deviceRepo *repository.DeviceRepository,
	ticketRepo *repository.TicketRepository,
	hub *ws.Hub,
	ticketTTL time.Duration,
) *WsService {
	return &WsService{
		deviceRepo:      deviceRepo,
		ticketRepo:      ticketRepo,
		hub:             hub,
		ticketTTL:       ticketTTL,
		ticketLimitByID: make(map[string][]time.Time),
		lastSeenByID:    make(map[uuid.UUID]time.Time),
	}
}

func (s *WsService) IssueTicket(userID string, deviceID string) (*TicketResult, error) {
	device, err := ensureOwnedDevice(s.deviceRepo, userID, deviceID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	if !s.allowTicketIssue(userID, now) {
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

	return &TicketResult{
		Ticket:    ticketValue,
		ExpiresIn: int64(s.ticketTTL / time.Second),
	}, nil
}

func (s *WsService) ConsumeTicket(ticket string) (*WsSession, error) {
	if ticket == "" {
		return nil, pkg.NewAppError(http.StatusUnauthorized, pkg.CodeUnauthorized, "unauthorized")
	}

	record, err := s.ticketRepo.ConsumeValid(ticket, time.Now().UTC())
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, pkg.NewAppError(http.StatusUnauthorized, pkg.CodeUnauthorized, "unauthorized")
		}
		return nil, pkg.InternalError(err)
	}

	device, err := s.deviceRepo.FindByIDAndUserID(record.DeviceID, record.UserID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
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

func (s *WsService) HandleConnected(client *ws.Client) {
	s.refreshLastSeen(client.DeviceUUID(), time.Now().UTC(), true)
	if s.hub.Register(client) {
		s.broadcastOnline(client)
	}
	s.sendOnlineCatchup(client)
}

func (s *WsService) HandleDisconnect(client *ws.Client) {
	if !s.hub.Unregister(client) {
		return
	}

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
	}
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

	_ = s.deviceRepo.UpdateLastSeen(deviceID, at)
}

func (s *WsService) allowTicketIssue(userID string, now time.Time) bool {
	s.ticketLimitMu.Lock()
	defer s.ticketLimitMu.Unlock()

	windowStart := now.Add(-time.Minute)
	history := s.ticketLimitByID[userID]
	filtered := make([]time.Time, 0, len(history))
	for _, item := range history {
		if item.After(windowStart) {
			filtered = append(filtered, item)
		}
	}
	if len(filtered) == 0 {
		delete(s.ticketLimitByID, userID)
	}
	if len(filtered) >= 5 {
		s.ticketLimitByID[userID] = filtered
		return false
	}

	filtered = append(filtered, now)
	s.ticketLimitByID[userID] = filtered
	return true
}

func (s *WsService) handleRelay(client *ws.Client, message ws.ClientMessage) {
	if message.To == nil {
		return
	}
	if _, err := parseUUID(*message.To); err != nil {
		return
	}

	from := client.DeviceID()
	to := *message.To
	s.hub.SendToDevice(client.UserID(), to, ws.MessageEnvelope{
		ID:            message.ID,
		Type:          "relay",
		From:          &from,
		To:            &to,
		CorrelationID: message.CorrelationID,
		Payload:       json.RawMessage(message.Payload),
		Timestamp:     time.Now().UTC().UnixMilli(),
	})
}

func (s *WsService) handleBroadcast(client *ws.Client, message ws.ClientMessage) {
	if len(message.Payload) == 0 {
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
